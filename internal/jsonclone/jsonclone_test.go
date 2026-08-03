package jsonclone

import (
	"encoding/json"
	"reflect"
	"testing"
)

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

func TestValueClonesSelfReferentialSlice(t *testing.T) {
	original := make([]any, 1)
	original[0] = original

	cloned := Value(original).([]any)
	self := cloned[0].([]any)
	if reflect.ValueOf(cloned).Pointer() != reflect.ValueOf(self).Pointer() {
		t.Fatal("slice cycle does not point at the cloned slice")
	}
	if reflect.ValueOf(cloned).Pointer() == reflect.ValueOf(original).Pointer() {
		t.Fatal("cloned slice still aliases the original")
	}
}

func TestMapPreservesAliasesWithinTheClone(t *testing.T) {
	sharedMap := map[string]any{"value": "original"}
	sharedSlice := []any{"original"}
	sharedBytes := []byte("original")
	original := map[string]any{
		"map-one":   sharedMap,
		"map-two":   sharedMap,
		"slice-one": sharedSlice,
		"slice-two": sharedSlice,
		"bytes-one": sharedBytes,
		"bytes-two": sharedBytes,
	}

	cloned := Map(original)
	cloned["map-one"].(map[string]any)["value"] = "changed"
	cloned["slice-one"].([]any)[0] = "changed"
	cloned["bytes-one"].([]byte)[0] = 'X'

	if got := cloned["map-two"].(map[string]any)["value"]; got != "changed" {
		t.Fatalf("map alias value = %q, want changed", got)
	}
	if got := cloned["slice-two"].([]any)[0]; got != "changed" {
		t.Fatalf("slice alias value = %q, want changed", got)
	}
	if got := cloned["bytes-two"].([]byte)[0]; got != 'X' {
		t.Fatalf("byte alias value = %q, want X", got)
	}
	if sharedMap["value"] != "original" || sharedSlice[0] != "original" || sharedBytes[0] != 'o' {
		t.Fatal("clone mutation leaked to original aliases")
	}
}

func TestValuePreservesFastPathTypesAndNilShapes(t *testing.T) {
	emptyBytes := make([]byte, 0)
	emptyRaw := make(json.RawMessage, 0)
	var nilAnySlice []any
	input := map[string]any{
		"null":        nil,
		"bytes":       []byte("bytes"),
		"raw":         json.RawMessage(`{"value":true}`),
		"strings":     []string{"one", "two"},
		"string-map":  map[string]string{"key": "value"},
		"empty-bytes": emptyBytes,
		"empty-raw":   emptyRaw,
		"nil-any":     nilAnySlice,
	}

	cloned := Value(input).(map[string]any)
	if cloned["null"] != nil {
		t.Fatalf("null value = %#v, want nil", cloned["null"])
	}
	if reflect.TypeOf(cloned["raw"]) != reflect.TypeOf(json.RawMessage{}) {
		t.Fatalf("raw type = %T, want json.RawMessage", cloned["raw"])
	}
	if cloned["empty-bytes"].([]byte) == nil {
		t.Fatal("non-nil empty []byte became nil")
	}
	if cloned["empty-raw"].(json.RawMessage) == nil {
		t.Fatal("non-nil empty json.RawMessage became nil")
	}
	if cloned["nil-any"].([]any) != nil {
		t.Fatal("typed nil []any became non-nil")
	}

	cloned["bytes"].([]byte)[0] = 'X'
	cloned["raw"].(json.RawMessage)[0] = '['
	if input["bytes"].([]byte)[0] != 'b' || input["raw"].(json.RawMessage)[0] != '{' {
		t.Fatal("fast-path byte mutation leaked to original")
	}
}

func TestValueWithPointersClonesPointersAndPreservesAliases(t *testing.T) {
	shared := map[string]any{"value": "original"}
	pointer := &shared
	input := map[string]any{"first": pointer, "second": pointer}

	cloned := ValueWithPointers(input).(map[string]any)
	first := cloned["first"].(*map[string]any)
	second := cloned["second"].(*map[string]any)
	if first == pointer {
		t.Fatal("pointer clone still aliases the original pointer")
	}
	(*first)["value"] = "changed"
	if got := (*second)["value"]; got != "changed" {
		t.Fatalf("pointer alias value = %q, want changed", got)
	}
	if got := shared["value"]; got != "original" {
		t.Fatalf("pointer clone mutation leaked to original: %q", got)
	}

	standard := Value(input).(map[string]any)
	if standard["first"].(*map[string]any) != pointer {
		t.Fatal("Value unexpectedly cloned a pointer")
	}
}
