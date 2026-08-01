package ai_test

import (
	"fmt"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

func buildLookupToolSet(n int) *ai.ToolSet {
	defs := make([]ai.ToolDefinition, n)
	for i := range defs {
		defs[i] = ai.ToolDefinition{Name: fmt.Sprintf("tool-%d", i)}
	}
	return &ai.ToolSet{Definitions: defs}
}

// TestToolSet_Lookup_HitAndMiss covers the basic Lookup contract: a
// registered name returns its definition, an unregistered name reports
// ok == false, and a nil receiver is a safe miss rather than a panic (a
// *ToolSet is carried as a possibly-nil pointer through the tool loop).
func TestToolSet_Lookup_HitAndMiss(t *testing.T) {
	ts := buildLookupToolSet(5)

	def, ok := ts.Lookup("tool-3")
	if !ok || def.Name != "tool-3" {
		t.Errorf("Lookup(tool-3) = %+v, %v", def, ok)
	}

	if _, ok := ts.Lookup("does-not-exist"); ok {
		t.Error("Lookup should report false for an unregistered name")
	}

	var nilSet *ai.ToolSet
	if _, ok := nilSet.Lookup("anything"); ok {
		t.Error("Lookup on a nil *ToolSet should report false, not panic")
	}
}

// TestToolSet_Lookup_StaleIndexFallsBackToScan proves Lookup notices when
// Definitions grew after the index was built (detected via a length
// mismatch) and falls back to a scan instead of missing a tool that is
// actually present.
func TestToolSet_Lookup_StaleIndexFallsBackToScan(t *testing.T) {
	ts := buildLookupToolSet(2)
	if _, ok := ts.Lookup("tool-0"); !ok {
		t.Fatal("expected tool-0 to be found before mutation")
	}

	// Simulate a caller appending after first use — unsupported per the
	// Lookup doc comment, but must not silently miss a tool that is actually
	// present in the (now longer) Definitions slice.
	ts.Definitions = append(ts.Definitions, ai.ToolDefinition{Name: "tool-2"})

	if _, ok := ts.Lookup("tool-2"); !ok {
		t.Error("Lookup should fall back to a scan when Definitions changed after the index was built")
	}
}

// BenchmarkToolSet_Lookup demonstrates that per-call dispatch cost stays flat
// as the number of registered tools grows — the O(1) map lookup replacing the
// former O(n) scan through Definitions.
func BenchmarkToolSet_Lookup(b *testing.B) {
	for _, n := range []int{10, 200} {
		b.Run(fmt.Sprintf("definitions=%d", n), func(b *testing.B) {
			ts := buildLookupToolSet(n)
			name := fmt.Sprintf("tool-%d", n-1) // last entry: worst case for a linear scan
			ts.Lookup(name)                     // warm the lazy index outside the timed loop

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, ok := ts.Lookup(name); !ok {
					b.Fatal("lookup miss")
				}
			}
		})
	}
}
