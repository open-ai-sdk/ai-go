package uistream

import (
	"bytes"
	"net/http"
	"testing"
)

func TestSSEFramerWritesNamedAndUnnamedFrames(t *testing.T) {
	var output bytes.Buffer
	framer := SSEFramer{}
	if err := framer.WriteFrame(&output, Frame{Data: []byte(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := framer.WriteFrame(&output, Frame{Name: "update", Data: []byte(`{"n":1}`)}); err != nil {
		t.Fatal(err)
	}
	want := "data: {\"ok\":true}\n\nevent: update\ndata: {\"n\":1}\n\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestSSEFramerHeaders(t *testing.T) {
	header := make(http.Header)
	(SSEFramer{}).ApplyHeaders(header)
	if header.Get("Content-Type") != "text/event-stream" || header.Get("Cache-Control") != "no-cache" ||
		header.Get("Connection") != "keep-alive" {
		t.Fatalf("headers = %#v", header)
	}
}
