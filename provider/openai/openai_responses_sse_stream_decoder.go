package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/transport"
)

// responsesChunk represents a single SSE event from the OpenAI Responses API stream.
// Only the fields needed for stream normalization are decoded.
type responsesChunk struct {
	Type        string          `json:"type"`
	Code        string          `json:"code"`
	Message     string          `json:"message"`
	RawResponse json.RawMessage `json:"-"`
	RawEvent    json.RawMessage `json:"-"`

	// response.created / response.completed
	Response *struct {
		ID     string `json:"id"`
		Model  string `json:"model"`
		Status string `json:"status"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			TotalTokens        int `json:"total_tokens"`
			InputTokensDetails *struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"input_tokens_details,omitempty"`
			OutputTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"output_tokens_details,omitempty"`
		} `json:"usage"`
	} `json:"response"`

	// response.output_text.delta
	Delta string `json:"delta"`

	// response.output_item.added (web search action / function call)
	Item *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`

	// response.function_call_arguments.delta
	// response.function_call_arguments.done
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`

	// response.reasoning_summary_text.delta
	// (same Delta field, distinguished by Type)

	// response.web_search_call.action.sources (when include requested)
	Sources []struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"sources"`

	// Error event
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// streamState holds mutable state accumulated during SSE stream decoding.
type streamState struct {
	responseID    string
	requestID     string
	model         string
	messageID     string
	callsByItemID map[string]*pendingCall
	callOrder     []string
}

type pendingCall struct {
	id   string
	name string
	args strings.Builder
}

// decodeResponsesSSEStream reads OpenAI Responses API SSE lines and emits
// normalized aikit.StreamEvents onto ch. Closes ch when done or on error.
// encodingWarnings are merged onto the finish event so callers see them in the
// agent.Result.Warnings field without a separate event.
func decodeResponsesSSEStream(
	ctx context.Context,
	reader *transport.SSEReader,
	ch chan<- aikit.StreamEvent,
	encodingWarnings ...aikit.Warning,
) error {
	return decodeResponsesSSEStreamWithRequestID(ctx, reader, ch, "", encodingWarnings...)
}

func decodeResponsesSSEStreamWithRequestID(
	ctx context.Context,
	reader *transport.SSEReader,
	ch chan<- aikit.StreamEvent,
	requestID string,
	encodingWarnings ...aikit.Warning,
) error {
	return decodeResponsesSSEStreamWithOptions(ctx, reader, ch, requestID, false, false, encodingWarnings...)
}

func decodeResponsesSSEStreamWithOptions(
	ctx context.Context,
	reader *transport.SSEReader,
	ch chan<- aikit.StreamEvent,
	requestID string,
	emitReasoningEvents bool,
	allowMissingTerminalEvent bool,
	encodingWarnings ...aikit.Warning,
) error {
	state := &streamState{callsByItemID: make(map[string]*pendingCall), requestID: requestID}
	var reasoning *responsesReasoningState
	if emitReasoningEvents {
		reasoning = newResponsesReasoningState()
	}

	terminal := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := reader.NextData()
		if errors.Is(err, io.EOF) {
			return responsesStreamEnd(ctx, terminal, allowMissingTerminalEvent)
		}
		if err != nil {
			return fmt.Errorf("openai: read stream: %w", err)
		}
		if data == "[DONE]" {
			return responsesStreamEnd(ctx, terminal, allowMissingTerminalEvent)
		}
		var chunk responsesChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf(
				"openai: unmarshal responses chunk: %w",
				err,
			)
		}
		if terminalResponsesEvent(chunk.Type) {
			terminal = true
			var envelope struct {
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal([]byte(data), &envelope); err != nil {
				return fmt.Errorf("openai: unmarshal response envelope: %w", err)
			}
			chunk.RawResponse = envelope.Response
		}
		if (chunk.Type == "error" || chunk.Type == "response.failed") && transport.CaptureErrorBody(ctx) {
			chunk.RawEvent = json.RawMessage(data)
		}
		if reasoning != nil {
			handled, err := reasoning.dispatch(data, chunk.Type, ch)
			if err != nil {
				return err
			}
			if handled {
				continue
			}
		}
		if done := dispatchChunk(
			chunk,
			state,
			ch,
			encodingWarnings,
		); done {
			return nil
		}
	}
}

func responsesStreamEnd(ctx context.Context, terminal, allowMissing bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !terminal && !allowMissing {
		return &aikit.MissingTerminalEventError{Provider: "openai"}
	}
	return nil
}

func terminalResponsesEvent(kind string) bool {
	switch kind {
	case "response.completed", "response.failed", "response.cancelled", "response.incomplete":
		return true
	default:
		return false
	}
}

func handleResponseFailed(chunk responsesChunk, state *streamState, ch chan<- aikit.StreamEvent) {
	code, message := "", "response failed"
	if chunk.Response != nil && chunk.Response.Error != nil {
		code, message = chunk.Response.Error.Code, chunk.Response.Error.Message
	}
	ch <- aikit.StreamEvent{Type: aikit.StreamEventError, Error: &aikit.APIError{
		Provider: "openai", Code: code, Message: message,
		RequestID: state.requestID, RawBody: chunk.RawEvent,
	}}
}

// dispatchChunk routes a single SSE chunk to the appropriate handler.
// Returns true if the stream should terminate.
func dispatchChunk(
	chunk responsesChunk,
	state *streamState,
	ch chan<- aikit.StreamEvent,
	encodingWarnings []aikit.Warning,
) bool {
	switch chunk.Type {
	case "response.created":
		if chunk.Response != nil {
			state.responseID = chunk.Response.ID
			state.model = chunk.Response.Model
		}

	case "response.completed":
		handleResponseCompleted(chunk, state, ch, encodingWarnings)

	case "response.failed":
		handleResponseFailed(chunk, state, ch)
		return true
	case "response.cancelled", "response.incomplete":
		ch <- aikit.StreamEvent{
			Type:             aikit.StreamEventFinish,
			FinishReason:     mapResponsesFinishReason(chunk.Type, false),
			RawFinishReason:  chunk.Type,
			ProviderMetadata: responsesChunkMetadata(chunk, state),
		}
		return true

	case "error":
		code, message := chunk.Code, chunk.Message
		if chunk.Error != nil {
			code, message = chunk.Error.Code, chunk.Error.Message
		}
		ch <- aikit.StreamEvent{
			Type: aikit.StreamEventError,
			Error: &aikit.APIError{
				Provider: "openai", Code: code, Message: message, RequestID: state.requestID,
				RawBody: chunk.RawEvent,
			},
		}
		return true

	case "response.output_text.delta":
		if chunk.Delta != "" {
			ch <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: chunk.Delta}
		}

	case "response.reasoning_summary_text.delta":
		if chunk.Delta != "" {
			ch <- aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: chunk.Delta}
		}

	case "response.output_item.added":
		if chunk.Item != nil && chunk.Item.Type == "message" && state.messageID == "" {
			state.messageID = chunk.Item.ID
		}
		handleOutputItemAdded(chunk, state, ch)

	case "response.function_call_arguments.delta":
		handleFunctionCallArgsDelta(chunk, state, ch)

	case "response.web_search_call.sources":
		for _, s := range chunk.Sources {
			ch <- aikit.StreamEvent{
				Type:   aikit.StreamEventSource,
				Source: &aikit.Source{SourceType: "url", ID: s.ID, URL: s.URL, Title: s.Title},
			}
		}
	}
	return false
}

func handleResponseCompleted(
	chunk responsesChunk,
	state *streamState,
	ch chan<- aikit.StreamEvent,
	encodingWarnings []aikit.Warning,
) {
	if chunk.Response == nil {
		return
	}
	if chunk.Response.ID != "" {
		state.responseID = chunk.Response.ID
	}
	if chunk.Response.Model != "" {
		state.model = chunk.Response.Model
	}
	if u := chunk.Response.Usage; u != nil {
		var cachedTokens, cacheWriteTokens, reasoningTokens int
		if u.InputTokensDetails != nil {
			cachedTokens = u.InputTokensDetails.CachedTokens
			cacheWriteTokens = u.InputTokensDetails.CacheWriteTokens
		}
		if u.OutputTokensDetails != nil {
			reasoningTokens = u.OutputTokensDetails.ReasoningTokens
		}
		// input_tokens already includes cached and cache-write tokens.
		noCache := u.InputTokens - cachedTokens - cacheWriteTokens
		if noCache < 0 {
			noCache = 0
		}
		ch <- aikit.StreamEvent{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{
			InputTokens: u.InputTokens,
			InputTokenDetails: aikit.InputTokenDetails{
				NoCacheTokens:    noCache,
				CacheReadTokens:  cachedTokens,
				CacheWriteTokens: cacheWriteTokens,
			},
			OutputTokens: u.OutputTokens,
			OutputTokenDetails: aikit.OutputTokenDetails{
				TextTokens:      u.OutputTokens - reasoningTokens,
				ReasoningTokens: reasoningTokens,
			},
			TotalTokens: u.TotalTokens,
			Raw: map[string]any{
				"input_tokens":     u.InputTokens,
				"output_tokens":    u.OutputTokens,
				"total_tokens":     u.TotalTokens,
				"cached_tokens":    cachedTokens,
				"reasoning_tokens": reasoningTokens,
			},
		}}
	}
	ch <- aikit.StreamEvent{
		Type:             aikit.StreamEventFinish,
		MessageID:        state.messageID,
		FinishReason:     mapResponsesFinishReason(chunk.Response.Status, len(state.callsByItemID) > 0),
		RawFinishReason:  chunk.Response.Status,
		ProviderMetadata: responsesChunkMetadata(chunk, state),
		Warnings:         encodingWarnings,
	}
}

func handleOutputItemAdded(chunk responsesChunk, state *streamState, ch chan<- aikit.StreamEvent) {
	if chunk.Item == nil || chunk.Item.Type != "function_call" {
		return
	}
	itemID := chunk.Item.ID
	pc := &pendingCall{id: chunk.Item.CallID, name: chunk.Item.Name}
	state.callsByItemID[itemID] = pc
	state.callOrder = append(state.callOrder, itemID)
	ch <- aikit.StreamEvent{
		Type:             aikit.StreamEventToolCallDelta,
		ToolCallIndex:    len(state.callOrder) - 1,
		ToolCallID:       chunk.Item.CallID,
		ToolCallName:     chunk.Item.Name,
		ProviderMetadata: map[string]any{"openai": map[string]any{"itemID": itemID}},
	}
}

func handleFunctionCallArgsDelta(chunk responsesChunk, state *streamState, ch chan<- aikit.StreamEvent) {
	if chunk.Delta == "" || chunk.ItemID == "" {
		return
	}
	pc, ok := state.callsByItemID[chunk.ItemID]
	if !ok {
		return
	}
	pc.args.WriteString(chunk.Delta)
	ch <- aikit.StreamEvent{
		Type:              aikit.StreamEventToolCallDelta,
		ToolCallIndex:     indexOfItemID(state.callOrder, chunk.ItemID),
		ToolCallID:        pc.id,
		ToolCallName:      pc.name,
		ToolCallArgsDelta: chunk.Delta,
	}
}

func indexOfItemID(order []string, id string) int {
	for i, v := range order {
		if v == id {
			return i
		}
	}
	return 0
}

func mapResponsesFinishReason(status string, hasFunctionCall bool) aikit.FinishReason {
	switch status {
	case "completed":
		if hasFunctionCall {
			return aikit.FinishReasonToolCalls
		}
		return aikit.FinishReasonStop
	case "max_output_tokens":
		return aikit.FinishReasonLength
	case "content_filter":
		return aikit.FinishReasonContentFilter
	default:
		if hasFunctionCall {
			return aikit.FinishReasonToolCalls
		}
		return aikit.FinishReasonUnknown
	}
}

func responsesNativeMetadata(responseID, model string) map[string]any {
	meta := map[string]any{"responseId": responseID}
	if model != "" {
		meta["model"] = model
	}
	return meta
}

func responsesChunkMetadata(chunk responsesChunk, state *streamState) map[string]any {
	metadata := responsesNativeMetadata(state.responseID, state.model)
	if state.requestID != "" {
		metadata["requestID"] = state.requestID
	}
	if len(chunk.RawResponse) != 0 {
		metadata["rawResponse"] = chunk.RawResponse
	}
	return map[string]any{"openai": metadata}
}
