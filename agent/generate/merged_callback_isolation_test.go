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
