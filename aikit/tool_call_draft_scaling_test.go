package aikit

import (
	"encoding/json"
	"strings"
	"testing"
)

// referenceFold is the fold's intended semantics written the obvious, slow way:
// accumulate every delta, and after each one stop only once a complete JSON
// object or array has arrived. It is the oracle the optimized implementation is
// checked against.
//
// A plain json.Valid-per-delta fold is deliberately NOT the oracle. It shares
// the defect the structured check exists to remove: a root scalar streamed as
// "123" then "45" parses after the first delta, so an ungated fold stops there
// and silently truncates the arguments to "123".
func referenceFold(deltas []string) (args string, complete bool) {
	var accumulated strings.Builder
	for _, delta := range deltas {
		if complete || delta == "" {
			continue
		}
		accumulated.WriteString(delta)
		current := accumulated.String()
		if startsStructured(current) && json.Valid([]byte(current)) {
			complete = true
		}
	}
	return accumulated.String(), complete
}

// The optimized fold must agree with the reference on both the final arguments
// and the delta on which Complete flips. Divergence means arguments are either
// truncated or over-appended.
func TestToolCallFoldMatchesReferenceSemantics(t *testing.T) {
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
		`  {"leading":1}`,
	}
	for _, payload := range streams {
		for _, size := range []int{1, 3, 7} {
			t.Run(payload+"/"+string(rune('0'+size)), func(t *testing.T) {
				deltas := chunkString(payload, size)

				var fold ToolCallFold
				for step := range deltas {
					fold.Add(StreamEvent{
						Type: StreamEventToolCallDelta, ToolCallArgsDelta: deltas[step],
					})
					_, wantComplete := referenceFold(deltas[:step+1])
					if got := foldComplete(&fold); got != wantComplete {
						t.Fatalf("after delta %d (%q): Complete = %v, reference = %v",
							step, deltas[step], got, wantComplete)
					}
				}

				wantArgs, _ := referenceFold(deltas)
				if got := fold.Completed()[0].Args; got != wantArgs {
					t.Errorf("Args = %q, reference = %q", got, wantArgs)
				}
			})
		}
	}
}

// A root scalar has no closing token, so no prefix of it may be treated as a
// finished value. Completing on the first parsable prefix would hand the tool
// "123" when the model sent 12345.
func TestToolCallFoldNeverCompletesRootScalars(t *testing.T) {
	for _, payload := range []string{`12345`, `true`, `null`, `"a bare string"`} {
		t.Run(payload, func(t *testing.T) {
			var fold ToolCallFold
			for _, delta := range chunkString(payload, 2) {
				fold.Add(StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta})
			}
			draft := fold.Completed()[0]
			if draft.Complete {
				t.Errorf("Complete = true for the root scalar %s", payload)
			}
			if draft.Args != payload {
				t.Errorf("Args = %q, want the whole payload %q", draft.Args, payload)
			}
		})
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
// where the optimized fold and the reference semantics disagree about the
// arguments or about when they completed.
func FuzzToolCallFoldMatchesReferenceSemantics(f *testing.F) {
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
		deltas := chunkString(payload, size)
		wantArgs, wantComplete := referenceFold(deltas)

		var fold ToolCallFold
		for _, delta := range deltas {
			fold.Add(StreamEvent{Type: StreamEventToolCallDelta, ToolCallArgsDelta: delta})
		}

		drafts := fold.Completed()
		if len(drafts) == 0 {
			if wantArgs != "" {
				t.Fatalf("fold produced no draft for %q", payload)
			}
			return
		}
		if drafts[0].Complete != wantComplete {
			t.Fatalf("payload %q size %d: Complete = %v, reference = %v (args %q)",
				payload, size, drafts[0].Complete, wantComplete, drafts[0].Args)
		}
		if drafts[0].Args != wantArgs {
			t.Fatalf("payload %q size %d: Args = %q, reference = %q",
				payload, size, drafts[0].Args, wantArgs)
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
