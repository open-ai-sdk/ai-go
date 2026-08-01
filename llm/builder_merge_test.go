package llm_test

import (
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type testProviderOptions struct {
	Value string
}

func (testProviderOptions) ProviderName() string { return "test" }

func TestRequestBuilderMatchesExplicitRequest(t *testing.T) {
	tool := aikit.ToolDefinition{Name: "lookup"}
	choice := aikit.ToolChoice{Type: "tool", ToolName: "lookup"}
	output := llm.OutputSchema{Type: "object", Schema: map[string]any{"type": "object"}}
	expected := llm.Request{
		Instructions: "Be concise.",
		Messages: []aikit.Message{{
			Role:    aikit.RoleUser,
			Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: "Hello"}},
		}},
		Tools:      []aikit.ToolDefinition{tool},
		ToolChoice: &choice,
		Output:     &output,
		Settings: llm.CallSettings{
			Temperature:   float32Pointer(0.5),
			MaxTokens:     256,
			TopP:          float32Pointer(0.9),
			TopK:          intPointer(20),
			Seed:          intPointer(7),
			StopSequences: []string{"END"},
		},
		ProviderOptions: map[string]any{"test": testProviderOptions{Value: "typed"}},
		ToolsContext:    aikit.ToolsContext{"lookup": map[string]any{"tenant": "acme"}},
		RuntimeContext:  aikit.RuntimeContext{"requestID": "req-1"},
	}

	actual := llm.NewRequest("Hello").
		Instructions("Be concise.").
		Tools(tool).
		ToolChoice(choice).
		Output(output).
		Temperature(0.5).
		MaxTokens(256).
		TopP(0.9).
		TopK(20).
		Seed(7).
		StopSequences("END").
		With(testProviderOptions{Value: "typed"}).
		ToolsContext(expected.ToolsContext).
		RuntimeContext(expected.RuntimeContext).
		Build()

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Build() mismatch\nactual:   %#v\nexpected: %#v", actual, expected)
	}
}

func TestRequestBuilderCopiesOwnedSlices(t *testing.T) {
	defaults := llm.Request{
		Messages: []aikit.Message{{Role: aikit.RoleUser}},
		Settings: llm.CallSettings{StopSequences: []string{"default"}},
	}
	builder := llm.FromRequest(defaults)
	first := builder.StopSequences("first").Build()
	second := builder.StopSequences("second").Build()

	first.Settings.StopSequences[0] = "mutated"
	if got := second.Settings.StopSequences[0]; got != "second" {
		t.Fatalf("second build changed through first build: %q", got)
	}
	if got := defaults.Settings.StopSequences[0]; got != "default" {
		t.Fatalf("defaults mutated: %q", got)
	}
}

func TestRequestBuilderPreservesReferencedNestedValues(t *testing.T) {
	schema := map[string]any{"type": "object"}
	request := llm.NewRequest("").
		Output(llm.OutputSchema{Type: "object", Schema: schema}).
		Build()

	schema["type"] = "string"
	if got := request.Output.Schema["type"]; got != "string" {
		t.Fatalf("nested schema was unexpectedly deep-copied: %v", got)
	}
}

func TestRequestBuilderIgnoresTypedNilProviderOption(t *testing.T) {
	var option *testProviderOptions
	request := llm.NewRequest("").With(option).Build()
	if request.ProviderOptions != nil {
		t.Fatalf("ProviderOptions = %#v, want nil", request.ProviderOptions)
	}
}

func float32Pointer(value float32) *float32 { return &value }
func intPointer(value int) *int             { return &value }
