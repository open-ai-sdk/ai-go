package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

type builderTestModel struct{ id string }

func (m *builderTestModel) ModelID() string { return m.id }

func (*builderTestModel) Stream(context.Context, llm.Request) (<-chan aikit.StreamEvent, error) {
	return nil, errors.New("unexpected model call")
}

type builderTestApprover struct{}

func (builderTestApprover) RequestApproval(context.Context, ApprovalRequest) (ApprovalResponse, error) {
	return ApprovalResponse{}, nil
}

func TestBuilderBuildDefaults(t *testing.T) {
	model := &builderTestModel{id: "model-id"}
	built, err := New(model).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if built.ID() != "model-id" {
		t.Fatalf("ID() = %q, want model-id", built.ID())
	}
	if built.MaxTurns() != 1 {
		t.Fatalf("MaxTurns() = %d, want 1", built.MaxTurns())
	}
	if built.config.maxParallelTools != 1 || built.config.parallelToolExecution {
		t.Fatalf(
			"tool concurrency = (%d, %v), want (1, false)",
			built.config.maxParallelTools,
			built.config.parallelToolExecution,
		)
	}
	if built.config.model != model {
		t.Fatal("Build changed the configured model")
	}
}

func TestBuilderBuildRejectsStaticConfigurationErrors(t *testing.T) {
	var typedNilModel *builderTestModel
	model := &builderTestModel{id: "model-id"}
	shortKey := make([]byte, minApprovalKeyBytes-1)
	dangerousTools := testToolSet([]ToolDefinition{{Name: "dangerous"}}, nil)

	tests := []struct {
		name  string
		build Builder
		field string
		cause error
	}{
		{name: "nil model", build: New(nil), field: "Model", cause: errNilModel},
		{name: "typed nil model", build: New(typedNilModel), field: "Model", cause: errNilModel},
		{name: "zero max turns", build: New(model).MaxTurns(0), field: "MaxTurns", cause: errInvalidMaxTurns},
		{
			name: "zero concurrency", build: New(model).ToolConcurrency(0),
			field: "ToolConcurrency", cause: errInvalidConcurrency,
		},
		{
			name:  "short approval key",
			build: New(model).ApprovalKey(shortKey),
			field: "ApprovalKey",
			cause: errInvalidApprovalKey,
		},
		{
			name: "suspending approval without key",
			build: New(model).Tools(dangerousTools).ToolApproval(map[string]ApprovalPolicy{
				"dangerous": func(string, string) bool { return true },
			}),
			field: "ApprovalKey",
			cause: errMissingApprovalKey,
		},
		{
			name: "nil approval policy",
			build: New(model).Tools(dangerousTools).ToolApproval(map[string]ApprovalPolicy{
				"dangerous": nil,
			}).Approver(builderTestApprover{}),
			field: "ToolApproval[dangerous]",
			cause: errNilApprovalPolicy,
		},
		{
			name:  "active tool without registry",
			build: New(model).ActiveTools("missing"),
			field: "ActiveTools",
			cause: errUnknownActiveTool,
		},
		{
			name: "approval for unknown tool",
			build: New(model).ToolApproval(map[string]ApprovalPolicy{
				"missing": func(string, string) bool { return true },
			}).Approver(builderTestApprover{}),
			field: "ToolApproval[missing]",
			cause: errUnknownActiveTool,
		},
		{
			name:  "unsupported output type",
			build: New(model).Output(llm.OutputSchema{Type: "yaml"}),
			field: "Output",
			cause: &StructuredOutputError{Kind: StructuredOutputErrorKindValidation},
		},
		{
			name: "invalid output schema",
			build: New(model).Output(llm.OutputSchema{
				Type: "object", Schema: map[string]any{"type": 42},
			}),
			field: "Output",
			cause: &StructuredOutputError{Kind: StructuredOutputErrorKindValidation},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.build.Build()
			var buildErr *BuildError
			if !errors.As(err, &buildErr) {
				t.Fatalf("Build() error = %T %v, want *BuildError", err, err)
			}
			if buildErr.Field != test.field {
				t.Fatalf("BuildError.Field = %q, want %q", buildErr.Field, test.field)
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("Build() error = %v, want cause %v", err, test.cause)
			}
		})
	}
}

func TestBuilderBuildValidatesToolsAndChoice(t *testing.T) {
	model := &builderTestModel{id: "model-id"}
	lookup, err := tool.NewDynamic(
		"lookup",
		"",
		map[string]any{"type": "object"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	)
	if err != nil {
		t.Fatalf("NewDynamic() error = %v", err)
	}
	set, err := tool.NewSet(lookup)
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}

	for _, test := range []struct {
		name  string
		build Builder
		field string
	}{
		{
			name:  "unknown active tool",
			build: New(model).Tools(set).ActiveTools("missing"),
			field: "ActiveTools",
		},
		{
			name: "specific tool excluded",
			build: New(model).Tools(set).ActiveTools().ToolChoice(aikit.ToolChoice{
				Type: "tool", ToolName: "lookup",
			}),
			field: "ToolChoice",
		},
		{
			name:  "required with no active tools",
			build: New(model).Tools(set).ActiveTools().ToolChoice(aikit.ToolChoice{Type: "required"}),
			field: "ToolChoice",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.build.Build()
			var buildErr *BuildError
			if !errors.As(err, &buildErr) || buildErr.Field != test.field {
				t.Fatalf("Build() error = %#v, want BuildError field %q", err, test.field)
			}
		})
	}

	if _, err := New(model).Tools(set).ActiveTools("lookup").ToolChoice(aikit.ToolChoice{
		Type: "tool", ToolName: "lookup",
	}).Build(); err != nil {
		t.Fatalf("Build(valid tools) error = %v", err)
	}
}

func TestBuilderBuildDefensivelyCopiesMutableConfiguration(t *testing.T) {
	model := &builderTestModel{id: "model-id"}
	temperature := float32(0.25)
	topK := 4
	settings := llm.CallSettings{
		Temperature:   &temperature,
		TopK:          &topK,
		StopSequences: []string{"first"},
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string"},
		},
	}
	providerOptions := map[string]any{
		"provider": map[string]any{"nested": []any{"original"}},
	}
	toolsContext := aikit.ToolsContext{"lookup": "original"}
	runtimeContext := aikit.RuntimeContext{"request": "original"}
	approval := map[string]ApprovalPolicy{
		"dangerous": func(string, string) bool { return true },
	}
	key := make([]byte, minApprovalKeyBytes)
	for i := range key {
		key[i] = byte(i + 1)
	}

	builder := New(model).
		Tools(testToolSet([]ToolDefinition{{Name: "dangerous"}}, nil)).
		Settings(settings).
		Output(llm.OutputSchema{Type: "json", Schema: schema}).
		ProviderOptions(providerOptions).
		ToolsContext(toolsContext).
		RuntimeContext(runtimeContext).
		ToolApproval(approval).
		ApprovalKey(key).
		ActiveTools()

	// Mutations after fluent calls must not alter the builder.
	temperature = 0.9
	topK = 99
	settings.StopSequences[0] = "mutated"
	schema["type"] = "array"
	schema["properties"].(map[string]any)["city"].(map[string]any)["type"] = "number"
	providerOptions["provider"].(map[string]any)["nested"].([]any)[0] = "mutated"
	toolsContext["lookup"] = "mutated"
	runtimeContext["request"] = "mutated"
	delete(approval, "dangerous")
	key[0] = 0

	built, err := builder.Approver(builderTestApprover{}).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got := *built.config.settings.Temperature; got != 0.25 {
		t.Fatalf("temperature = %v, want 0.25", got)
	}
	if got := *built.config.settings.TopK; got != 4 {
		t.Fatalf("topK = %v, want 4", got)
	}
	if !reflect.DeepEqual(built.config.settings.StopSequences, []string{"first"}) {
		t.Fatalf("stop sequences = %#v", built.config.settings.StopSequences)
	}
	if got := built.config.output.Schema["properties"].(map[string]any)["city"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("schema city type = %v, want string", got)
	}
	if got := built.config.providerOptions["provider"].(map[string]any)["nested"].([]any)[0]; got != "original" {
		t.Fatalf("provider nested value = %v, want original", got)
	}
	if built.config.toolsContext["lookup"] != "original" || built.config.runtimeContext["request"] != "original" {
		t.Fatalf("contexts = %#v %#v", built.config.toolsContext, built.config.runtimeContext)
	}
	if built.config.toolApproval["dangerous"] == nil {
		t.Fatal("approval policy was not copied")
	}
	if built.config.approvalKey[0] != 1 {
		t.Fatalf("approval key first byte = %d, want 1", built.config.approvalKey[0])
	}
	if built.config.activeTools == nil {
		t.Fatal("explicit empty active-tools allowlist collapsed to nil")
	}

	// A Runner snapshot can mutate its containers without changing the Agent.
	runConfig := cloneConfig(built.config)
	runConfig.settings.StopSequences[0] = "runner"
	runConfig.output.Schema["type"] = "runner"
	runConfig.providerOptions["provider"].(map[string]any)["nested"].([]any)[0] = "runner"
	runConfig.toolsContext["lookup"] = "runner"
	runConfig.approvalKey[0] = 0
	if built.config.settings.StopSequences[0] != "first" ||
		built.config.output.Schema["type"] != "object" ||
		built.config.providerOptions["provider"].(map[string]any)["nested"].([]any)[0] != "original" ||
		built.config.toolsContext["lookup"] != "original" ||
		built.config.approvalKey[0] != 1 {
		t.Fatal("cloneConfig mutation changed immutable Agent state")
	}
}

func TestBuilderBuildSnapshotsToolRegistry(t *testing.T) {
	model := &builderTestModel{id: "model-id"}
	lookup, err := tool.NewDynamic(
		"lookup",
		"original",
		map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
		func(context.Context, json.RawMessage) (json.RawMessage, error) { return json.RawMessage(`{}`), nil },
	)
	if err != nil {
		t.Fatalf("NewDynamic() error = %v", err)
	}
	set, err := tool.NewSet(lookup)
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}
	built, err := New(model).Tools(set).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if built.config.tools == set {
		t.Fatal("Build retained the caller's tool registry pointer")
	}

	// Read views are independent and cannot alter the Agent's canonical
	// registry or a per-Runner clone.
	definitions := set.DefinitionsSnapshot()
	definitions[0].Name = "mutated"
	definition, exists := built.config.tools.Lookup("lookup")
	if !exists || definition.Description != "original" {
		t.Fatalf("Agent tool = (%#v, %v), want original lookup", definition, exists)
	}
	runConfig := cloneConfig(built.config)
	if runConfig.tools == built.config.tools {
		t.Fatal("cloneConfig retained the Agent's tool registry pointer")
	}
}

func TestBuilderValueBranchesDoNotShareMaps(t *testing.T) {
	model := &builderTestModel{id: "model-id"}
	base := New(model).ProviderOptionsJSON("shared", map[string]any{"value": "base"})
	left, err := base.ProviderOptionsJSON("left", map[string]any{"value": 1}).Build()
	if err != nil {
		t.Fatalf("left Build() error = %v", err)
	}
	right, err := base.ProviderOptionsJSON("right", map[string]any{"value": 2}).Build()
	if err != nil {
		t.Fatalf("right Build() error = %v", err)
	}
	if _, exists := left.config.providerOptions["right"]; exists {
		t.Fatal("right branch mutated left branch")
	}
	if _, exists := right.config.providerOptions["left"]; exists {
		t.Fatal("left branch mutated right branch")
	}
}
