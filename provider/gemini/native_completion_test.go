package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestNativeCompleteRetainsRawMessageIDAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test:generateContent" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		const payload = `{"responseId":"resp_1","modelVersion":"gemini-test","candidates":[` +
			`{"content":{"role":"model","parts":[` +
			`{"thought":true,"text":"think","thoughtSignature":"sig"},{"text":"answer"},` +
			`{"functionCall":{"name":"lookup","args":{"q":1}}}]},"finishReason":"STOP",` +
			`"citationMetadata":{"source":"original"},` +
			`"urlContextMetadata":{"url":"original"},"safetyRatings":[{"rating":"safe"}]}],` +
			`"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,` +
			`"totalTokenCount":8,"thoughtsTokenCount":1,"cachedContentTokenCount":2,` +
			`"toolUsePromptTokenCount":2}}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	model := NewNativeLanguageModel("gemini-test", Config{
		BaseURL: server.URL + "/v1beta", HTTPClient: server.Client(),
	})
	response, err := model.Complete(context.Background(), llm.Request{Messages: []aikit.Message{
		aikit.UserMessage("question"),
	}})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response.MessageID != "resp_1" || response.Text != "answer" || response.Reasoning != "think" ||
		response.Usage.ToolUsePromptTokens != 2 || response.Usage.OutputTokenDetails.ReasoningTokens != 1 {
		t.Fatalf("response = %#v", response)
	}
	raw, ok := llm.RawResponseAs[*GenerateContentResponse](response)
	if !ok || raw.ResponseID != "resp_1" || len(raw.Raw) == 0 {
		t.Fatalf("raw = %#v, ok=%v", raw, ok)
	}
	google := response.ProviderMetadata["google"].(map[string]any)
	raw.Candidates[0].CitationMetadata["source"] = "raw-mutated"
	if got := google["citationMetadata"].(map[string]any)["source"]; got != "original" {
		t.Fatalf("normalized metadata aliases raw response: %v", got)
	}
	google["urlContextMetadata"].(map[string]any)["url"] = "normalized-mutated"
	if got := raw.Candidates[0].URLContextMetadata["url"]; got != "original" {
		t.Fatalf("raw response aliases normalized metadata: %v", got)
	}
}

func TestNormalizeGenerateContentWarnsForUnknownCandidatePart(t *testing.T) {
	response, err := normalizeGenerateContent(&GenerateContentResponse{
		Candidates: []GenerateContentCandidate{{
			Content: &GenerateContent{Role: "model", Parts: []GenerateContentPart{
				{Text: "answer"},
				{},
			}},
			FinishReason: "STOP",
		}},
	})
	if err != nil {
		t.Fatalf("normalizeGenerateContent() error = %v", err)
	}
	if len(response.Warnings) != 1 || response.Warnings[0].Setting != "candidateContentPart" {
		t.Fatalf("warnings = %#v, want unknown candidate-part warning", response.Warnings)
	}
}
