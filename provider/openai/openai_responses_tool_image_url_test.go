package openai

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestResponsesRichToolImageURLPreservesOrderAndOptionalDetail(t *testing.T) {
	for _, detail := range []string{"", "auto"} {
		t.Run(detail, func(t *testing.T) {
			request := llm.Request{
				Messages: []aikit.Message{{Role: aikit.RoleTool, Content: []aikit.ContentPart{
					aikit.RichToolResultPart("call_1", "lookup",
						aikit.TextToolResultContent("before"),
						aikit.ImageURLToolResultContent("https://example.test/image.png", "image/png"),
						aikit.TextToolResultContent("after")),
				}}},
				ProviderOptions: map[string]any{"openai": ProviderOptions{ImageDetail: detail}},
			}
			encoded, warnings, err := encodeRequest("example-model", request, false)
			if err != nil || len(warnings) != 0 {
				t.Fatalf("encode: %v, %#v", err, warnings)
			}
			imageDetail := ""
			if detail != "" {
				imageDetail = `,"detail":"auto"`
			}
			assertResponsesInputJSON(
				t,
				encoded.Input,
				`[{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"before"},{"type":"input_image","image_url":"https://example.test/image.png"`+imageDetail+`},{"type":"input_text","text":"after"}]}]`,
			)
		})
	}
}
