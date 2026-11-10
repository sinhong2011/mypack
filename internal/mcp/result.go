package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"mypack.dev/mypack/internal/appservice"
	"mypack.dev/mypack/internal/domain"
)

// toolError is a tool-level failure (reported with isError=true so the model
// can read it and adapt) as opposed to a JSON-RPC protocol error.
type toolError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
	Field       string `json:"field,omitempty"`
	Issues      any    `json:"issues,omitempty"`
}

func (e *toolError) Error() string { return e.Code + ": " + e.Message }

func argError(msg string) error {
	return &toolError{Code: "INVALID_ARGUMENTS", Message: msg,
		Remediation: "Check the tool's input schema and call it again."}
}

// toolResult renders a handler outcome as an MCP CallToolResult. Successful
// results carry both a JSON text block and structuredContent (an object).
func toolResult(data any, err error) map[string]any {
	if err != nil {
		te := normalizeError(err)
		return map[string]any{
			"isError":           true,
			"content":           []map[string]any{{"type": "text", "text": mustJSON(te)}},
			"structuredContent": map[string]any{"error": te},
		}
	}
	if data == nil {
		data = map[string]any{}
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": mustJSON(data)}},
		"structuredContent": data,
	}
}

func normalizeError(err error) *toolError {
	var te *toolError
	if errors.As(err, &te) {
		return te
	}
	var de *domain.Error
	if errors.As(err, &de) {
		out := &toolError{Code: de.Code, Message: de.Message, Remediation: de.Remediation, Field: de.Field}
		if is := appservice.ConfigIssues(de); len(is) > 0 {
			out.Issues = is
		}
		return out
	}
	if errors.Is(err, context.Canceled) {
		return &toolError{Code: "CANCELLED", Message: "The request was cancelled."}
	}
	return &toolError{Code: "INTERNAL", Message: err.Error()}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"code":"INTERNAL","message":"cannot encode result"}`
	}
	return string(b)
}

// audit records a tool call in the activity log (when the service supports
// it). Arguments are not stored: configs may hold sensitive values (spec §12.6).
func (s *Server) audit(ctx context.Context, tool string, args json.RawMessage, err error) {
	a, ok := s.svc.(appservice.Auditor)
	if !ok {
		return
	}
	res := "ok"
	if err != nil {
		res = "error: " + normalizeError(err).Code
	}
	var target struct {
		ID        string `json:"id"`
		ProjectID string `json:"projectId"`
		JobID     string `json:"jobId"`
	}
	_ = json.Unmarshal(args, &target)
	t := target.ID
	if t == "" {
		t = target.ProjectID
	}
	if t == "" {
		t = target.JobID
	}
	_ = a.RecordActivity(context.WithoutCancel(ctx), domain.Activity{
		Actor: s.actor(), Action: "mcp." + tool, Target: t, Result: res,
	})
}
