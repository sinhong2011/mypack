package companion

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrFrameTooLarge is returned when an incoming frame exceeds the limit.
var ErrFrameTooLarge = errors.New("companion: frame too large")

// ReadFrame reads one length-prefixed message. io.EOF means clean end of
// input. A frame over max returns ErrFrameTooLarge without reading the body.
func ReadFrame(r io.Reader, max int) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err // io.EOF when no bytes, ErrUnexpectedEOF when partial
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if uint64(n) > uint64(max) {
		return nil, ErrFrameTooLarge
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return buf, nil
}

// WriteFrame marshals v and writes it as one frame (header and body in a
// single Write). Bodies over max are refused.
func WriteFrame(w io.Writer, v interface{}, max int) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(body) > max {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(body))
	}
	out := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(out, uint32(len(body)))
	copy(out[4:], body)
	_, err = w.Write(out)
	return err
}
