package openai

import (
	"errors"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestResponsesProviderOptionsAcceptJSONNumber(t *testing.T) {
	request, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
		ProviderOptions: map[string]any{
			"openai": map[string]any{"maxOutputTokens": float64(321)},
		},
	}, true)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	if request.MaxOutputTokens != 321 {
		t.Fatalf("MaxOutputTokens = %d, want 321", request.MaxOutputTokens)
	}
}

func TestResponsesProviderOptionsAcceptPDFDetail(t *testing.T) {
	request, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentPart{
			ai.DocumentDataPart([]byte("pdf"), "application/pdf", "report.pdf"),
		}}},
		ProviderOptions: map[string]any{
			"openai": map[string]any{"pdfDetail": "low"},
		},
	}, false)
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}
	if got := decodeInputPart(t, request.Input[0].Content[0]).Detail; got != "low" {
		t.Fatalf("Detail = %q, want low", got)
	}
}

func TestResponsesProviderOptionsRejectInvalidPDFDetail(t *testing.T) {
	_, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
		ProviderOptions: map[string]any{
			"openai": ProviderOptions{PDFDetail: "ultra"},
		},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "pdf detail") {
		t.Fatalf("error = %v, want invalid pdf detail", err)
	}
}

func TestResponsesProviderOptionsRejectInvalidValues(t *testing.T) {
	tests := []any{
		"wrong",
		map[string]any{"maxOutputTokens": "many"},
		map[string]any{"unknown": true},
	}
	for _, value := range tests {
		_, _, err := encodeRequest("gpt-test", ai.LanguageModelRequest{
			ProviderOptions: map[string]any{"openai": value},
		}, true)
		var optionErr *llm.ProviderOptionsError
		if !errors.As(err, &optionErr) {
			t.Fatalf("value %#v error = %v, want *ProviderOptionsError", value, err)
		}
	}
}

func TestChatProviderOptionsRejectResponsesOptions(t *testing.T) {
	_, err := parseChatProviderOptions(map[string]any{
		"openai": ProviderOptions{ReasoningEffort: "high"},
	})
	var optionErr *llm.ProviderOptionsError
	if !errors.As(err, &optionErr) {
		t.Fatalf("error = %v, want *ProviderOptionsError", err)
	}
}

func TestReasoningEffortOptionWorksForBothOpenAIAPIs(t *testing.T) {
	options := map[string]any{"openai": WithReasoningEffort("high")}
	responses, err := parseProviderOptions(options)
	if err != nil {
		t.Fatalf("responses parse error = %v", err)
	}
	chat, err := parseChatProviderOptions(options)
	if err != nil {
		t.Fatalf("chat parse error = %v", err)
	}
	if responses.ReasoningEffort != "high" || chat.ReasoningEffort != "high" {
		t.Fatalf("reasoning efforts = %q/%q, want high/high",
			responses.ReasoningEffort, chat.ReasoningEffort)
	}
}

func TestProviderOptionPointersAreAccepted(t *testing.T) {
	responses, err := parseProviderOptions(map[string]any{
		"openai": &ProviderOptions{MaxOutputTokens: 12},
	})
	if err != nil || responses.MaxOutputTokens != 12 {
		t.Fatalf("responses = %#v, error = %v", responses, err)
	}
	chat, err := parseChatProviderOptions(map[string]any{
		"openai": &ChatProviderOptions{ReasoningEffort: "medium"},
	})
	if err != nil || chat.ReasoningEffort != "medium" {
		t.Fatalf("chat = %#v, error = %v", chat, err)
	}
}
