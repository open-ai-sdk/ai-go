package agent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestRunnerRunAndStreamExposeEquivalentSuccessfulTurn(t *testing.T) {
	usage := &aikit.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}
	script := []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "same "},
		{Type: aikit.StreamEventTextDelta, TextDelta: "answer"},
		{Type: aikit.StreamEventUsage, Usage: usage},
		{Type: aikit.StreamEventFinish, MessageID: "same-id", FinishReason: aikit.FinishReasonStop},
	}
	runAgent := mustRunnerAgent(t, &runnerScriptModel{scripts: [][]aikit.StreamEvent{script}})
	result, err := runAgent.Runner().Prompt("question").Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	streamAgent := mustRunnerAgent(t, &runnerScriptModel{scripts: [][]aikit.StreamEvent{script}})
	sequence, err := streamAgent.Runner().Prompt("question").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var text, messageID string
	var finish aikit.FinishReason
	var streamUsage aikit.Usage
	var eventTypes []aikit.StepEventType
	for event, eventErr := range sequence {
		if eventErr != nil {
			t.Fatalf("stream iteration error = %v", eventErr)
		}
		eventTypes = append(eventTypes, event.Type)
		switch event.Type {
		case aikit.StepEventTextDelta:
			text += event.TextDelta
		case aikit.StepEventUsage:
			if event.Usage != nil {
				streamUsage = *event.Usage
			}
		case aikit.StepEventStepEnd:
			messageID, finish = event.MessageID, event.FinishReason
		}
	}
	wantTypes := []aikit.StepEventType{
		aikit.StepEventStepStart,
		aikit.StepEventTextDelta,
		aikit.StepEventTextDelta,
		aikit.StepEventUsage,
		aikit.StepEventStepEnd,
		aikit.StepEventDone,
	}
	if !reflect.DeepEqual(eventTypes, wantTypes) {
		t.Fatalf("stream event types = %v, want %v", eventTypes, wantTypes)
	}
	if text != result.Text || messageID != result.MessageID || finish != result.FinishReason ||
		!reflect.DeepEqual(streamUsage, result.Usage) {
		t.Fatalf("stream aggregate = (%q, %q, %q, %#v), Run result = %#v", text, messageID, finish, streamUsage, result)
	}
}

type cancellationModel struct {
	cancelled chan struct{}
	once      sync.Once
}

func (m *cancellationModel) ModelID() string { return "cancellation-model" }

func (m *cancellationModel) Stream(ctx context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	stream := make(chan aikit.StreamEvent)
	go func() {
		defer close(stream)
		select {
		case stream <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: "provider-started"}:
		case <-ctx.Done():
			m.once.Do(func() { close(m.cancelled) })
			return
		}
		<-ctx.Done()
		m.once.Do(func() { close(m.cancelled) })
	}()
	return stream, nil
}

func TestRunnerStreamIteratorIsSingleUseAndEarlyBreakCancelsProvider(t *testing.T) {
	model := &cancellationModel{cancelled: make(chan struct{})}
	built := mustRunnerAgent(t, model)
	sequence, err := built.Runner().Prompt("start").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	for range sequence {
		break
	}
	select {
	case <-model.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("provider context was not cancelled after early iterator break")
	}

	var secondErr error
	var secondEvents int
	for _, err := range sequence {
		secondEvents++
		secondErr = err
	}
	if secondEvents != 1 || !errors.Is(secondErr, agent.ErrStreamUsed) {
		t.Fatalf("second iteration = (%d events, %v), want one ErrStreamUsed", secondEvents, secondErr)
	}
}

type concurrentEchoModel struct {
	mu       sync.Mutex
	requests []llm.Request
}

func (m *concurrentEchoModel) ModelID() string { return "concurrent-echo" }

func (m *concurrentEchoModel) Stream(_ context.Context, request llm.Request) (<-chan aikit.StreamEvent, error) {
	snapshot := cloneRunnerRequest(request)
	m.mu.Lock()
	m.requests = append(m.requests, snapshot)
	m.mu.Unlock()

	last := snapshot.Messages[len(snapshot.Messages)-1]
	if len(last.Content) == 0 {
		return nil, errors.New("missing prompt content")
	}
	text := last.Content[0].Text
	stream := make(chan aikit.StreamEvent, 2)
	stream <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: text}
	stream <- aikit.StreamEvent{Type: aikit.StreamEventFinish, MessageID: "id-" + text, FinishReason: aikit.FinishReasonStop}
	close(stream)
	return stream, nil
}

func TestAgentRunnerSnapshotsAreDefensiveAndSafeForConcurrentRuns(t *testing.T) {
	model := &concurrentEchoModel{}
	providerOptions := map[string]any{"provider": map[string]any{"mode": "original"}}
	stopSequences := []string{"original-stop"}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
		return builder.
			Instructions("original instructions").
			ProviderOptions(providerOptions).
			StopSequences(stopSequences...)
	})
	providerOptions["provider"].(map[string]any)["mode"] = "mutated"
	stopSequences[0] = "mutated-stop"

	const runs = 16
	texts := make(chan string, runs)
	errs := make(chan error, runs)
	var wait sync.WaitGroup
	for i := 0; i < runs; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			prompt := fmt.Sprintf("prompt-%02d", i)
			result, err := built.Runner().Prompt(prompt).Run(context.Background())
			if err != nil {
				errs <- err
				return
			}
			texts <- result.Text
		}(i)
	}
	wait.Wait()
	close(errs)
	close(texts)
	for err := range errs {
		t.Errorf("concurrent Run() error = %v", err)
	}
	if t.Failed() {
		return
	}
	gotTexts := make([]string, 0, runs)
	for text := range texts {
		gotTexts = append(gotTexts, text)
	}
	sort.Strings(gotTexts)
	wantTexts := make([]string, runs)
	for i := range wantTexts {
		wantTexts[i] = fmt.Sprintf("prompt-%02d", i)
	}
	if !reflect.DeepEqual(gotTexts, wantTexts) {
		t.Fatalf("concurrent results = %v, want %v", gotTexts, wantTexts)
	}

	model.mu.Lock()
	requests := append([]llm.Request(nil), model.requests...)
	model.mu.Unlock()
	if len(requests) != runs {
		t.Fatalf("model requests = %d, want %d", len(requests), runs)
	}
	for _, request := range requests {
		if len(request.Messages) != 2 || !reflect.DeepEqual(request.Messages[0], aikit.SystemMessage("original instructions")) {
			t.Fatalf("request messages = %#v", request.Messages)
		}
		if !reflect.DeepEqual(request.Settings.StopSequences, []string{"original-stop"}) {
			t.Fatalf("request stop sequences = %#v", request.Settings.StopSequences)
		}
		provider, ok := request.ProviderOptions["provider"].(map[string]any)
		if !ok || provider["mode"] != "original" {
			t.Fatalf("request provider options = %#v", request.ProviderOptions)
		}
	}
}
