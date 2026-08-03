package generate

import (
	"encoding/json"
	"testing"

	coreagent "github.com/open-ai-sdk/ai-go/agent"
)

func TestConsumePreservesMixedAssistantContentForContinuation(t *testing.T) {
	result := &GenerateTextResult{}
	state := consumeState{result: result}
	events := []StepEvent{
		{Type: StepEventStepStart},
		{Type: StepEventTextDelta, TextDelta: "Here is the image:"},
		{Type: StepEventFileDelta, FileData: []byte("png-data"), FileMediaType: "image/png"},
		{Type: StepEventTextDelta, TextDelta: " done"},
		{Type: StepEventStepEnd, FinishReason: FinishReasonStop},
	}
	for _, event := range events {
		if _, err := state.consume(event); err != nil {
			t.Fatalf("consume(%v): %v", event.Type, err)
		}
	}
	state.finishResponse()

	if len(result.Steps) != 1 || len(result.Steps[0].Files) != 1 {
		t.Fatalf("result lost generated file: %#v", result)
	}
	messages := result.Response.Messages
	if len(messages) != 1 || len(messages[0].Content) != 3 {
		t.Fatalf("continuation message = %#v", messages)
	}
	parts := messages[0].Content
	if parts[0].Text != "Here is the image:" || parts[1].Type != ContentPartTypeFile ||
		string(parts[1].Data) != "png-data" || parts[1].MediaType != "image/png" ||
		parts[2].Text != " done" {
		t.Fatalf("mixed content order = %#v", parts)
	}

	parts[1].Data[0] = 'X'
	if string(result.Steps[0].Files[0].Data) != "png-data" {
		t.Fatal("continuation message aliases Files data")
	}
}

func TestLifecycleCallbackResponsesPreserveMixedContent(t *testing.T) {
	var stepEvent StepEndEvent
	var endEvent EndEvent
	callbacks := lifecycleCallbacks(GenerateTextRequest{
		OnStepEnd: func(event StepEndEvent) {
			stepEvent = event
			// Callback payloads must not alias the state later used by OnEnd.
			event.Content[1].Data[0] = 'X'
			event.Files[0].Data[0] = 'Y'
		},
		OnEnd: func(event EndEvent) { endEvent = event },
	})
	for _, event := range []StepEvent{
		{Type: StepEventStepStart},
		{Type: StepEventTextDelta, TextDelta: "image:"},
		{Type: StepEventFileDelta, FileData: []byte("png"), FileMediaType: "image/png"},
		{Type: StepEventTextDelta, TextDelta: "done"},
		{Type: StepEventStepEnd},
	} {
		callbacks.OnChunk(event)
	}
	callbacks.OnStepEnd(coreagent.StepEndEvent{Text: "image:done", FinishReason: FinishReasonStop})
	callbacks.OnEnd(coreagent.EndEvent{
		Text:         "image:done",
		Steps:        []coreagent.StepResultInfo{{Text: "image:done", FinishReason: FinishReasonStop}},
		FinishReason: FinishReasonStop,
	})

	if len(stepEvent.Response.Messages) != 1 || len(stepEvent.Response.Messages[0].Content) != 3 ||
		len(stepEvent.Files) != 1 {
		t.Fatalf("step callback = %#v", stepEvent)
	}
	if len(endEvent.Response.Messages) != 1 || len(endEvent.Response.Messages[0].Content) != 3 ||
		string(endEvent.Response.Messages[0].Content[1].Data) != "png" ||
		len(endEvent.Steps) != 1 || len(endEvent.Steps[0].Files) != 1 {
		t.Fatalf("end callback = %#v", endEvent)
	}
}

func TestResponseMessagesReconcilesPartialContentWithToolCalls(t *testing.T) {
	step := StepOutput{
		Content: []ContentPart{{Type: ContentPartTypeFile, Data: []byte("image"), MediaType: "image/png"}},
		Text:    "caption",
		ToolCalls: []ToolCallOutput{{
			ID: "call-1", Name: "inspect", Args: json.RawMessage(`{"detail":"high"}`),
			ApprovalID: "approval-1", ApprovalSignature: "signature-1",
		}},
	}
	messages := ResponseMessagesForStep(step, nil)
	if len(messages) != 1 || len(messages[0].Content) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	parts := messages[0].Content
	if parts[0].Type != ContentPartTypeFile || parts[1].Text != "caption" ||
		parts[2].ToolCallID != "call-1" || parts[2].ToolApprovalID != "approval-1" ||
		parts[2].ToolApprovalSignature != "signature-1" {
		t.Fatalf("reconciled content = %#v", parts)
	}
}
