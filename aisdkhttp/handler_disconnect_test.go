package aisdkhttp

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type brokenResponseWriter struct {
	header http.Header
}

func (w *brokenResponseWriter) Header() http.Header { return w.header }
func (w *brokenResponseWriter) WriteHeader(int)     {}
func (w *brokenResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("broken pipe")
}

func TestClientDisconnectCancelsRun(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	run := func(ctx context.Context, _ []aikit.Message) (<-chan aikit.StepEvent, error) {
		events := make(chan aikit.StepEvent)
		go func() {
			defer close(events)
			defer close(stopped)
			events <- aikit.StepEvent{Type: aikit.StepEventStepStart}
			close(started)
			<-ctx.Done()
		}()
		return events, nil
	}

	server := httptest.NewServer(Handler(run))
	defer server.Close()
	request, err := http.NewRequest(
		http.MethodPost,
		server.URL,
		strings.NewReader(`{"messages":[]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("run did not start")
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("request context was not cancelled after client disconnect")
	}
}

func TestWriteFailureCancelsRunBeforeDraining(t *testing.T) {
	stopped := make(chan struct{})
	run := func(ctx context.Context, _ []aikit.Message) (<-chan aikit.StepEvent, error) {
		events := make(chan aikit.StepEvent)
		go func() {
			defer close(events)
			defer close(stopped)
			<-ctx.Done()
		}()
		return events, nil
	}

	Handler(run).ServeHTTP(
		&brokenResponseWriter{header: make(http.Header)},
		httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"messages":[]}`)),
	)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("write failure did not cancel the run")
	}
}
