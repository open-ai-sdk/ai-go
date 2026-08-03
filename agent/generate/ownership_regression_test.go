package generate

import "testing"

func TestPruneMessagesClonesMessagesThatDoNotNeedPerMessagePruning(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Content: []ContentPart{ReasoningPart("old")}},
		{Role: RoleAssistant, Content: []ContentPart{ImageDataPart([]byte("image"), "image/png")}},
	}
	pruned := PruneMessages(messages, PruneOptions{Reasoning: PruneModeBeforeLastMsg})
	pruned[1].Content[0].Data[0] = 'X'
	if string(messages[1].Content[0].Data) != "image" {
		t.Fatal("unpruned message retained caller-owned content bytes")
	}
}

func TestFinishResponseKeepsResponseAndTranscriptIndependent(t *testing.T) {
	state := consumeState{
		initialMessages: []Message{UserMessage("prompt")},
		result: &GenerateTextResult{Steps: []StepOutput{{
			Content: []ContentPart{ImageDataPart([]byte("image"), "image/png")},
		}}},
	}
	state.finishResponse()
	if len(state.result.Response.Messages) != 1 || len(state.result.Transcript) != 2 {
		t.Fatalf("response=%#v transcript=%#v", state.result.Response, state.result.Transcript)
	}
	state.result.Response.Messages[0].Content[0].Data[0] = 'R'
	if string(state.result.Transcript[1].Content[0].Data) != "image" {
		t.Fatal("Transcript aliases Response.Messages")
	}
	state.result.Transcript[0].Content[0].Text = "changed"
	if state.initialMessages[0].Content[0].Text != "prompt" {
		t.Fatal("Transcript aliases initial messages")
	}
}

func TestPreludeToolResultPreservesIndependentTypedContent(t *testing.T) {
	result := ToolResult{
		ID: "call-1", Name: "image", Output: "fallback",
		Content: []ToolResultContent{ImageToolResultContent([]byte("pixels"), "image/png")},
	}
	messages := responseMessagesWithPrelude([]ToolResult{result}, nil, nil)
	content := messages[0].Content[0].ToolResultContent
	if len(content) != 1 || content[0].Type != ToolResultContentTypeImage {
		t.Fatalf("typed content = %#v", content)
	}
	content[0].Data[0] = 'X'
	if string(result.Content[0].Data) != "pixels" {
		t.Fatal("prelude response aliases tool-result content")
	}
}

func TestToolResultContentAliasesAreComplete(t *testing.T) {
	if ToolResultContentTypeJSON == "" || ToolResultContentTypeImage == "" {
		t.Fatal("JSON/image tool-result aliases must be exported from generate")
	}
}
