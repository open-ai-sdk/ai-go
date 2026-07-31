package jsonclone

import "testing"

type namedStrings map[string]string

func TestMapClonesTypedContainersAndCycles(t *testing.T) {
	original := map[string]any{
		"typed-map": namedStrings{"value": "original"},
		"typed-slice": []map[string]any{{
			"value": "original",
		}},
	}
	original["self"] = original

	cloned := Map(original)
	cloned["typed-map"].(namedStrings)["value"] = "changed"
	cloned["typed-slice"].([]map[string]any)[0]["value"] = "changed"
	cloned["self"].(map[string]any)["clone-only"] = true

	if got := original["typed-map"].(namedStrings)["value"]; got != "original" {
		t.Fatalf("original typed map value = %q, want original", got)
	}
	if got := original["typed-slice"].([]map[string]any)[0]["value"]; got != "original" {
		t.Fatalf("original typed slice value = %q, want original", got)
	}
	if _, exists := original["clone-only"]; exists {
		t.Fatal("cloned cycle still points at the original map")
	}
}

func TestMapPreservesOverlappingSliceViewShapes(t *testing.T) {
	base := []any{"first", "second"}
	original := map[string]any{
		"short": base[:1],
		"long":  base[:2],
	}

	cloned := Map(original)
	if got := len(cloned["short"].([]any)); got != 1 {
		t.Fatalf("short slice length = %d, want 1", got)
	}
	if got := len(cloned["long"].([]any)); got != 2 {
		t.Fatalf("long slice length = %d, want 2", got)
	}
}
