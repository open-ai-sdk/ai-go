package ainode

import (
	"encoding/base64"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ChunkDataStructuredOutput carries structured output on the v7 wire. The
// protocol has no dedicated chunk for it, and `data-` is its sanctioned
// extension prefix.
const ChunkDataStructuredOutput = "data-structured-output"

// chunksFile serializes a model-emitted file as a data URL. The v7 `file` chunk
// carries only url and mediaType; it has no filename field.
func (cp *ChunkProducer) chunksFile(event aikit.StepEvent) []Chunk {
	if len(event.FileData) == 0 {
		return nil
	}
	mediaType := event.FileMediaType
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	fields := withProviderMetadata(map[string]any{
		"mediaType": mediaType,
		"url":       "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(event.FileData),
	}, event.ProviderMetadata)
	return []Chunk{{Type: ChunkFile, Fields: fields}}
}

// chunksStructuredOutput publishes the final object as a transient data chunk,
// so it reaches the UI without being persisted into the message parts.
func (cp *ChunkProducer) chunksStructuredOutput(event aikit.StepEvent) []Chunk {
	if len(event.StructuredOutput) == 0 {
		return nil
	}
	var data any
	if err := json.Unmarshal(event.StructuredOutput, &data); err != nil {
		data = string(event.StructuredOutput)
	}
	return []Chunk{{
		Type:   ChunkDataStructuredOutput,
		Fields: map[string]any{"data": data, "transient": true},
	}}
}

// usageMetadata renders the run's token totals for the finish chunk's
// messageMetadata. v7 removed every usage field from the chunk schema, so
// metadata is the only channel that reaches useChat's onFinish.
func (cp *ChunkProducer) usageMetadata() map[string]any {
	total := cp.usage.snapshot()
	if total == (UsageInfo{}) {
		return nil
	}
	usage := map[string]any{
		"inputTokens":  total.InputTokens,
		"outputTokens": total.OutputTokens,
		"totalTokens":  total.TotalTokens,
	}
	if details := inputTokenDetails(total.InputTokenDetails); len(details) > 0 {
		usage["inputTokenDetails"] = details
	}
	if details := outputTokenDetails(total.OutputTokenDetails); len(details) > 0 {
		usage["outputTokenDetails"] = details
	}
	return map[string]any{"usage": usage}
}

func inputTokenDetails(details InputTokenDetails) map[string]any {
	fields := map[string]any{}
	if details.NoCacheTokens != 0 {
		fields["noCacheTokens"] = details.NoCacheTokens
	}
	if details.CacheReadTokens != 0 {
		fields["cacheReadTokens"] = details.CacheReadTokens
	}
	if details.CacheWriteTokens != 0 {
		fields["cacheWriteTokens"] = details.CacheWriteTokens
	}
	return fields
}

func outputTokenDetails(details OutputTokenDetails) map[string]any {
	fields := map[string]any{}
	if details.TextTokens != 0 {
		fields["textTokens"] = details.TextTokens
	}
	if details.ReasoningTokens != 0 {
		fields["reasoningTokens"] = details.ReasoningTokens
	}
	return fields
}
