package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdk"
)

// fakeAgent is a minimal Agent test double: Stream returns a pre-built
// *StreamResult (or an error), Generate is never exercised by these tests.
type fakeAgent struct {
	sr  *StreamResult
	err error
}

func (f *fakeAgent) ID() string      { return "fake" }
func (f *fakeAgent) Tools() *ToolSet { return nil }
func (f *fakeAgent) Prompt(context.Context, string) (string, error) {
	return "", errors.New("fakeAgent.Prompt not implemented")
}

func (f *fakeAgent) Chat(context.Context, string, ...Message) (string, error) {
	return "", errors.New("fakeAgent.Chat not implemented")
}

func (f *fakeAgent) Generate(context.Context, ...Option) (*GenerateTextResult, error) {
	return nil, errors.New("fakeAgent.Generate not implemented")
}

func (f *fakeAgent) Stream(context.Context, ...Option) (*StreamResult, error) {
	return f.sr, f.err
}

func textOnlyStreamResult() *StreamResult {
	ch := make(chan aikit.StepEvent, 4)
	for _, event := range []aikit.StepEvent{
		{Type: aikit.StepEventStepStart},
		{Type: aikit.StepEventTextDelta, TextDelta: "hello"},
		{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		{Type: aikit.StepEventDone},
	} {
		ch <- event
	}
	close(ch)
	return NewStreamResult(ch)
}

// TestAgentStream_YieldsChunksFromAgentRun verifies AgentStream drives a's
// Stream() result through the same UI-message-stream chunk pipeline as
// ToUIMessageStream.
func TestAgentStream_YieldsChunksFromAgentRun(t *testing.T) {
	agent := &fakeAgent{sr: textOnlyStreamResult()}

	ch, err := AgentStream(context.Background(), agent, "msg-1")
	if err != nil {
		t.Fatalf("AgentStream: %v", err)
	}
	chunks := drainAgentChunks(ch)

	if _, ok := findAgentChunk(chunks, aisdk.ChunkStart); !ok {
		t.Error("expected start chunk")
	}
	if _, ok := findAgentChunk(chunks, aisdk.ChunkTextDelta); !ok {
		t.Error("expected text-delta chunk")
	}
	if _, ok := findAgentChunk(chunks, aisdk.ChunkFinish); !ok {
		t.Error("expected finish chunk")
	}
}

func drainAgentChunks(ch <-chan aisdk.Chunk) []aisdk.Chunk {
	var chunks []aisdk.Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func findAgentChunk(chunks []aisdk.Chunk, typ string) (aisdk.Chunk, bool) {
	for _, chunk := range chunks {
		if chunk.Type == typ {
			return chunk, true
		}
	}
	return aisdk.Chunk{}, false
}

// TestAgentStream_PropagatesAgentError verifies an error from a.Stream is
// returned directly rather than surfacing as an empty channel.
func TestAgentStream_PropagatesAgentError(t *testing.T) {
	wantErr := errors.New("boom")
	agent := &fakeAgent{err: wantErr}

	ch, err := AgentStream(context.Background(), agent, "msg-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if ch != nil {
		t.Error("expected nil channel on error")
	}
}

// TestPipeAgentStream_SetsHeadersAndWritesSSE verifies PipeAgentStream sets
// the SSE headers and writes the agent's run as SSE-encoded chunks.
func TestPipeAgentStream_SetsHeadersAndWritesSSE(t *testing.T) {
	agent := &fakeAgent{sr: textOnlyStreamResult()}
	rr := httptest.NewRecorder()

	if err := PipeAgentStream(context.Background(), rr, agent, "msg-http"); err != nil {
		t.Fatalf("PipeAgentStream: %v", err)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if accel := rr.Header().Get("x-accel-buffering"); accel != "no" {
		t.Errorf("x-accel-buffering = %q, want no", accel)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"type":"start"`) {
		t.Errorf("expected start chunk in body, got: %s", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Errorf("expected [DONE] terminator in body, got: %s", body)
	}
}

// failingResponseWriter simulates a disconnected client: every Write fails.
type failingResponseWriter struct {
	header http.Header
}

func (f *failingResponseWriter) Header() http.Header { return f.header }
func (f *failingResponseWriter) WriteHeader(int)     {}
func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write: broken pipe")
}

// TestPipeAgentStream_ReturnsErrorOnWriteFailure proves a disconnected client
// becomes a non-nil error rather than a silently abandoned run.
func TestPipeAgentStream_ReturnsErrorOnWriteFailure(t *testing.T) {
	agent := &fakeAgent{sr: textOnlyStreamResult()}
	w := &failingResponseWriter{header: make(http.Header)}

	err := PipeAgentStream(context.Background(), w, agent, "msg-disconnect")
	if err == nil {
		t.Fatal("expected a non-nil error when the client write fails")
	}
}

// TestAgentHandler_DecodesEnvelopeAndStreams verifies the handler decodes a
// ChatRequestEnvelope body, forwards its messages to the agent via
// WithMessages, and streams the SSE response.
func TestAgentHandler_DecodesEnvelopeAndStreams(t *testing.T) {
	var capturedMessages []Message
	agent := &capturingAgent{sr: textOnlyStreamResult(), onGenerateOpts: func(req *GenerateTextRequest) {
		capturedMessages = req.Messages
	}}

	body := `{"messages":[{"role":"user","content":"hi there"}]}`
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(body))
	rr := httptest.NewRecorder()

	AgentHandler(agent).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"start"`) {
		t.Errorf("expected start chunk in body, got: %s", rr.Body.String())
	}
	if len(capturedMessages) != 1 || capturedMessages[0].Role != RoleUser {
		t.Fatalf("expected one user message forwarded to the agent, got: %+v", capturedMessages)
	}
}

// TestAgentHandler_InvalidBodyReturns400 verifies a malformed request body is
// rejected before the agent ever runs.
func TestAgentHandler_InvalidBodyReturns400(t *testing.T) {
	agent := &fakeAgent{sr: textOnlyStreamResult()}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader("not json"))
	rr := httptest.NewRecorder()

	AgentHandler(agent).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// capturingAgent records the merged GenerateTextRequest opts would apply to,
// so AgentHandler's message-forwarding can be asserted without depending on
// AgentStream's internals.
type capturingAgent struct {
	sr             *StreamResult
	onGenerateOpts func(*GenerateTextRequest)
}

func (c *capturingAgent) ID() string      { return "capturing" }
func (c *capturingAgent) Tools() *ToolSet { return nil }
func (c *capturingAgent) Prompt(context.Context, string) (string, error) {
	return "", errors.New("capturingAgent.Prompt not implemented")
}

func (c *capturingAgent) Chat(context.Context, string, ...Message) (string, error) {
	return "", errors.New("capturingAgent.Chat not implemented")
}

func (c *capturingAgent) Generate(context.Context, ...Option) (*GenerateTextResult, error) {
	return nil, errors.New("capturingAgent.Generate not implemented")
}

func (c *capturingAgent) Stream(_ context.Context, opts ...Option) (*StreamResult, error) {
	req := &GenerateTextRequest{}
	for _, o := range opts {
		o(req)
	}
	if c.onGenerateOpts != nil {
		c.onGenerateOpts(req)
	}
	return c.sr, nil
}
