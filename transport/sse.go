package transport

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// SSEFrame is one server-sent event. Multiple data fields are joined with a
// newline as required by the SSE specification.
type SSEFrame struct {
	Event string
	Data  string
	ID    string
	Retry time.Duration
}

// SSEReader decodes server-sent event frames without imposing a line-size cap.
type SSEReader struct {
	reader *bufio.Reader
}

// NewSSEReader creates an uncapped SSE frame reader.
func NewSSEReader(reader io.Reader) *SSEReader {
	return &SSEReader{reader: bufio.NewReader(reader)}
}

// ReadLine reads one uncapped SSE protocol line with its trailing CRLF removed.
// A final unterminated line is returned together with io.EOF.
func (r *SSEReader) ReadLine() (string, error) {
	line, err := r.reader.ReadString('\n')
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, err
}

// NextData reads the next SSE data field using the same uncapped line reader as
// [SSEReader.Next]. It accepts both "data: value" and "data:value".
//
// Provider APIs historically send one complete JSON object per data field and
// some omit the blank line required between formal SSE frames. NextData keeps
// that established tolerance while centralizing field parsing. Consumers that
// need full SSE frame semantics, including multiline data joining, use Next.
func (r *SSEReader) NextData() (string, error) {
	for {
		line, err := r.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}

		field, value, found := strings.Cut(line, ":")
		if found && strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		if field == "data" && found {
			return value, nil
		}
		if errors.Is(err, io.EOF) {
			return "", io.EOF
		}
	}
}

// Next reads the next SSE frame. It returns io.EOF only when no frame remains.
func (r *SSEReader) Next() (SSEFrame, error) {
	var frame SSEFrame
	var data []string
	var seenField bool

	for {
		line, err := r.ReadLine()
		if err != nil && !errors.Is(err, io.EOF) {
			return SSEFrame{}, err
		}

		if line == "" {
			if seenField {
				frame.Data = strings.Join(data, "\n")
				return frame, nil
			}
			if errors.Is(err, io.EOF) {
				return SSEFrame{}, io.EOF
			}
			continue
		}

		if !strings.HasPrefix(line, ":") {
			seenField = true
			field, value, found := strings.Cut(line, ":")
			if found && strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			switch field {
			case "event":
				frame.Event = value
			case "data":
				data = append(data, value)
			case "id":
				if !strings.ContainsRune(value, '\x00') {
					frame.ID = value
				}
			case "retry":
				if milliseconds, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil &&
					milliseconds >= 0 {
					frame.Retry = time.Duration(milliseconds) * time.Millisecond
				}
			}
		}

		if errors.Is(err, io.EOF) {
			if seenField {
				frame.Data = strings.Join(data, "\n")
				return frame, nil
			}
			return SSEFrame{}, io.EOF
		}
	}
}
