package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPTransport_CloseCancelsInboundRequestAwaitingHeaders(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var startedOnce sync.Once
	var cancelledOnce sync.Once
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(
		_ http.ResponseWriter,
		req *http.Request,
	) {
		requests.Add(1)
		startedOnce.Do(func() { close(started) })
		<-req.Context().Done()
		cancelledOnce.Do(func() { close(cancelled) })
	}))
	defer server.Close()

	transport := NewHTTPTransport(HTTPTransportConfig{URL: server.URL})
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("inbound request did not start")
	}
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel inbound request awaiting headers")
	}

	time.Sleep(25 * time.Millisecond)
	if got := requests.Load(); got != 1 {
		t.Fatalf("inbound request count = %d, want 1", got)
	}
}
