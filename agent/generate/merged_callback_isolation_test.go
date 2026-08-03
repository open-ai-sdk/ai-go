package generate

import "testing"

func TestMergeCallback_IsolatesMutableEventPayloads(t *testing.T) {
	mutated := make(chan struct{})
	var observed any

	merged := mergeCallback(
		func(event ChunkEvent) {
			event.Usage.InputTokens = 999
			event.Usage.Raw["nested"].(map[string]any)["value"] = "corrupt"
			close(mutated)
		},
		func(event ChunkEvent) {
			<-mutated
			observed = event.Usage.Raw["nested"].(map[string]any)["value"]
			if event.Usage.InputTokens != 1 {
				t.Errorf("call callback InputTokens = %d, want 1", event.Usage.InputTokens)
			}
		},
	)

	merged(ChunkEvent{
		Type: "usage",
		Usage: &Usage{
			InputTokens: 1,
			Raw: map[string]any{
				"nested": map[string]any{"value": "original"},
			},
		},
	})

	if observed != "original" {
		t.Fatalf("call callback Raw value = %v, want original", observed)
	}
}

func TestMergeCallbackIsolatesStepEndMixedContent(t *testing.T) {
	mutated := make(chan struct{})
	var content, files string
	merged := mergeCallback(
		func(event StepEndEvent) {
			event.Content[0].Data[0] = 'X'
			event.Files[0].Data[0] = 'Y'
			close(mutated)
		},
		func(event StepEndEvent) {
			<-mutated
			content = string(event.Content[0].Data)
			files = string(event.Files[0].Data)
		},
	)

	merged(StepEndEvent{
		Content: []ContentPart{{Type: ContentPartTypeFile, Data: []byte("content"), MediaType: "image/png"}},
		Files:   []GeneratedFile{{Data: []byte("file"), MediaType: "image/png"}},
	})
	if content != "content" {
		t.Errorf("call callback Content data = %q", content)
	}
	if files != "file" {
		t.Errorf("call callback Files data = %q", files)
	}
}
