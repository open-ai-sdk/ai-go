package extractor

import (
	"context"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type testPerson struct {
	Name string `json:"name"`
}

type testModel struct{ instructions string }

func (*testModel) ModelID() string { return "extractor-test" }
func (m *testModel) Stream(_ context.Context, request llm.Request) (<-chan aikit.StreamEvent, error) {
	m.instructions = request.Instructions
	ch := make(chan aikit.StreamEvent, 2)
	ch <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: `{"name":"Ada"}`}
	ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
	close(ch)
	return ch, nil
}

func TestBuilderAppliesInstructionsAndContext(t *testing.T) {
	model := &testModel{}
	value, err := New[testPerson](model).Instructions("Extract a person.").Context("Names use title case.").Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	result, err := value.ExtractWithUsage(context.Background(), "Ada is a programmer.")
	if err != nil {
		t.Fatalf("ExtractWithUsage() error = %v", err)
	}
	if result.Object.Name != "Ada" || model.instructions != "Extract a person.\n\nContext:\nNames use title case." {
		t.Fatalf("result=%#v instructions=%q", result, model.instructions)
	}
}
