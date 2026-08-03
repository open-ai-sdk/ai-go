package aikit

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateMessageRoleContentMatrix(t *testing.T) {
	args := json.RawMessage(`{"city":"Hue"}`)
	parts := map[ContentPartType]ContentPart{
		ContentPartTypeText:                 TextPart("hello"),
		ContentPartTypeFile:                 FilePart("https://example.com/legacy", "application/pdf"),
		ContentPartTypeImage:                ImageURLPart("https://example.com/image.png"),
		ContentPartTypeAudio:                AudioURLPart("https://example.com/audio.mp3", "audio/mpeg"),
		ContentPartTypeDocument:             DocumentURLPart("https://example.com/doc.pdf", "application/pdf"),
		ContentPartTypeVideo:                VideoURLPart("https://example.com/video.mp4", "video/mp4"),
		ContentPartTypeToolCall:             ToolCallPart("call-1", "weather", args),
		ContentPartTypeToolResult:           ToolResultPart("call-1", "weather", `{"temp":30}`),
		ContentPartTypeToolApprovalResponse: ToolApprovalResponsePart("approval-1", "signature", true, ""),
		ContentPartTypeReasoning:            ReasoningPart("thinking"),
	}
	allowed := map[Role]map[ContentPartType]bool{
		RoleSystem: {ContentPartTypeText: true},
		RoleUser: {
			ContentPartTypeText: true, ContentPartTypeFile: true,
			ContentPartTypeImage: true, ContentPartTypeAudio: true,
			ContentPartTypeDocument: true, ContentPartTypeVideo: true,
			ContentPartTypeToolResult: true, ContentPartTypeToolApprovalResponse: true,
		},
		RoleAssistant: {
			ContentPartTypeText: true, ContentPartTypeFile: true, ContentPartTypeImage: true,
			ContentPartTypeToolCall: true, ContentPartTypeReasoning: true,
		},
		RoleTool: {ContentPartTypeToolResult: true},
	}

	for role, roleAllowed := range allowed {
		for kind, part := range parts {
			t.Run(string(role)+"/"+string(kind), func(t *testing.T) {
				err := (Message{Role: role, Content: []ContentPart{part}}).Validate()
				if roleAllowed[kind] && err != nil {
					t.Fatalf("expected valid message: %v", err)
				}
				if !roleAllowed[kind] && !errors.Is(err, ErrInvalidMessage) {
					t.Fatalf("expected ErrInvalidMessage, got %v", err)
				}
			})
		}
	}
}

func TestClonePreservesNilVersusEmptyBytes(t *testing.T) {
	if cloned := (ContentPart{}).Clone(); cloned.Data != nil {
		t.Fatalf("nil data became non-nil: %#v", cloned.Data)
	}
	cloned := (ContentPart{Data: make([]byte, 0)}).Clone()
	if cloned.Data == nil || len(cloned.Data) != 0 {
		t.Fatalf("non-nil empty data was not preserved: %#v", cloned.Data)
	}
	if media := ImageDataPart(make([]byte, 0), "image/png"); media.Data == nil {
		t.Fatal("media constructor collapsed non-nil empty data")
	}
	if cloned := (ContentPart{ToolCallArgs: make(json.RawMessage, 0)}).Clone(); cloned.ToolCallArgs == nil {
		t.Fatal("non-nil empty raw message became nil")
	}
}

func TestValidateMessageRejectsEmptyAndIncompleteCalls(t *testing.T) {
	tests := []Message{
		{Role: RoleUser},
		{
			Role: RoleAssistant,
			Content: []ContentPart{{
				Type: ContentPartTypeToolCall, ToolCallName: "lookup", ToolCallArgs: json.RawMessage(`{}`),
			}},
		},
		{
			Role: RoleAssistant,
			Content: []ContentPart{{
				Type: ContentPartTypeToolCall, ToolCallID: "call-1", ToolCallArgs: json.RawMessage(`{}`),
			}},
		},
		{
			Role: RoleAssistant,
			Content: []ContentPart{{
				Type: ContentPartTypeToolCall, ToolCallID: "call-1", ToolCallName: "lookup",
			}},
		},
		{
			Role: RoleTool,
			Content: []ContentPart{{
				Type: ContentPartTypeToolResult, ToolResultName: "lookup",
			}},
		},
		{ID: "assistant-id", Role: RoleUser, Content: []ContentPart{TextPart("no")}},
	}
	for i, message := range tests {
		if err := message.Validate(); !errors.Is(err, ErrInvalidMessage) {
			t.Fatalf("case %d: expected ErrInvalidMessage, got %v", i, err)
		}
	}
}

func TestValidateMessageMediaSourceRule(t *testing.T) {
	for _, kind := range []ContentPartType{ContentPartTypeFile, ContentPartTypeImage, ContentPartTypeAudio, ContentPartTypeDocument, ContentPartTypeVideo} {
		valid := ContentPart{Type: kind, FileURL: "https://example.com/media"}
		if err := (Message{Role: RoleUser, Content: []ContentPart{valid}}).Validate(); err != nil {
			t.Fatalf("%s URL source should be valid: %v", kind, err)
		}
		for _, invalid := range []ContentPart{
			{Type: kind},
			{Type: kind, FileURL: "https://example.com/media", Data: []byte("media")},
			{Type: kind, FileURL: "https://example.com/media", FileID: "file-1"},
		} {
			err := (Message{
				Role: RoleUser, Content: []ContentPart{invalid},
			}).Validate()
			if !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("%s ambiguous/missing source should fail, got %v", kind, err)
			}
		}
	}
}

func TestConstructorsAndAccessorsDeepCopy(t *testing.T) {
	data := []byte("image")
	args := json.RawMessage(`{"ok":true}`)
	message := Message{
		ID:   "assistant-message-1",
		Role: RoleAssistant,
		Content: []ContentPart{
			ImageDataPart(data, "image/png"),
			ToolCallPart("call-1", "lookup", args),
		},
	}
	data[0] = 'X'
	args[2] = 'X'
	if string(message.Content[0].Data) != "image" || string(message.Content[1].ToolCallArgs) != `{"ok":true}` {
		t.Fatal("constructors retained caller-owned bytes")
	}

	content, err := message.AssistantContent()
	if err != nil {
		t.Fatal(err)
	}
	content[0].Data[0] = 'Y'
	content[1].ToolCallArgs[2] = 'Y'
	if string(message.Content[0].Data) != "image" || string(message.Content[1].ToolCallArgs) != `{"ok":true}` {
		t.Fatal("accessor returned aliased nested bytes")
	}
	if _, err := message.UserContent(); !errors.Is(err, ErrWrongMessageRole) {
		t.Fatalf("expected wrong-role error, got %v", err)
	}

	clone := message.Clone()
	clone.Content[0].Data[0] = 'Z'
	if string(message.Content[0].Data) != "image" || clone.ID != message.ID {
		t.Fatal("message clone aliased data or lost assistant message ID")
	}
}

func TestToolResultTypedContentIsExplicitAndCopied(t *testing.T) {
	text := TextToolResultContent(`{"looks":"json"}`)
	if text.Type != ToolResultContentTypeText || text.Text != `{"looks":"json"}` || text.JSON != nil {
		t.Fatalf("JSON-looking text was reinterpreted: %+v", text)
	}

	raw := json.RawMessage(`{"structured":true}`)
	data := []byte("pixels")
	result := ToolResult{Content: []ToolResultContent{
		JSONToolResultContent(raw),
		ImageToolResultContent(data, "image/png"),
	}}
	raw[2] = 'X'
	data[0] = 'X'
	if string(result.Content[0].JSON) != `{"structured":true}` || string(result.Content[1].Data) != "pixels" {
		t.Fatal("typed constructors retained caller-owned bytes")
	}
	clone := result.Clone()
	clone.Content[0].JSON[2] = 'Y'
	clone.Content[1].Data[0] = 'Y'
	if string(result.Content[0].JSON) != `{"structured":true}` || string(result.Content[1].Data) != "pixels" {
		t.Fatal("tool result clone retained aliases")
	}
	parsed, err := ParseToolResultJSON(`{"parsed":true}`)
	if err != nil || parsed.Type != ToolResultContentTypeJSON {
		t.Fatalf("explicit JSON parsing failed: %+v, %v", parsed, err)
	}
	if _, err := ParseToolResultJSON(`{not-json}`); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestTerminalEventsCarryAssistantMessageID(t *testing.T) {
	stream := StreamEvent{Type: StreamEventFinish, MessageID: "assistant-message-1"}
	step := StepEvent{Type: StepEventDone, MessageID: stream.MessageID}
	if stream.Type != StreamEventFinish || step.MessageID != "assistant-message-1" {
		t.Fatalf("message ID was lost: %+v", step)
	}
}
