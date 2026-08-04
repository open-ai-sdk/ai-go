package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type extractionTestValue struct {
	Name string `json:"name"`
}

type extractionModel struct {
	calls     int
	responses []string
	errs      []error
}

func (*extractionModel) ModelID() string { return "extractor-test" }

func (m *extractionModel) Stream(_ context.Context, request llm.Request) (<-chan aikit.StreamEvent, error) {
	if request.Output == nil || request.Output.Schema == nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindPrompt,
			Reason: "missing strict output schema",
		}
	}
	if len(m.errs) > m.calls && m.errs[m.calls] != nil {
		m.calls++
		return nil, m.errs[m.calls-1]
	}
	text := m.responses[m.calls]
	m.calls++
	events := make(chan aikit.StreamEvent, 3)
	events <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: text}
	events <- aikit.StreamEvent{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{InputTokens: 1, OutputTokens: 2}}
	events <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
	close(events)
	return events, nil
}

func TestExtractorRetriesRetryableProviderFailures(t *testing.T) {
	model := &extractionModel{
		responses: []string{"", `{"name":"Ada"}`},
		errs:      []error{aikit.NewAPIError("test", 503, nil)},
	}
	extractor, err := NewExtractor[extractionTestValue](model, WithExtractorRetries(1))
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}
	result, err := extractor.ExtractWithUsage(context.Background(), "extract a name")
	if err != nil {
		t.Fatalf("ExtractWithUsage() error = %v", err)
	}
	if result.Object.Name != "Ada" || result.Attempts != 2 || model.calls != 2 {
		t.Fatalf("result = %#v, calls = %d", result, model.calls)
	}
}

func TestExtractionErrorWithoutCauseDoesNotPanic(t *testing.T) {
	if got := (&ExtractionError{}).Error(); got != "agent: extraction failed" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is((&ExtractionError{Cause: context.Canceled}), context.Canceled) {
		t.Fatal("ExtractionError did not retain its cause")
	}
}

func TestExtractorRetriesAndAccumulatesUsage(t *testing.T) {
	model := &extractionModel{responses: []string{`{"name":1}`, `prefix {"name":"Ada"} suffix`}}
	extractor, err := NewExtractor[extractionTestValue](model, WithExtractorRetries(1))
	if err != nil {
		t.Fatalf("NewExtractor() error = %v", err)
	}
	result, err := extractor.ExtractWithUsage(context.Background(), "extract a name")
	if err != nil {
		t.Fatalf("ExtractWithUsage() error = %v", err)
	}
	if result.Object.Name != "Ada" || result.Attempts != 2 || result.Usage.InputTokens != 2 || model.calls != 2 {
		t.Fatalf("result = %#v, calls = %d", result, model.calls)
	}
}

func TestRunObjectReturnsDecodedValueAndResult(t *testing.T) {
	model := &extractionModel{responses: []string{`{"name":"Grace"}`}}
	agent, err := New(model).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	result, err := RunObject[extractionTestValue](context.Background(), agent.Runner().Prompt("extract a name"))
	if err != nil {
		t.Fatalf("RunObject() error = %v", err)
	}
	if result.Object.Name != "Grace" || result.Result == nil ||
		string(result.Result.StructuredOutput) != `{"name":"Grace"}` {
		t.Fatalf("result = %#v", result)
	}
}
