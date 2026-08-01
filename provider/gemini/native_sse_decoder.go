// ai-go: file-length-justification: keeps the stateful Gemini SSE grammar and event assembly in one decoder.
package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/transport"
)

// --------------------------------------------------------------------
// Native Gemini SSE response types
// --------------------------------------------------------------------

type nativeSSEChunk struct {
	Candidates    []nativeCandidate    `json:"candidates"`
	UsageMetadata *nativeUsageMetadata `json:"usageMetadata"`
	ModelVersion  string               `json:"modelVersion"`
}

type nativeCandidate struct {
	Content            *nativeCandidateContent `json:"content"`
	FinishReason       string                  `json:"finishReason"`
	Index              int                     `json:"index"`
	GroundingMetadata  json.RawMessage         `json:"groundingMetadata"`
	CitationMetadata   map[string]any          `json:"citationMetadata"`
	URLContextMetadata map[string]any          `json:"urlContextMetadata"`
	SafetyRatings      []any                   `json:"safetyRatings"`
}

type nativeCandidateContent struct {
	Parts []nativeResponsePart `json:"parts"`
	Role  string               `json:"role"`
}

type nativeResponsePart struct {
	Text             string             `json:"text,omitempty"`
	Thought          *bool              `json:"thought,omitempty"`
	ThoughtSignature string             `json:"thoughtSignature,omitempty"`
	FunctionCall     *nativeSSEFuncCall `json:"functionCall,omitempty"`
	InlineData       *nativeInlineData  `json:"inlineData,omitempty"`
}

type nativeSSEFuncCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// nativeInlineData is defined in native_request_encoder.go

type nativeUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
}

// nativeUsageToAI maps Gemini usage metadata to the v7 nested usage shape.
// promptTokenCount already includes cached content, so the non-cached count is
// the remainder after subtracting cachedContentTokenCount.
func nativeUsageToAI(u *nativeUsageMetadata) *aikit.Usage {
	noCache := u.PromptTokenCount - u.CachedContentTokenCount
	if noCache < 0 {
		noCache = 0
	}
	return &aikit.Usage{
		InputTokens: u.PromptTokenCount,
		InputTokenDetails: aikit.InputTokenDetails{
			NoCacheTokens:   noCache,
			CacheReadTokens: u.CachedContentTokenCount,
		},
		OutputTokens:       u.CandidatesTokenCount,
		OutputTokenDetails: aikit.OutputTokenDetails{ReasoningTokens: u.ThoughtsTokenCount},
		TotalTokens:        u.TotalTokenCount,
		Raw: map[string]any{
			"promptTokenCount":        u.PromptTokenCount,
			"candidatesTokenCount":    u.CandidatesTokenCount,
			"totalTokenCount":         u.TotalTokenCount,
			"thoughtsTokenCount":      u.ThoughtsTokenCount,
			"cachedContentTokenCount": u.CachedContentTokenCount,
		},
	}
}

type nativeGroundingMetadata struct {
	WebSearchQueries   []string               `json:"webSearchQueries"`
	ImageSearchQueries []string               `json:"imageSearchQueries"`
	GroundingChunks    []nativeGroundingChunk `json:"groundingChunks"`
	GroundingSupports  []any                  `json:"groundingSupports"`
}

type nativeGroundingChunk struct {
	Web              *nativeWebChunk     `json:"web,omitempty"`
	Image            *nativeImageChunk   `json:"image,omitempty"`
	RetrievedContext *nativeRetrievedCtx `json:"retrievedContext,omitempty"`
	Maps             *nativeMapsChunk    `json:"maps,omitempty"`
}

type nativeWebChunk struct {
	URI   string `json:"uri"`
	Title string `json:"title"`
}

type nativeImageChunk struct {
	URI       string `json:"uri"`
	SourceURI string `json:"sourceUri"`
	ImageURI  string `json:"imageUri"`
	Title     string `json:"title"`
}

type nativeRetrievedCtx struct {
	URI   string `json:"uri"`
	Title string `json:"title"`
}

type nativeMapsChunk struct {
	URI   string `json:"uri"`
	Title string `json:"title"`
}

// --------------------------------------------------------------------
// Decoder
// --------------------------------------------------------------------

// decodeNativeSSEStream reads Gemini native SSE from body and emits aikit.StreamEvents onto ch.
// It closes ch when the stream ends (EOF or context cancellation).
// body is closed when decoding finishes.
func decodeNativeSSEStream(
	ctx context.Context,
	reader *transport.SSEReader,
	ch chan<- aikit.StreamEvent,
) error {
	seen := make(map[string]bool)
	var lastGoogleMeta map[string]any
	toolCallIndex := 0
	hasToolCalls := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := reader.NextData()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("gemini-native: read stream: %w", err)
		}
		if data != "" {
			var chunk nativeSSEChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				ch <- aikit.StreamEvent{
					Type: aikit.StreamEventError,
					Error: fmt.Errorf(
						"gemini-native: unmarshal chunk: %w",
						err,
					),
				}
			} else {
				emitNativeChunkEvents(
					chunk,
					ch,
					seen,
					&lastGoogleMeta,
					&toolCallIndex,
					&hasToolCalls,
				)
			}
		}
	}
}

// emitNativeChunkEvents processes a single native SSE chunk and emits events in order:
// sources → content parts → usage → finish.
func emitNativeChunkEvents(
	chunk nativeSSEChunk,
	ch chan<- aikit.StreamEvent,
	seen map[string]bool,
	lastGoogleMeta *map[string]any,
	toolCallIndex *int,
	hasToolCalls *bool,
) {
	if len(chunk.Candidates) > 0 {
		cand := chunk.Candidates[0]

		mergeNativeGoogleMetadata(lastGoogleMeta, cand)

		// 1. Sources from grounding metadata.
		grounding := decodeNativeGroundingMetadata(cand.GroundingMetadata)
		for _, src := range extractNativeGroundingSources(grounding, seen) {
			ch <- aikit.StreamEvent{Type: aikit.StreamEventSource, Source: &src}
		}

		// 2. Content parts.
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.FunctionCall != nil {
					*hasToolCalls = true
					args := string(part.FunctionCall.Args)
					ch <- aikit.StreamEvent{
						Type:              aikit.StreamEventToolCallDelta,
						ToolCallIndex:     *toolCallIndex,
						ToolCallID:        fmt.Sprintf("call_%d", *toolCallIndex),
						ToolCallName:      part.FunctionCall.Name,
						ToolCallArgsDelta: args,
						ThoughtSignature:  part.ThoughtSignature,
					}
					*toolCallIndex++
					continue
				}

				if part.Thought != nil && *part.Thought {
					ch <- aikit.StreamEvent{
						Type:             aikit.StreamEventReasoningDelta,
						TextDelta:        part.Text,
						ThoughtSignature: part.ThoughtSignature,
					}
				} else if part.Text != "" {
					ch <- aikit.StreamEvent{
						Type:             aikit.StreamEventTextDelta,
						TextDelta:        part.Text,
						ThoughtSignature: part.ThoughtSignature,
					}
				}

				// Handle inlineData (image/file output).
				if part.InlineData != nil {
					decoded, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
					if err == nil {
						ch <- aikit.StreamEvent{
							Type:          aikit.StreamEventFileDelta,
							FileData:      decoded,
							FileMediaType: part.InlineData.MediaType,
						}
					}
				}
			}
		}

		// 3. Usage (emit before finish so consumers see token counts first).
		if chunk.UsageMetadata != nil {
			ch <- aikit.StreamEvent{
				Type:  aikit.StreamEventUsage,
				Usage: nativeUsageToAI(chunk.UsageMetadata),
			}
		}

		// 4. Finish.
		if cand.FinishReason != "" {
			reason, raw := mapNativeFinishReason(cand.FinishReason, *hasToolCalls)
			var provMeta map[string]any
			if *lastGoogleMeta != nil {
				provMeta = map[string]any{"google": *lastGoogleMeta}
			}
			ch <- aikit.StreamEvent{
				Type:             aikit.StreamEventFinish,
				FinishReason:     reason,
				RawFinishReason:  raw,
				ProviderMetadata: provMeta,
			}
		}

		return
	}

	// No candidates — might still have usage metadata.
	if chunk.UsageMetadata != nil {
		ch <- aikit.StreamEvent{
			Type:  aikit.StreamEventUsage,
			Usage: nativeUsageToAI(chunk.UsageMetadata),
		}
	}
}

// mapNativeFinishReason converts a Gemini native API finish reason to an aikit.FinishReason.
func mapNativeFinishReason(raw string, hasToolCalls bool) (aikit.FinishReason, string) {
	switch raw {
	case "STOP":
		if hasToolCalls {
			return aikit.FinishReasonToolCalls, raw
		}
		return aikit.FinishReasonStop, raw
	case "MAX_TOKENS":
		return aikit.FinishReasonLength, raw
	case "SAFETY", "RECITATION", "BLOCKLIST":
		return aikit.FinishReasonContentFilter, raw
	case "MALFORMED_FUNCTION_CALL":
		return aikit.FinishReasonError, raw
	default:
		return aikit.FinishReasonUnknown, raw
	}
}

// extractNativeGroundingSources extracts deduplicated aikit.Source values from grounding metadata.
func extractNativeGroundingSources(gm *nativeGroundingMetadata, seen map[string]bool) []aikit.Source {
	if gm == nil {
		return nil
	}
	var sources []aikit.Source
	for _, gc := range gm.GroundingChunks {
		if gc.Web != nil {
			if src := extractNativeWebSource(gc.Web, seen); src != nil {
				sources = append(sources, *src)
				continue
			}
		}
		if gc.RetrievedContext != nil {
			if src := extractNativeRetrievedContextSource(gc.RetrievedContext, seen); src != nil {
				sources = append(sources, *src)
				continue
			}
		}
		if gc.Image != nil {
			if src := extractNativeImageSource(gc.Image, seen); src != nil {
				sources = append(sources, *src)
				continue
			}
		}
		if gc.Maps != nil {
			if src := extractNativeMapsSource(gc.Maps, seen); src != nil {
				sources = append(sources, *src)
			}
		}
	}
	return sources
}

func extractNativeWebSource(web *nativeWebChunk, seen map[string]bool) *aikit.Source {
	if web.URI == "" {
		return nil
	}
	if seen[web.URI] {
		return nil
	}
	seen[web.URI] = true
	return &aikit.Source{
		SourceType: "url",
		URL:        web.URI,
		Title:      web.Title,
	}
}

func extractNativeRetrievedContextSource(rc *nativeRetrievedCtx, seen map[string]bool) *aikit.Source {
	if rc.URI == "" {
		return nil
	}
	if seen[rc.URI] {
		return nil
	}
	seen[rc.URI] = true
	return &aikit.Source{
		SourceType: "retrieved-context",
		URL:        rc.URI,
		Title:      rc.Title,
	}
}

func extractNativeImageSource(img *nativeImageChunk, seen map[string]bool) *aikit.Source {
	uri := img.ImageURI
	if uri == "" {
		uri = img.SourceURI
	}
	if uri == "" {
		uri = img.URI
	}
	if uri == "" {
		uri = "image-chunk"
	}
	key := "image:" + uri
	if seen[key] {
		return nil
	}
	seen[key] = true
	return &aikit.Source{
		SourceType: "image",
		URL:        uri,
		Title:      img.Title,
		ProviderMetadata: map[string]any{
			"sourceUri": img.SourceURI,
			"imageUri":  img.ImageURI,
		},
	}
}

func extractNativeMapsSource(m *nativeMapsChunk, seen map[string]bool) *aikit.Source {
	uri := m.URI
	if uri == "" {
		uri = "maps-chunk"
	}
	key := "maps:" + uri
	if seen[key] {
		return nil
	}
	seen[key] = true
	return &aikit.Source{
		SourceType: "maps",
		URL:        uri,
		Title:      m.Title,
	}
}

func decodeNativeGroundingMetadata(raw json.RawMessage) *nativeGroundingMetadata {
	if len(raw) == 0 {
		return nil
	}
	var metadata nativeGroundingMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil
	}
	return &metadata
}

// mergeNativeGoogleMetadata preserves metadata delivered incrementally across
// streamed candidates. Array fields are appended; scalar/object fields keep
// the latest value.
func mergeNativeGoogleMetadata(destination *map[string]any, candidate nativeCandidate) {
	if *destination == nil {
		*destination = make(map[string]any)
	}
	if len(candidate.GroundingMetadata) > 0 {
		var incoming map[string]any
		if json.Unmarshal(candidate.GroundingMetadata, &incoming) == nil {
			current, ok := (*destination)["groundingMetadata"].(map[string]any)
			if !ok {
				current = nil
			}
			if current == nil {
				current = make(map[string]any)
			}
			for key, value := range incoming {
				if items, ok := value.([]any); ok {
					if existing, ok := current[key].([]any); ok {
						items = append(existing, items...)
					}
					current[key] = items
					continue
				}
				current[key] = value
			}
			(*destination)["groundingMetadata"] = current
		}
	}
	if candidate.CitationMetadata != nil {
		(*destination)["citationMetadata"] = candidate.CitationMetadata
	}
	if candidate.URLContextMetadata != nil {
		(*destination)["urlContextMetadata"] = candidate.URLContextMetadata
	}
	if candidate.SafetyRatings != nil {
		(*destination)["safetyRatings"] = candidate.SafetyRatings
	}
	if len(*destination) == 0 {
		*destination = nil
	}
}
