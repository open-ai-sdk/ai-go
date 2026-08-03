package jsonclone

import "testing"

func BenchmarkMapRepresentativeMetadata(b *testing.B) {
	input := map[string]any{
		"provider": "openai",
		"model":    "gpt-5",
		"usage": map[string]any{
			"input_tokens":  uint64(1280),
			"output_tokens": uint64(320),
			"details": map[string]any{
				"cached_tokens":    uint64(768),
				"reasoning_tokens": uint64(96),
			},
		},
		"choices": []any{
			map[string]any{"index": 0, "finish_reason": "tool_calls"},
			map[string]any{"index": 1, "finish_reason": "stop"},
		},
		"headers": map[string]string{
			"request-id": "req_123",
			"region":     "sg",
		},
		"embedding": []any{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
	}

	for b.Loop() {
		_ = Map(input)
	}
}

func BenchmarkValueNamedContainers(b *testing.B) {
	type namedSlice []map[string]int
	type namedMap map[string]namedSlice
	input := namedMap{
		"items": {
			{"one": 1, "two": 2},
			{"three": 3, "four": 4},
		},
	}

	for b.Loop() {
		_ = Value(input)
	}
}
