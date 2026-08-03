package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/jsonclone"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

// GenerateContentResponse is the provider-native successful Gemini response.
// Raw retains the exact response JSON.
type GenerateContentResponse struct {
	ResponseID    string                     `json:"responseId,omitempty"`
	Candidates    []GenerateContentCandidate `json:"candidates"`
	UsageMetadata *GenerateContentUsage      `json:"usageMetadata,omitempty"`
	ModelVersion  string                     `json:"modelVersion,omitempty"`
	Raw           json.RawMessage            `json:"-"`
}

type GenerateContentCandidate struct {
	Content            *GenerateContent `json:"content,omitempty"`
	FinishReason       string           `json:"finishReason,omitempty"`
	Index              int              `json:"index,omitempty"`
	GroundingMetadata  json.RawMessage  `json:"groundingMetadata,omitempty"`
	CitationMetadata   map[string]any   `json:"citationMetadata,omitempty"`
	URLContextMetadata map[string]any   `json:"urlContextMetadata,omitempty"`
	SafetyRatings      []any            `json:"safetyRatings,omitempty"`
}

type GenerateContent struct {
	Role  string                `json:"role"`
	Parts []GenerateContentPart `json:"parts"`
}

type GenerateContentPart struct {
	Text             string                `json:"text,omitempty"`
	Thought          *bool                 `json:"thought,omitempty"`
	ThoughtSignature string                `json:"thoughtSignature,omitempty"`
	FunctionCall     *GenerateFunctionCall `json:"functionCall,omitempty"`
	InlineData       *GenerateInlineData   `json:"inlineData,omitempty"`
}

type GenerateFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type GenerateInlineData struct {
	MediaType string `json:"mimeType"`
	Data      string `json:"data"`
}

type GenerateContentUsage struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	ToolUsePromptTokenCount int `json:"toolUsePromptTokenCount"`
}

var _ llm.CompletionModel = (*NativeLanguageModel)(nil)

// Complete performs one non-streaming native generateContent request.
func (m *NativeLanguageModel) Complete(
	ctx context.Context,
	req llm.Request,
) (*llm.CompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, m.cfg.Timeout)
	defer cancel()
	if _, err := resolveProviderOptions(req.ProviderOptions); err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "gemini-native", err)
	}
	nativeReq := encodeNativeRequestForModel(m.modelID, req)
	opts := parseProviderOptions(req.ProviderOptions)
	toolResult := encodeNativeTools(req.Tools, req.ToolChoice, opts)
	nativeReq.Tools = toolResult.Tools
	nativeReq.ToolConfig = toolResult.ToolConfig
	body, err := json.Marshal(nativeReq)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorJSON, "complete", "gemini-native", err)
	}
	if m.clientErr != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "gemini-native", m.clientErr)
	}
	target := fmt.Sprintf("models/%s:generateContent", m.modelID)
	httpReq, err := m.client.NewRequest(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorRequest, "complete", "gemini-native", err)
	}
	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", "gemini-native", err)
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return nil, llm.WrapCompletionError(
			llm.CompletionErrorProvider,
			"complete",
			"gemini-native",
			transport.APIErrorFromResponse(ctx, "gemini-native", httpResp),
		)
	}
	defer httpResp.Body.Close()
	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorTransport, "complete", "gemini-native", err)
	}
	var native GenerateContentResponse
	if err := json.Unmarshal(raw, &native); err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorJSON, "complete", "gemini-native", err)
	}
	native.Raw = append(json.RawMessage(nil), raw...)
	response, err := normalizeGenerateContent(&native)
	if err != nil {
		return nil, llm.WrapCompletionError(llm.CompletionErrorResponse, "complete", "gemini-native", err)
	}
	return response, nil
}

func normalizeGenerateContent(native *GenerateContentResponse) (*llm.CompletionResponse, error) {
	if native == nil || len(native.Candidates) == 0 {
		return nil, fmt.Errorf("generateContent returned no candidates")
	}
	candidate := native.Candidates[0]
	if candidate.Content == nil {
		return nil, fmt.Errorf("generateContent returned no candidate content")
	}
	if candidate.Content.Role != "" && candidate.Content.Role != "model" {
		return nil, fmt.Errorf("generateContent returned role %q", candidate.Content.Role)
	}
	message := aikit.Message{ID: native.ResponseID, Role: aikit.RoleAssistant}
	var text, reasoning string
	var files []llm.GeneratedFile
	var warnings []aikit.Warning
	toolIndex := 0
	for _, part := range candidate.Content.Parts {
		if isUnknownNativeResponsePart(
			part.Text,
			part.Thought,
			part.FunctionCall != nil,
			part.InlineData != nil,
		) {
			warnings = append(warnings, unknownCandidatePartWarning())
			continue
		}
		if part.FunctionCall != nil {
			message.Content = append(message.Content, aikit.ContentPart{
				Type:       aikit.ContentPartTypeToolCall,
				ToolCallID: fmt.Sprintf("call_%d", toolIndex), ToolCallName: part.FunctionCall.Name,
				ToolCallArgs:     append(json.RawMessage(nil), part.FunctionCall.Args...),
				ThoughtSignature: part.ThoughtSignature,
			})
			toolIndex++
			continue
		}
		if part.Thought != nil && *part.Thought && part.Text != "" {
			reasoning += part.Text
			message.Content = append(message.Content, aikit.ContentPart{
				Type: aikit.ContentPartTypeReasoning, ReasoningText: part.Text,
				ThoughtSignature: part.ThoughtSignature,
			})
		} else if part.Text != "" {
			text += part.Text
			message.Content = append(message.Content, aikit.ContentPart{
				Type: aikit.ContentPartTypeText, Text: part.Text,
				ThoughtSignature: part.ThoughtSignature,
			})
		}
		if part.InlineData != nil {
			data, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				return nil, fmt.Errorf("decode inline data: %w", err)
			}
			files = append(files, llm.GeneratedFile{
				Data: append([]byte(nil), data...), MediaType: part.InlineData.MediaType,
			})
			message.Content = append(message.Content, aikit.ContentPart{
				Type: aikit.ContentPartTypeFile, Data: append([]byte(nil), data...),
				MediaType: part.InlineData.MediaType,
			})
		}
	}
	if len(message.Content) == 0 {
		return nil, fmt.Errorf("generateContent returned no assistant content")
	}
	finish, rawFinish := mapNativeFinishReason(candidate.FinishReason, toolIndex > 0)
	response := &llm.CompletionResponse{
		Message: message, MessageID: native.ResponseID, Text: text, Reasoning: reasoning,
		FinishReason: finish, RawFinishReason: rawFinish, Files: files, RawResponse: native,
		Warnings: warnings,
	}
	if native.UsageMetadata != nil {
		u := native.UsageMetadata
		response.Usage = *nativeUsageToAI(&nativeUsageMetadata{
			PromptTokenCount: u.PromptTokenCount, CandidatesTokenCount: u.CandidatesTokenCount,
			TotalTokenCount: u.TotalTokenCount, ThoughtsTokenCount: u.ThoughtsTokenCount,
			CachedContentTokenCount: u.CachedContentTokenCount,
			ToolUsePromptTokenCount: u.ToolUsePromptTokenCount,
		})
	}
	grounding := decodeNativeGroundingMetadata(candidate.GroundingMetadata)
	response.Sources = extractNativeGroundingSources(grounding, make(map[string]bool))
	if metadata := nativeGoogleMetadata(grounding, candidate); metadata != nil {
		response.ProviderMetadata = map[string]any{"google": metadata}
	}
	return response, nil
}

func isUnknownNativeResponsePart(text string, thought *bool, hasFunctionCall, hasInlineData bool) bool {
	return text == "" && thought == nil && !hasFunctionCall && !hasInlineData
}

func unknownCandidatePartWarning() aikit.Warning {
	return aikit.Warning{
		Type:    "unsupported-response-part",
		Setting: "candidateContentPart",
		Message: "gemini-native: unsupported candidate content part, skipping",
	}
}

func nativeGoogleMetadata(
	grounding *nativeGroundingMetadata,
	candidate GenerateContentCandidate,
) map[string]any {
	metadata := make(map[string]any)
	if grounding != nil {
		if len(grounding.WebSearchQueries) > 0 {
			metadata["webSearchQueries"] = append([]string(nil), grounding.WebSearchQueries...)
		}
		if len(grounding.ImageSearchQueries) > 0 {
			metadata["imageSearchQueries"] = append([]string(nil), grounding.ImageSearchQueries...)
		}
	}
	if candidate.CitationMetadata != nil {
		metadata["citationMetadata"] = jsonclone.Map(candidate.CitationMetadata)
	}
	if candidate.URLContextMetadata != nil {
		metadata["urlContextMetadata"] = jsonclone.Map(candidate.URLContextMetadata)
	}
	if len(candidate.SafetyRatings) > 0 {
		metadata["safetyRatings"] = jsonclone.Value(candidate.SafetyRatings)
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
