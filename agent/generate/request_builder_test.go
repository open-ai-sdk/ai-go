package generate

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type builderModel struct{}

func (builderModel) ModelID() string { return "builder" }

func (builderModel) Stream(
	context.Context,
	llm.Request,
) (<-chan aikit.StreamEvent, error) {
	channel := make(chan aikit.StreamEvent)
	close(channel)
	return channel, nil
}

type builderProviderOptions struct {
	Value string
}

func (builderProviderOptions) ProviderName() string { return "builder" }

func TestRequestBuilderAndExplicitStructProduceSameModelRequest(t *testing.T) {
	model := builderModel{}
	choice := ToolChoice{Type: "auto"}
	output := OutputObject(map[string]any{"type": "object"})
	settings := CallSettings{MaxTokens: 100}
	explicit := GenerateTextRequest{
		Model:        model,
		Instructions: "Be concise.",
		Messages:     []Message{UserMessage("Hello")},
		ToolChoice:   &choice,
		Output:       output,
		Settings:     settings,
		ProviderOptions: map[string]any{
			"builder": builderProviderOptions{Value: "typed"},
		},
		ToolsContext:   ToolsContext{"tool": "context"},
		RuntimeContext: RuntimeContext{"request": "context"},
	}
	built := NewRequest(model, "Hello").
		Instructions("Be concise.").
		ToolChoice(choice).
		Output(output).
		Settings(settings).
		With(builderProviderOptions{Value: "typed"}).
		ToolsContext(explicit.ToolsContext).
		RuntimeContext(explicit.RuntimeContext).
		Build()

	if got, want := runParams(built).Request, runParams(explicit).Request; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized requests differ\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestRequestBuilderSetsEveryGenerateTextKnob(t *testing.T) {
	model := builderModel{}
	messages := []Message{UserMessage("custom")}
	tools := &ToolSet{Definitions: []ToolDefinition{{Name: "lookup"}}}
	choice := ToolChoice{Type: "tool", ToolName: "lookup"}
	stopWhen := func(int, *StepResult) bool { return true }
	output := OutputObject(map[string]any{"type": "object"})
	prepare := func(PrepareStepContext) *PrepareStepResult { return nil }
	repair := func(context.Context, RepairToolCallInput) (*ToolCallOutput, error) {
		return nil, nil
	}
	approval := map[string]ToolApprovalFunc{
		"lookup": func(string, json.RawMessage) ApprovalDecision { return ApprovalRequired },
	}
	responder := func(
		context.Context,
		ToolApprovalRequest,
	) (ToolApprovalResponse, error) {
		return ToolApprovalResponse{Approved: true}, nil
	}
	onStepEnd := func(StepEndEvent) {}
	onEnd := func(EndEvent) {}
	onChunk := func(ChunkEvent) {}
	onError := func(error) {}
	smooth := NewSmoothStream()
	middleware := func(model LanguageModel) LanguageModel { return model }
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	request := NewRequest(nil, "initial").
		Model(model).
		Instructions("system").
		Messages(messages...).
		Tools(tools).
		ToolChoice(choice).
		StopWhen(stopWhen).
		Output(output).
		Settings(CallSettings{MaxTokens: 1, StopSequences: []string{"old"}}).
		Temperature(0.2).
		MaxTokens(200).
		TopP(0.8).
		TopK(40).
		Seed(7).
		StopSequences("END").
		MaxSteps(3).
		ProviderOptionsJSON("json", map[string]any{"value": "decoded"}).
		With(builderProviderOptions{Value: "typed"}).
		PrepareStep(prepare).
		RepairToolCall(repair).
		ActiveTools("lookup").
		ToolsContext(ToolsContext{"lookup": "tool-context"}).
		RuntimeContext(RuntimeContext{"request": "runtime-context"}).
		ToolApproval(approval).
		ToolApprovalResponder(responder).
		OnStepEnd(onStepEnd).
		OnEnd(onEnd).
		OnChunk(onChunk).
		OnError(onError).
		SmoothStream(smooth).
		Middlewares(middleware).
		ParallelToolExecution(true).
		MaxParallelTools(4).
		Logger(logger).
		TraceContent(true).
		Build()

	if request.Model.ModelID() != "builder" ||
		request.Instructions != "system" ||
		!reflect.DeepEqual(request.Messages, messages) ||
		request.Tools != tools ||
		!reflect.DeepEqual(request.ToolChoice, &choice) ||
		!request.StopWhen(1, &StepResult{}) ||
		request.Output != output ||
		request.Settings.Temperature == nil || *request.Settings.Temperature != 0.2 ||
		request.Settings.MaxTokens != 200 ||
		request.Settings.TopP == nil || *request.Settings.TopP != 0.8 ||
		request.Settings.TopK == nil || *request.Settings.TopK != 40 ||
		request.Settings.Seed == nil || *request.Settings.Seed != 7 ||
		!reflect.DeepEqual(request.Settings.StopSequences, []string{"END"}) ||
		request.MaxSteps != 3 {
		t.Fatalf("core builder fields were not applied: %#v", request)
	}
	if request.ProviderOptions["json"].(map[string]any)["value"] != "decoded" ||
		request.ProviderOptions["builder"].(builderProviderOptions).Value != "typed" ||
		request.PrepareStep == nil ||
		request.RepairToolCall == nil ||
		!reflect.DeepEqual(request.ActiveTools, []string{"lookup"}) ||
		request.ToolsContext["lookup"] != "tool-context" ||
		request.RuntimeContext["request"] != "runtime-context" ||
		request.ToolApproval["lookup"] == nil ||
		request.ToolApprovalResponder == nil ||
		request.OnStepEnd == nil ||
		request.OnEnd == nil ||
		request.OnChunk == nil ||
		request.OnError == nil ||
		request.SmoothStream != smooth ||
		len(request.Middlewares) != 1 ||
		!request.ParallelToolExecution ||
		request.MaxParallelTools != 4 ||
		request.Logger != logger ||
		!request.TraceContent {
		t.Fatal("execution builder fields were not applied")
	}
}

func TestRequestBuilderIgnoresTypedNilProviderOption(t *testing.T) {
	var option *builderProviderOptions
	request := NewRequest(builderModel{}, "").With(option).Build()
	if request.ProviderOptions != nil {
		t.Fatalf("ProviderOptions = %#v, want nil", request.ProviderOptions)
	}
}
