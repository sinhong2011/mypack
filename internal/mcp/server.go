// Package mcp implements the Model Context Protocol server of spec §12 over
// stdio: newline-delimited JSON-RPC 2.0 messages, no network listener.
//
// The tool surface is deliberately small and semantic (projects, runtimes,
// builds, artifacts). There is no shell, no arbitrary filesystem access and
// no way for an agent to approve its own runtime download: those need the
// mypack GUI (spec §12.5). Every call is written to the activity log with the
// actor "mcp:<clientInfo.name>".
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"mypack.dev/mypack/internal/appservice"
)

// Protocol versions this server speaks; the newest is offered when the
// client asks for something else.
var supportedProtocols = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// LatestProtocol is the version offered by default.
const LatestProtocol = "2025-06-18"

// maxMessage bounds a single incoming line.
const maxMessage = 8 << 20

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// ServerInfo identifies this server in the initialize response.
type ServerInfo struct{ Name, Version string }

// Server is one MCP session bound to a Service. Serve handles one connection.
type Server struct {
	svc  appservice.Service
	info ServerInfo

	tools  []tool
	byName map[string]*tool

	clientName atomic.Value // string
	protocol   atomic.Value // string

	wmu sync.Mutex // serialises writes to the output stream
	out io.Writer

	imu      sync.Mutex
	inflight map[string]context.CancelFunc // request id -> cancel
}

// NewServer creates a server. info.Name defaults to "mypack".
func NewServer(svc appservice.Service, info ServerInfo) *Server {
	if info.Name == "" {
		info.Name = "mypack"
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	s := &Server{svc: svc, info: info, inflight: map[string]context.CancelFunc{}}
	s.tools = buildTools(s)
	s.byName = map[string]*tool{}
	for i := range s.tools {
		s.byName[s.tools[i].Name] = &s.tools[i]
	}
	return s
}

// actor is the audit identity of the connected client.
func (s *Server) actor() string {
	n, _ := s.clientName.Load().(string)
	if n == "" {
		n = "unknown"
	}
	return "mcp:" + n
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads newline-delimited JSON-RPC messages from r and writes
// responses to w until r is exhausted or ctx is cancelled. Requests are
// handled concurrently; in-flight requests are finished before Serve returns
// on EOF. Only protocol messages are ever written to w.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	s.out = w
	var wg sync.WaitGroup
	br := bufio.NewReaderSize(r, 64<<10)

	type line struct {
		b   []byte
		err error
	}
	lines := make(chan line)
	go func() {
		defer close(lines)
		for {
			b, err := readLine(br)
			select {
			case lines <- line{b, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var result error
loop:
	for {
		select {
		case <-ctx.Done():
			result = ctx.Err()
			break loop
		case l, ok := <-lines:
			if !ok {
				break loop
			}
			if l.err != nil {
				if !errors.Is(l.err, io.EOF) {
					result = l.err
				}
				if len(bytes.TrimSpace(l.b)) > 0 {
					wg.Add(1)
					go func(b []byte) { defer wg.Done(); s.handleLine(ctx, b) }(l.b)
				}
				break loop
			}
			if len(bytes.TrimSpace(l.b)) == 0 {
				continue
			}
			wg.Add(1)
			go func(b []byte) { defer wg.Done(); s.handleLine(ctx, b) }(l.b)
		}
	}
	// Finish (or cancel, if the context ended) everything in flight.
	wg.Wait()
	return result
}

func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		buf = append(buf, chunk...)
		if len(buf) > maxMessage {
			return nil, fmt.Errorf("mcp: message exceeds %d bytes", maxMessage)
		}
		if err != nil {
			return buf, err
		}
		if !isPrefix {
			return buf, nil
		}
	}
}

func (s *Server) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &rpcError{Code: codeInternal, Message: "cannot encode response"}})
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_, _ = s.out.Write(append(b, '\n'))
}

func (s *Server) reply(id json.RawMessage, result any) {
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) fail(id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func (s *Server) handleLine(ctx context.Context, b []byte) {
	defer func() {
		if r := recover(); r != nil {
			s.fail(nil, codeInternal, fmt.Sprintf("internal error: %v", r))
		}
	}()
	var req rpcRequest
	if err := json.Unmarshal(b, &req); err != nil {
		s.fail(nil, codeParse, "parse error: "+err.Error())
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		// Responses to server-initiated requests (we send none) and junk.
		if len(req.ID) > 0 && req.Method == "" {
			return
		}
		s.fail(req.ID, codeInvalidRequest, "invalid request")
		return
	}
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		s.initialize(req)
	case "ping":
		if !isNotification {
			s.reply(req.ID, map[string]any{})
		}
	case "notifications/initialized":
		// nothing to do
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(req.Params, &p) == nil {
			s.imu.Lock()
			cancel := s.inflight[string(p.RequestID)]
			s.imu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	case "tools/list":
		if isNotification {
			return
		}
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			m := map[string]any{
				"name": t.Name, "title": t.Title, "description": t.Description,
				"inputSchema": t.InputSchema, "annotations": t.Annotations,
			}
			list = append(list, m)
		}
		s.reply(req.ID, map[string]any{"tools": list})
	case "tools/call":
		if isNotification {
			return
		}
		s.callTool(ctx, req)
	default:
		if !isNotification {
			s.fail(req.ID, codeMethodNotFound, "method not found: "+req.Method)
		}
	}
}

func (s *Server) initialize(req rpcRequest) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	_ = json.Unmarshal(req.Params, &p)
	proto := LatestProtocol
	for _, v := range supportedProtocols {
		if v == p.ProtocolVersion {
			proto = v
		}
	}
	s.protocol.Store(proto)
	if p.ClientInfo.Name != "" {
		s.clientName.Store(p.ClientInfo.Name)
	}
	s.reply(req.ID, map[string]any{
		"protocolVersion": proto,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": s.info.Name, "version": s.info.Version},
		"instructions": "mypack turns websites into desktop apps. Typical flow: project_create (draft) -> " +
			"project_validate / build_preflight -> build_start -> build_status -> artifact_list. " +
			"Runtime downloads and other sensitive changes need the user's approval in the MyPack app; " +
			"build_start then returns approval_required. Apps made from a JavaScript project folder need the project's build " +
			"script run first; that always needs the user's approval and is never run on an agent's request.",
	})
}

func (s *Server) callTool(ctx context.Context, req rpcRequest) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
		s.fail(req.ID, codeInvalidParams, "tools/call requires a tool name")
		return
	}
	t := s.byName[p.Name]
	if t == nil {
		s.fail(req.ID, codeInvalidParams, "unknown tool: "+p.Name)
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	key := string(req.ID)
	s.imu.Lock()
	s.inflight[key] = cancel
	s.imu.Unlock()
	defer func() {
		cancel()
		s.imu.Lock()
		delete(s.inflight, key)
		s.imu.Unlock()
	}()

	args := p.Arguments
	if len(bytes.TrimSpace(args)) == 0 || string(bytes.TrimSpace(args)) == "null" {
		args = json.RawMessage("{}")
	}
	data, err := func() (res any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		return t.Handler(cctx, args)
	}()
	s.audit(cctx, t.Name, args, err)
	s.reply(req.ID, toolResult(data, err))
}
