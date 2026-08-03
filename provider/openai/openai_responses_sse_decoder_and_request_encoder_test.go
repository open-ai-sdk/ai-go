package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/transport"
)

// helpers

type inputPartWire struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url"`
	FileID   string `json:"file_id"`
	FileData string `json:"file_data"`
	FileURL  string `json:"file_url"`
	Filename string `json:"filename"`
	Detail   string `json:"detail"`
}

func decodeInputPart(t *testing.T, part inputPart) inputPartWire {
	t.Helper()
	raw, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("json.Marshal(input part) error = %v", err)
	}
	var wire inputPartWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("json.Unmarshal(input part) error = %v", err)
	}
	return wire
}

func inputPartJSON(t *testing.T, part inputPart) map[string]any {
	t.Helper()
	raw, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("json.Marshal(input part) error = %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("json.Unmarshal(input part) error = %v", err)
	}
	return object
}

func streamFromString(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

func collectEvents(body io.ReadCloser) []ai.StreamEvent {
	var out []ai.StreamEvent
	for e := range responsesTestStream(context.Background(), body) {
		out = append(out, e)
	}
	return out
}

func responsesTestStream(
	ctx context.Context,
	body io.ReadCloser,
) <-chan ai.StreamEvent {
	return transport.Stream(
		ctx,
		&http.Response{Body: body},
		func(
			ctx context.Context,
			reader *transport.SSEReader,
			events chan<- ai.StreamEvent,
		) error {
			return decodeResponsesSSEStream(ctx, reader, events)
		},
	)
}

// SSE decoder tests

func TestOpenAISSE_TextDelta(t *testing.T) {
	sse := `data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}
data: {"type":"response.output_text.delta","delta":"Hello"}
data: {"type":"response.output_text.delta","delta":" world"}
data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}
`
	events := collectEvents(streamFromString(sse))

	textDeltas := 0
	usageCount := 0
	finishCount := 0
	for _, e := range events {
		switch e.Type {
		case ai.StreamEventTextDelta:
			textDeltas++
		case ai.StreamEventUsage:
			usageCount++
			if e.Usage.InputTokens != 5 {
				t.Errorf("expected 5 input tokens, got %d", e.Usage.InputTokens)
			}
		case ai.StreamEventFinish:
			finishCount++
			if e.FinishReason != ai.FinishReasonStop {
				t.Errorf("expected FinishReasonStop, got %q", e.FinishReason)
			}
			if e.RawFinishReason != "completed" {
				t.Errorf("expected RawFinishReason=completed, got %q", e.RawFinishReason)
			}
		}
	}
	if textDeltas != 2 {
		t.Errorf("expected 2 text deltas, got %d", textDeltas)
	}
	if usageCount != 1 {
		t.Errorf("expected 1 usage event, got %d", usageCount)
	}
	if finishCount != 1 {
		t.Errorf("expected 1 finish event, got %d", finishCount)
	}
}

func TestOpenAISSE_ReasoningDelta(t *testing.T) {
	sse := `data: {"type":"response.reasoning_summary_text.delta","delta":"thinking..."}
data: {"type":"response.completed","response":{"id":"resp_2","status":"completed"}}
`
	events := collectEvents(streamFromString(sse))

	hasReasoning := false
	for _, e := range events {
		if e.Type == ai.StreamEventReasoningDelta {
			hasReasoning = true
			if e.TextDelta != "thinking..." {
				t.Errorf("unexpected reasoning delta: %q", e.TextDelta)
			}
		}
	}
	if !hasReasoning {
		t.Error("expected a reasoning delta event")
	}
}

func TestOpenAISSE_FunctionCallDelta(t *testing.T) {
	sse := `data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_abc","name":"search","arguments":""}}
data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"q\":\""}
data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"hello\"}"}
data: {"type":"response.completed","response":{"id":"resp_3","status":"completed"}}
`
	events := collectEvents(streamFromString(sse))

	toolEvents := 0
	for _, e := range events {
		if e.Type == ai.StreamEventToolCallDelta {
			toolEvents++
			if e.ToolCallID != "call_abc" {
				t.Errorf("expected call_id=call_abc, got %q", e.ToolCallID)
			}
		}
	}
	if toolEvents < 2 {
		t.Errorf("expected at least 2 tool call delta events, got %d", toolEvents)
	}
}

func TestOpenAISSE_SourceEvents(t *testing.T) {
	sse := `data: {"type":"response.web_search_call.sources","sources":[{"type":"url","id":"src_1","url":"https://example.com","title":"Example"}]}
data: {"type":"response.completed","response":{"id":"resp_4","status":"completed"}}
`
	events := collectEvents(streamFromString(sse))

	sourceCount := 0
	for _, e := range events {
		if e.Type == ai.StreamEventSource {
			sourceCount++
			if e.Source == nil {
				t.Error("expected non-nil Source")
				continue
			}
			if e.Source.URL != "https://example.com" {
				t.Errorf("expected URL=https://example.com, got %q", e.Source.URL)
			}
			if e.Source.Title != "Example" {
				t.Errorf("expected Title=Example, got %q", e.Source.Title)
			}
		}
	}
	if sourceCount != 1 {
		t.Errorf("expected 1 source event, got %d", sourceCount)
	}
}

func TestOpenAISSE_ProviderMetadataOnFinish(t *testing.T) {
	sse := `data: {"type":"response.completed","response":{"id":"resp_meta","status":"completed"}}
`
	events := collectEvents(streamFromString(sse))

	for _, e := range events {
		if e.Type == ai.StreamEventFinish {
			if e.ProviderMetadata == nil {
				t.Fatal("expected ProviderMetadata on finish event")
			}
			openai, ok := e.ProviderMetadata["openai"].(map[string]any)
			if !ok {
				t.Fatalf("expected openai metadata map, got %T", e.ProviderMetadata["openai"])
			}
			if openai["responseId"] != "resp_meta" {
				t.Errorf("expected responseId=resp_meta, got %v", openai["responseId"])
			}
			return
		}
	}
	t.Error("no finish event found")
}

func TestOpenAISSE_ErrorEvent(t *testing.T) {
	sse := `data: {"type":"error","error":{"code":"rate_limit","message":"Too many requests"}}
`
	events := collectEvents(streamFromString(sse))

	for _, e := range events {
		if e.Type == ai.StreamEventError {
			if e.Error == nil {
				t.Error("expected non-nil error")
			}
			return
		}
	}
	t.Error("expected an error event")
}

func TestOpenAISSE_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sse := `data: {"type":"response.output_text.delta","delta":"text"}
`
	var events []ai.StreamEvent
	for e := range responsesTestStream(ctx, streamFromString(sse)) {
		events = append(events, e)
	}

	for _, e := range events {
		if e.Type == ai.StreamEventError {
			return
		}
	}
	t.Error("expected an error event on context cancellation")
}

// Request encoder tests

func TestEncodeRequest_InstructionsAndUserMessage(t *testing.T) {
	req := ai.LanguageModelRequest{
		Instructions: "You are helpful",
		Messages:     []ai.Message{ai.UserMessage("hello")},
	}
	r, _, err := encodeRequest("gpt-4o", req, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Model != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %q", r.Model)
	}
	if !r.Stream {
		t.Error("expected stream=true")
	}
	if len(r.Input) != 2 {
		t.Fatalf("expected 2 input items (system + user), got %d", len(r.Input))
	}
	if r.Input[0].Role != "system" {
		t.Errorf("expected first item role=system, got %q", r.Input[0].Role)
	}
	if r.Input[1].Role != "user" {
		t.Errorf("expected second item role=user, got %q", r.Input[1].Role)
	}
}

func TestEncodeRequest_SystemMessageInHistory(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{
			ai.SystemMessage("You are helpful"),
			ai.UserMessage("hello"),
		},
	}
	r, _, err := encodeRequest("gpt-4o", req, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Input) != 2 {
		t.Fatalf("expected 2 input items (system + user), got %d", len(r.Input))
	}
	systemPart := decodeInputPart(t, r.Input[0].Content[0])
	if r.Input[0].Role != "system" || len(r.Input[0].Content) != 1 ||
		systemPart.Type != "input_text" || systemPart.Text != "You are helpful" {
		t.Errorf("unexpected system input: %#v", r.Input[0])
	}
	if r.Input[1].Role != "user" {
		t.Errorf("expected second item role=user, got %q", r.Input[1].Role)
	}
}

func TestEncodeRequest_SystemMessageSkipsUnsupportedContent(t *testing.T) {
	req := ai.LanguageModelRequest{Messages: []ai.Message{
		{Role: ai.RoleSystem, Content: []ai.ContentPart{
			ai.TextPart("You are helpful"),
			{Type: ai.ContentPartTypeFile, Data: []byte("unsupported"), MediaType: "text/plain"},
		}},
		ai.UserMessage("hello"),
	}}
	r, warnings, err := encodeRequest("gpt-4o", req, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Input) != 2 || len(r.Input[0].Content) != 1 ||
		decodeInputPart(t, r.Input[0].Content[0]).Text != "You are helpful" || r.Input[1].Role != "user" {
		t.Fatalf("input = %#v", r.Input)
	}
	if len(warnings) != 1 || warnings[0].Setting != string(ai.ContentPartTypeFile) ||
		!strings.Contains(warnings[0].Message, "unsupported system content") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestEncodeRequest_PreviousResponseID(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("continue")},
		ProviderOptions: map[string]any{
			"openai": ProviderOptions{PreviousResponseID: "resp_abc"},
		},
	}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.PreviousResponseID != "resp_abc" {
		t.Errorf("expected previous_response_id=resp_abc, got %q", r.PreviousResponseID)
	}
}

func TestEncodeRequest_ReasoningOptions(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("think")},
		ProviderOptions: map[string]any{
			"openai": ProviderOptions{
				ReasoningEffort:  "high",
				ReasoningSummary: "detailed",
			},
		},
	}
	r, _, err := encodeRequest("o3", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Reasoning == nil {
		t.Fatal("expected reasoning config, got nil")
	}
	if r.Reasoning.Effort != "high" {
		t.Errorf("expected effort=high, got %q", r.Reasoning.Effort)
	}
	if r.Reasoning.Summary != "detailed" {
		t.Errorf("expected summary=detailed, got %q", r.Reasoning.Summary)
	}
}

func TestEncodeRequest_WebSearch(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("search something")},
		ProviderOptions: map[string]any{
			"openai": ProviderOptions{EnableWebSearch: true, IncludeSources: true},
		},
	}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasWebSearch := false
	for _, tool := range r.Tools {
		if tool.Type == "web_search_preview" {
			hasWebSearch = true
		}
	}
	if !hasWebSearch {
		t.Error("expected web_search_preview tool")
	}

	hasInclude := false
	for _, inc := range r.Include {
		if inc == "web_search_call.action.sources" {
			hasInclude = true
		}
	}
	if !hasInclude {
		t.Error("expected web_search_call.action.sources in include list")
	}
}

func TestEncodeRequest_FileIDInput(t *testing.T) {
	msg := ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.TextPart("what is in this file?"),
			{Type: ai.ContentPartTypeFile, FileID: "file-abc123"},
		},
	}
	req := ai.LanguageModelRequest{Messages: []ai.Message{msg}}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(r.Input))
	}
	parts := r.Input[0].Content
	if len(parts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(parts))
	}
	filePart := decodeInputPart(t, parts[1])
	if filePart.FileID != "file-abc123" {
		t.Errorf("expected file_id=file-abc123, got %q", filePart.FileID)
	}
}

func TestEncodeRequest_ImageDataInput(t *testing.T) {
	data := []byte("fake-png-bytes")
	msg := ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.TextPart("describe this image"),
			ai.ImageDataPart(data, "image/png"),
		},
	}
	req := ai.LanguageModelRequest{Messages: []ai.Message{msg}}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := r.Input[0].Content
	imgPart := decodeInputPart(t, parts[1])
	if imgPart.Type != "input_image" {
		t.Errorf("expected type=input_image, got %q", imgPart.Type)
	}
	if imgPart.ImageURL == "" {
		t.Error("expected ImageURL to be set with data URI")
	}
	if imgPart.FileID != "" {
		t.Errorf("expected FileID to be empty, got %q", imgPart.FileID)
	}
}

func TestEncodeRequest_ImageFileIDInput(t *testing.T) {
	msg := ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.TextPart("describe this image"),
			ai.ImageFileIDPart("file-img123"),
		},
	}
	req := ai.LanguageModelRequest{Messages: []ai.Message{msg}}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := r.Input[0].Content
	imgPart := decodeInputPart(t, parts[1])
	if imgPart.Type != "input_image" {
		t.Errorf("expected type=input_image, got %q", imgPart.Type)
	}
	if imgPart.FileID != "file-img123" {
		t.Errorf("expected FileID=file-img123, got %q", imgPart.FileID)
	}
}

func TestEncodeRequest_FileDataInput(t *testing.T) {
	data := []byte("pdf-content")
	msg := ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.TextPart("summarize this file"),
			ai.FileDataPart(data, "application/pdf", "doc.pdf"),
		},
	}
	req := ai.LanguageModelRequest{Messages: []ai.Message{msg}}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := r.Input[0].Content
	filePart := decodeInputPart(t, parts[1])
	if filePart.Type != "input_file" {
		t.Errorf("expected type=input_file, got %q", filePart.Type)
	}
	if filePart.FileData != "data:application/pdf;base64,cGRmLWNvbnRlbnQ=" {
		t.Errorf("FileData = %q, want PDF data URI", filePart.FileData)
	}
	if filePart.FileURL != "" {
		t.Errorf("FileURL = %q, want empty for inline data", filePart.FileURL)
	}
	if filePart.Filename != "doc.pdf" {
		t.Errorf("expected Filename=doc.pdf, got %q", filePart.Filename)
	}
}

func TestEncodeRequest_PDFSourcesAndDetail(t *testing.T) {
	tests := []struct {
		name string
		part ai.ContentPart
		want func(t *testing.T, part inputPartWire)
	}{
		{
			name: "inline data",
			part: ai.DocumentDataPart([]byte("pdf"), "application/pdf", "report.pdf"),
			want: func(t *testing.T, part inputPartWire) {
				t.Helper()
				if part.FileData != "data:application/pdf;base64,cGRm" {
					t.Errorf("FileData = %q", part.FileData)
				}
			},
		},
		{
			name: "external URL",
			part: ai.DocumentURLPart("https://example.com/report.pdf", "application/pdf"),
			want: func(t *testing.T, part inputPartWire) {
				t.Helper()
				if part.FileURL != "https://example.com/report.pdf" {
					t.Errorf("FileURL = %q", part.FileURL)
				}
			},
		},
		{
			name: "file ID",
			part: ai.DocumentFileIDPart("file-pdf123", "application/pdf", "report.pdf"),
			want: func(t *testing.T, part inputPartWire) {
				t.Helper()
				if part.FileID != "file-pdf123" {
					t.Errorf("FileID = %q", part.FileID)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := ai.LanguageModelRequest{
				Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentPart{
					ai.TextPart("summarize"), test.part,
				}}},
				ProviderOptions: map[string]any{
					"openai": ProviderOptions{PDFDetail: "high"},
				},
			}
			encoded, _, err := encodeRequest("gpt-test", req, false)
			if err != nil {
				t.Fatalf("encodeRequest() error = %v", err)
			}
			part := decodeInputPart(t, encoded.Input[0].Content[1])
			if part.Type != "input_file" || part.Detail != "high" {
				t.Fatalf("part = %#v, want input_file detail=high", part)
			}
			test.want(t, part)
		})
	}
}

func TestEncodeRequest_InlinePDFUsesFileDataJSONKey(t *testing.T) {
	req := ai.LanguageModelRequest{Messages: []ai.Message{{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.DocumentDataPart([]byte("pdf"), "application/pdf", "report.pdf"),
		},
	}}}
	encoded, _, err := encodeRequest("gpt-test", req, false)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	raw, err := json.Marshal(encoded.Input[0].Content[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	jsonText := string(raw)
	if !strings.Contains(jsonText, `"file_data":"data:application/pdf;base64,cGRm"`) {
		t.Fatalf("JSON = %s, want file_data", jsonText)
	}
	if strings.Contains(jsonText, `"file_url"`) {
		t.Fatalf("JSON = %s, must not contain file_url for inline data", jsonText)
	}
}

func TestEncodeRequest_InputFileSourcesAreMutuallyExclusive(t *testing.T) {
	tests := []struct {
		name      string
		part      ai.ContentPart
		wantField string
		wantValue string
		forbidden []string
	}{
		{
			name:      "URL omits filename and other sources",
			part:      ai.DocumentURLPart("https://example.com/report.pdf", "application/pdf", "report.pdf"),
			wantField: "file_url",
			wantValue: "https://example.com/report.pdf",
			forbidden: []string{"file_id", "file_data", "filename"},
		},
		{
			name:      "file ID omits filename and other sources",
			part:      ai.DocumentFileIDPart("file-pdf123", "application/pdf", "report.pdf"),
			wantField: "file_id",
			wantValue: "file-pdf123",
			forbidden: []string{"file_url", "file_data", "filename"},
		},
		{
			name:      "inline data keeps filename",
			part:      ai.DocumentDataPart([]byte("pdf"), "application/pdf", "report.pdf"),
			wantField: "file_data",
			wantValue: "data:application/pdf;base64,cGRm",
			forbidden: []string{"file_url", "file_id"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
				Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentPart{test.part}}},
			}, false)
			if err != nil {
				t.Fatalf("encodeRequest() error = %v", err)
			}
			wire := inputPartJSON(t, encoded.Input[0].Content[0])
			if got := wire[test.wantField]; got != test.wantValue {
				t.Fatalf("%s = %v, want %q; wire = %#v", test.wantField, got, test.wantValue, wire)
			}
			for _, field := range test.forbidden {
				if _, exists := wire[field]; exists {
					t.Fatalf("wire contains mutually exclusive field %q: %#v", field, wire)
				}
			}
		})
	}
}

func TestEncodeRequest_RejectsInvalidMediaSourceCount(t *testing.T) {
	tests := []struct {
		name string
		part ai.ContentPart
		want string
	}{
		{
			name: "document without source",
			part: ai.ContentPart{Type: ai.ContentPartTypeDocument, MediaType: "application/pdf"},
			want: "got 0",
		},
		{
			name: "document with data and URL",
			part: ai.ContentPart{
				Type: ai.ContentPartTypeDocument, MediaType: "application/pdf",
				Data: []byte("pdf"), FileURL: "https://example.com/report.pdf",
			},
			want: "got 2",
		},
		{
			name: "image with file ID and URL",
			part: ai.ContentPart{
				Type: ai.ContentPartTypeImage, MediaType: "image/png",
				FileID: "file-image", FileURL: "https://example.com/image.png",
			},
			want: "got 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
				Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentPart{test.part}}},
			}, false)
			if err == nil || !strings.Contains(err.Error(), "exactly one source") ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want source-count error containing %q", err, test.want)
			}
		})
	}
}

func TestEncodeRequest_RichToolResultWireJSON(t *testing.T) {
	request := ai.LanguageModelRequest{Messages: []ai.Message{{
		Role: ai.RoleTool,
		Content: []ai.ContentPart{ai.RichToolResultPart(
			"call-1",
			"inspect",
			ai.TextToolResultContent("plain text"),
			ai.JSONToolResultContent(json.RawMessage(`{"ok":true}`)),
			ai.ImageToolResultContent([]byte("img"), "image/png"),
		)},
	}}}

	encoded, _, err := encodeRequest("gpt-test", request, false)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	if len(encoded.Input) != 1 {
		t.Fatalf("input length = %d, want 1", len(encoded.Input))
	}
	raw, err := json.Marshal(encoded.Input[0])
	if err != nil {
		t.Fatalf("json.Marshal(input item) error = %v", err)
	}
	want := `{"type":"function_call_output","call_id":"call-1","output":[{"type":"input_text","text":"plain text"},{"type":"input_text","text":"{\"ok\":true}"},{"type":"input_image","image_url":"data:image/png;base64,aW1n"}]}`
	if string(raw) != want {
		t.Fatalf("wire JSON = %s, want %s", raw, want)
	}
}

func TestEncodeRequest_PDFDetailIsOmittedForNonPDF(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentPart{
			ai.DocumentDataPart([]byte("notes"), "text/plain", "notes.txt"),
		}}},
		ProviderOptions: map[string]any{
			"openai": ProviderOptions{PDFDetail: "high"},
		},
	}
	encoded, _, err := encodeRequest("gpt-test", req, false)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	if got := decodeInputPart(t, encoded.Input[0].Content[0]).Detail; got != "" {
		t.Fatalf("Detail = %q, want empty for non-PDF input", got)
	}
}

func TestEncodeRequest_RejectsAudioAndVideoInput(t *testing.T) {
	tests := []struct {
		name string
		part ai.ContentPart
	}{
		{name: "audio", part: ai.AudioDataPart([]byte("wav"), "audio/wav")},
		{name: "video", part: ai.VideoDataPart([]byte("mp4"), "video/mp4")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
				Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentPart{
					test.part,
				}}},
			}, false)
			if err == nil || !strings.Contains(err.Error(), "does not support "+test.name+" input") {
				t.Fatalf("error = %v, want unsupported %s input", err, test.name)
			}
		})
	}
}

func TestEncodeRequest_StopSequencesWarning(t *testing.T) {
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{ai.UserMessage("hi")},
		Settings: ai.CallSettings{StopSequences: []string{"stop"}},
	}
	_, warnings, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, w := range warnings {
		if w.Type == "unsupported-setting" && w.Setting == "stopSequences" {
			found = true
		}
	}
	if !found {
		t.Error("expected unsupported-setting warning for stopSequences")
	}
}

// An image URL must be encoded as input_image. Before media-type routing
// replaced the old type discriminator, an ImageURLPart carried no media type
// and fell through to the input_file branch, so the model never saw an image.
func TestEncodeRequest_ImageURLInput(t *testing.T) {
	msg := ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.TextPart("describe this image"),
			ai.ImageURLPart("https://example.com/img.png"),
		},
	}
	req := ai.LanguageModelRequest{Messages: []ai.Message{msg}}
	r, _, err := encodeRequest("gpt-4o", req, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	imgPart := decodeInputPart(t, r.Input[0].Content[1])
	if imgPart.Type != "input_image" {
		t.Errorf("expected type=input_image, got %q", imgPart.Type)
	}
	if imgPart.ImageURL != "https://example.com/img.png" {
		t.Errorf("unexpected ImageURL: %q", imgPart.ImageURL)
	}
}
