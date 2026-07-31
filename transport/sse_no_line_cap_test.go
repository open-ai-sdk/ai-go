package transport

import (
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSSEReader_SingleFrameHasNoLineCap(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString(
		[]byte(strings.Repeat("x", 2*1024*1024)),
	)
	reader := NewSSEReader(strings.NewReader("data: " + payload + "\n\n"))

	frame, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if frame.Data != payload {
		t.Fatalf("data length = %d, want %d", len(frame.Data), len(payload))
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Next() error = %v, want io.EOF", err)
	}
}

func TestSSEReader_DecodesFrameFieldsAndMultilineData(t *testing.T) {
	reader := NewSSEReader(strings.NewReader(
		": comment\n" +
			"id: evt-1\n" +
			"event: message\n" +
			"retry: 1500\n" +
			"data: first\n" +
			"data:second\n\n",
	))

	frame, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if frame.Event != "message" {
		t.Errorf("Event = %q, want message", frame.Event)
	}
	if frame.ID != "evt-1" {
		t.Errorf("ID = %q, want evt-1", frame.ID)
	}
	if frame.Data != "first\nsecond" {
		t.Errorf("Data = %q, want multiline data", frame.Data)
	}
	if frame.Retry.Milliseconds() != 1500 {
		t.Errorf("Retry = %v, want 1500ms", frame.Retry)
	}
}

func TestSSEReader_NextDataAcceptsNoSpaceAndConsecutiveFields(t *testing.T) {
	reader := NewSSEReader(strings.NewReader(
		"data:first\n" +
			"data: second\n",
	))

	first, err := reader.NextData()
	if err != nil || first != "first" {
		t.Fatalf("first data = %q, error = %v", first, err)
	}
	second, err := reader.NextData()
	if err != nil || second != "second" {
		t.Fatalf("second data = %q, error = %v", second, err)
	}
	if _, err := reader.NextData(); !errors.Is(err, io.EOF) {
		t.Fatalf("third NextData() error = %v, want io.EOF", err)
	}
}
