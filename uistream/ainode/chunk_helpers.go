package ainode

import "strconv"

func blockID(number int) string { return "text_" + strconv.Itoa(number) }

func withProviderMetadata(fields, metadata map[string]any) map[string]any {
	if metadata == nil {
		return fields
	}
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["providerMetadata"] = metadata
	return fields
}

// MergeChunks preserves source order while allowing sources to interleave.
func MergeChunks(sources ...<-chan Chunk) <-chan Chunk {
	output := make(chan Chunk, 64)
	if len(sources) == 0 {
		close(output)
		return output
	}
	done := make(chan bool, len(sources))
	for _, source := range sources {
		go func() {
			panicked := false
			defer func() { done <- panicked }()
			defer recoverPanic(func(err error) {
				panicked = true
				recoverToChunk(output)(err)
			})
			for chunk := range source {
				output <- chunk
			}
		}()
	}
	go func() {
		defer recoverPanic(nil)
		panicked := false
		for range sources {
			panicked = <-done || panicked
		}
		if panicked {
			output <- Chunk{Type: ChunkFinish, Fields: map[string]any{"finishReason": "error"}}
		}
		close(output)
	}()
	return output
}
