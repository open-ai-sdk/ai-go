package aikit

import (
	"encoding/json"
	"strings"
	"testing"
)

// The structural gate must pick exactly the deltas a json.Valid-per-delta fold
// would pick. If it ever diverges, Complete fires at the wrong moment and
// arguments are either truncated or over-appended.
func TestToolCallFoldGateMatchesUngatedValidity(t *testing.T) {
	streams := []string{
		`{"q":"hello"}`,
		`{"a":{"b":[1,2,{"c":"}]"}]}}`,
		`{"escaped":"quote \" and backslash \\"}`,
		`{"brackets_in_string":"{[}]"}`,
		`[1,2,3]`,
		`{}`,
		`"a bare string"`,
		`12345`,
		`true`,
		`null`,
		`{"unterminated":`,
		`{"trailing":1}  `,
	}
	for _, complete := range streams {
		for _, size := range []int{1, 3, 7} {
			t.Run(complete+"/"+string(rune('0'+size)), func(t *testing.T) {
				deltas := chunkString(complete, size)

				var fold ToolCallFold
				var ungated strings.Builder
				for step, delta := range deltas {
					event := StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta}

					wantComplete := false
					if !foldComplete(&fold) {
						ungated.WriteString(delta)
						wantComplete = ungated.Len() > 0 && json.Valid([]byte(ungated.String()))
					} else {
						wantComplete = true
					}

					fold.Add(event)
					if got := foldComplete(&fold); got != wantComplete {
						t.Fatalf("after delta %d (%q): Complete = %v, ungated fold = %v",
							step, delta, got, wantComplete)
					}
				}
				if got := fold.Completed()[0].Args; got != ungated.String() {
					t.Errorf("Args = %q, ungated fold = %q", got, ungated.String())
				}
			})
		}
	}
}

func foldComplete(f *ToolCallFold) bool {
	drafts := f.Completed()
	return len(drafts) > 0 && drafts[0].Complete
}

func chunkString(value string, size int) []string {
	var chunks []string
	for start := 0; start < len(value); start += size {
		end := min(start+size, len(value))
		chunks = append(chunks, value[start:end])
	}
	return chunks
}

// The table above is hand-picked, so it can only find defects someone thought
// of. This searches for the one that matters: any split of any byte string
// where the gated fold and an ungated json.Valid-per-delta fold disagree about
// when the arguments completed.
func FuzzToolCallFoldGateMatchesUngatedValidity(f *testing.F) {
	f.Add(`{"q":"hello"}`, 3)
	f.Add(`{"escaped":"quote \" then \\"}`, 1)
	f.Add(`{"brackets_in_string":"{[}]"}`, 2)
	f.Add(`["a",{"b":[1,{"c":null}]}]`, 5)
	f.Add(`"\\"`, 1)
	f.Add(`12345`, 2)
	f.Add(`  {"padded":1}  `, 4)
	f.Add("{\"unicode\":\"é世\"}", 2)

	f.Fuzz(func(t *testing.T, payload string, size int) {
		if size < 1 || size > 64 || len(payload) > 4096 {
			t.Skip()
		}
		var fold ToolCallFold
		var ungated strings.Builder
		ungatedComplete := false

		for _, delta := range chunkString(payload, size) {
			if !ungatedComplete && delta != "" {
				ungated.WriteString(delta)
				ungatedComplete = ungated.Len() > 0 && json.Valid([]byte(ungated.String()))
			}
			fold.Add(StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta})
		}

		drafts := fold.Completed()
		if len(drafts) == 0 {
			if ungated.Len() != 0 {
				t.Fatalf("gated fold produced no draft for %q", payload)
			}
			return
		}
		if drafts[0].Complete != ungatedComplete {
			t.Fatalf("payload %q size %d: Complete = %v, ungated = %v (args %q)",
				payload, size, drafts[0].Complete, ungatedComplete, drafts[0].Args)
		}
		if drafts[0].Args != ungated.String() {
			t.Fatalf("payload %q size %d: Args = %q, ungated = %q",
				payload, size, drafts[0].Args, ungated.String())
		}
	})
}

// The benchmark below documents the cost but never runs in CI, which uses
// `go test -race ./...` with no -bench. This is the guard that does run.
//
// It asserts the invariant directly rather than inferring it. Each whole-buffer
// json.Valid pass is O(len(args)); doing one per delta is what made the fold
// quadratic. Counting those passes is deterministic — no timing, no allocation
// heuristics — and a revert to validate-every-delta fails it immediately.
//
// Timing and allocation were both tried first and both are the wrong
// instrument: the compiler elides the []byte(string) copy, so an ungated fold
// burns seconds of CPU rescanning while allocating almost nothing.
func TestToolCallFoldValidatesOncePerCompletedValue(t *testing.T) {
	const deltaSize = 20
	arguments := `{"data":"` + strings.Repeat("x", 256<<10) + `"}`
	deltas := chunkString(arguments, deltaSize)

	var fold ToolCallFold
	for _, delta := range deltas {
		fold.Add(StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta})
	}
	if !fold.Completed()[0].Complete {
		t.Fatal("arguments never completed")
	}

	// A streamed JSON object returns to depth zero exactly once, at its closing
	// brace, so exactly one validation should have run across all the deltas.
	if got := fold.drafts[0].validations; got != 1 {
		t.Fatalf("json.Valid ran %d times over %d deltas, want 1 — the structural gate "+
			"is not suppressing whole-buffer revalidation", got, len(deltas))
	}
}

// Nested structure returns to depth zero only at the outermost close, so
// nesting must not multiply validations either.
func TestToolCallFoldGateHoldsForNestedArguments(t *testing.T) {
	arguments := `{"a":{"b":{"c":[1,2,3]}},"d":"` + strings.Repeat("y", 4096) + `"}`
	deltas := chunkString(arguments, 8)

	var fold ToolCallFold
	for _, delta := range deltas {
		fold.Add(StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta})
	}
	if got := fold.drafts[0].validations; got != 1 {
		t.Fatalf("json.Valid ran %d times over %d deltas, want 1", got, len(deltas))
	}
}

// Accumulating n deltas must stay linear. A fold that concatenates strings and
// rescans the whole buffer per delta is quadratic in both, and it runs on the
// consumer's goroutine while a model streams a large tool argument.
func BenchmarkToolCallFoldLargeArguments(b *testing.B) {
	const (
		deltaSize = 20
		total     = 256 << 10
	)
	payload := `{"data":"` + strings.Repeat("x", total) + `"}`
	deltas := chunkString(payload, deltaSize)

	b.ResetTimer()
	for range b.N {
		var fold ToolCallFold
		for _, delta := range deltas {
			fold.Add(StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta})
		}
		if !fold.Completed()[0].Complete {
			b.Fatal("arguments never completed")
		}
	}
}
