package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/transport"
)

// SSE event types from Anthropic's streaming API.
const (
	eventMessageStart      = "message_start"
	eventContentBlockStart = "content_block_start"
	eventContentBlockDelta = "content_block_delta"
	eventContentBlockStop  = "content_block_stop"
	eventMessageDelta      = "message_delta"
	eventMessageStop       = "message_stop"
	eventPing              = "ping"
	eventError             = "error"
)

type sseMessageStart struct {
	Message struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type sseContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"content_block"`
}

type sseContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
	} `json:"delta"`
}

type sseMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type sseError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// blockState tracks an active content block during SSE decoding.
type blockState struct {
	index int
	typ   string
	id    string
	name  string
}

// decodeSSEStream reads Anthropic SSE events and emits normalized aikit.StreamEvents.
func decodeSSEStream(
	ctx context.Context,
	reader *transport.SSEReader,
	out chan<- aikit.StreamEvent,
	encodeWarnings ...aikit.Warning,
) error {
	send := func(ev aikit.StreamEvent) bool {
		select {
		case out <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	var eventType string
	var messageID string
	blocks := make(map[int]*blockState)
	warnings := append([]aikit.Warning(nil), encodeWarnings...)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("anthropic: read stream: %w", err)
		}

		eventType = frame.Event
		if frame.Data == "" {
			continue
		}
		if !dispatchSSEEvent(
			eventType,
			frame.Data,
			blocks,
			&messageID,
			send,
			&warnings,
		) {
			return ctx.Err()
		}
	}
}

// dispatchSSEEvent handles a single SSE data payload by event type.
// Returns false if the caller should stop (channel closed or terminal error).
func dispatchSSEEvent(
	eventType, data string,
	blocks map[int]*blockState,
	messageID *string,
	send func(aikit.StreamEvent) bool,
	warnings *[]aikit.Warning,
) bool {
	switch eventType {
	case eventMessageStart:
		return handleMessageStart(data, messageID, send)
	case eventContentBlockStart:
		return handleContentBlockStart(data, blocks, send, warnings)
	case eventContentBlockDelta:
		return handleContentBlockDelta(data, blocks, send)
	case eventMessageDelta:
		return handleMessageDelta(data, *messageID, send, *warnings)
	case eventError:
		return handleError(data, send)
	}
	return true
}

func handleMessageStart(
	data string,
	messageID *string,
	send func(aikit.StreamEvent) bool,
) bool {
	var msg sseMessageStart
	if json.Unmarshal([]byte(data), &msg) == nil {
		*messageID = msg.Message.ID
		u := msg.Message.Usage
		// Anthropic reports input_tokens as the non-cached prompt tokens; cache
		// reads and writes are counted separately. The v7 InputTokens total is
		// their sum, and NoCacheTokens is the raw input_tokens.
		return send(aikit.StreamEvent{
			Type: aikit.StreamEventUsage,
			Usage: &aikit.Usage{
				InputTokens:  u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens,
				OutputTokens: u.OutputTokens,
				InputTokenDetails: aikit.InputTokenDetails{
					NoCacheTokens:    u.InputTokens,
					CacheReadTokens:  u.CacheReadInputTokens,
					CacheWriteTokens: u.CacheCreationInputTokens,
				},
				Raw: map[string]any{
					"input_tokens":                u.InputTokens,
					"output_tokens":               u.OutputTokens,
					"cache_read_input_tokens":     u.CacheReadInputTokens,
					"cache_creation_input_tokens": u.CacheCreationInputTokens,
				},
			},
		})
	}
	return true
}

func handleContentBlockStart(
	data string,
	blocks map[int]*blockState,
	send func(aikit.StreamEvent) bool,
	warnings *[]aikit.Warning,
) bool {
	var block sseContentBlockStart
	if json.Unmarshal([]byte(data), &block) == nil {
		blocks[block.Index] = &blockState{
			index: block.Index,
			typ:   block.ContentBlock.Type,
			id:    block.ContentBlock.ID,
			name:  block.ContentBlock.Name,
		}
		if block.ContentBlock.Type == "tool_use" {
			return send(aikit.StreamEvent{
				Type:          aikit.StreamEventToolCallDelta,
				ToolCallIndex: block.Index,
				ToolCallID:    block.ContentBlock.ID,
				ToolCallName:  block.ContentBlock.Name,
			})
		}
		switch block.ContentBlock.Type {
		case "text", "tool_use", "thinking":
		default:
			*warnings = append(*warnings, unsupportedResponseBlockWarning(block.ContentBlock.Type))
		}
	}
	return true
}

func handleContentBlockDelta(
	data string,
	blocks map[int]*blockState,
	send func(aikit.StreamEvent) bool,
) bool {
	var delta sseContentBlockDelta
	if json.Unmarshal([]byte(data), &delta) != nil {
		return true
	}
	bs := blocks[delta.Index]
	switch delta.Delta.Type {
	case "text_delta":
		return send(aikit.StreamEvent{
			Type:      aikit.StreamEventTextDelta,
			TextDelta: delta.Delta.Text,
		})
	case "input_json_delta":
		if bs != nil {
			return send(aikit.StreamEvent{
				Type:              aikit.StreamEventToolCallDelta,
				ToolCallIndex:     delta.Index,
				ToolCallID:        bs.id,
				ToolCallName:      bs.name,
				ToolCallArgsDelta: delta.Delta.PartialJSON,
			})
		}
	case "thinking_delta":
		return send(aikit.StreamEvent{
			Type:      aikit.StreamEventReasoningDelta,
			TextDelta: delta.Delta.Thinking,
		})
	}
	return true
}

func handleMessageDelta(
	data string,
	messageID string,
	send func(aikit.StreamEvent) bool,
	encodeWarnings []aikit.Warning,
) bool {
	var msg sseMessageDelta
	if json.Unmarshal([]byte(data), &msg) != nil {
		return true
	}
	// Emit usage before finish so consumers don't miss the final token count.
	if msg.Usage.OutputTokens > 0 {
		if !send(aikit.StreamEvent{
			Type:  aikit.StreamEventUsage,
			Usage: &aikit.Usage{OutputTokens: msg.Usage.OutputTokens},
		}) {
			return false
		}
	}
	return send(aikit.StreamEvent{
		Type:            aikit.StreamEventFinish,
		MessageID:       messageID,
		FinishReason:    mapStopReason(msg.Delta.StopReason),
		RawFinishReason: msg.Delta.StopReason,
		Warnings:        encodeWarnings,
	})
}

func handleError(
	data string,
	send func(aikit.StreamEvent) bool,
) bool {
	var errMsg sseError
	if json.Unmarshal([]byte(data), &errMsg) == nil {
		send(aikit.StreamEvent{
			Type: aikit.StreamEventError,
			Error: fmt.Errorf(
				"anthropic: %s: %s",
				errMsg.Error.Type, errMsg.Error.Message,
			),
		})
		return false
	}
	return true
}

func mapStopReason(reason string) aikit.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence":
		return aikit.FinishReasonStop
	case "tool_use":
		return aikit.FinishReasonToolCalls
	case "max_tokens":
		return aikit.FinishReasonLength
	default:
		return aikit.FinishReasonUnknown
	}
}
