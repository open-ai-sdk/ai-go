package engine

import (
	"strings"
	"testing"
)

// TestNoSuchToolError_ListsAvailableTools verifies the populated branch reports
// the tools it collected instead of discarding them.
func TestNoSuchToolError_ListsAvailableTools(t *testing.T) {
	err := &NoSuchToolError{ToolName: "search", AvailableTools: []string{"add", "lookup"}}
	msg := err.Error()
	if !strings.Contains(msg, "search") {
		t.Errorf("error %q should name the unknown tool", msg)
	}
	if !strings.Contains(msg, "add") || !strings.Contains(msg, "lookup") {
		t.Errorf("error %q should list the available tools", msg)
	}
}

// TestNoSuchToolError_EmptyToolsOmitsList verifies the message stays clean when
// no tools are available.
func TestNoSuchToolError_EmptyToolsOmitsList(t *testing.T) {
	err := &NoSuchToolError{ToolName: "search"}
	if got := err.Error(); got != "unknown tool search" {
		t.Errorf("error = %q, want %q", got, "unknown tool search")
	}
}
