package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

func boolPtr(v bool) *bool { return &v }

func mcpTool(server, name string) MCPToolDef {
	return MCPToolDef{
		ServerID:    server,
		Name:        name,
		Description: name + " from " + server,
	}
}

func routingToolSet(t *testing.T, definitions ...aikit.ToolDefinition) *tool.Set {
	t.Helper()

	tools := make([]tool.Invokable, 0, len(definitions))
	for _, definition := range definitions {
		definition := definition
		dynamic, err := tool.NewDynamic(
			definition.Name,
			definition.Description,
			definition.InputSchema,
			func(context.Context, json.RawMessage) (json.RawMessage, error) {
				return json.RawMessage(definition.Name), nil
			},
		)
		if err != nil {
			t.Fatalf("new dynamic tool %q: %v", definition.Name, err)
		}
		tools = append(tools, dynamic)
	}

	set, err := tool.NewSet(tools...)
	if err != nil {
		t.Fatalf("new tool set: %v", err)
	}
	return set
}

// ── FilterToolDefs tests ─────────────────────────────────────────────

func TestFilterToolDefs_NilRouting_AllPreserved(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("browsermcp", "click"),
		mcpTool("browsermcp", "type"),
		mcpTool("serena", "find_symbol"),
	}
	got := FilterToolDefs(tools, nil)
	if len(got) != 3 {
		t.Fatalf("nil routing: expected 3 tools, got %d", len(got))
	}
}

func TestFilterToolDefs_EmptyRouting_AllPreserved(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("browsermcp", "click"),
		mcpTool("serena", "find_symbol"),
	}
	got := FilterToolDefs(tools, &Routing{})
	if len(got) != 2 {
		t.Fatalf("empty routing: expected 2 tools, got %d", len(got))
	}
}

func TestFilterToolDefs_PreferredServer_OnlyPreferredKept(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("browsermcp", "click"),
		mcpTool("browsermcp", "type"),
		mcpTool("serena", "find_symbol"),
		mcpTool("deepwiki", "ask"),
	}
	got := FilterToolDefs(tools, &Routing{
		PreferredServers: []string{"browsermcp"},
	})
	if len(got) != 2 {
		t.Fatalf("preferred=browsermcp: expected 2 tools, got %d", len(got))
	}
	for _, tool := range got {
		if tool.ServerID != "browsermcp" {
			t.Errorf("expected only browsermcp tools, got server=%s", tool.ServerID)
		}
	}
}

func TestFilterToolDefs_BlockedServer_Excluded(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("browsermcp", "click"),
		mcpTool("serena", "find_symbol"),
		mcpTool("deepwiki", "ask"),
	}
	got := FilterToolDefs(tools, &Routing{
		BlockedServers: []string{"serena"},
	})
	if len(got) != 2 {
		t.Fatalf("blocked=serena: expected 2 tools, got %d", len(got))
	}
	for _, tool := range got {
		if tool.ServerID == "serena" {
			t.Error("blocked server 'serena' should not appear in results")
		}
	}
}

func TestFilterToolDefs_PreferredAndBlocked_Combined(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("browsermcp", "click"),
		mcpTool("serena", "find_symbol"),
		mcpTool("deepwiki", "ask"),
	}
	got := FilterToolDefs(tools, &Routing{
		PreferredServers: []string{"browsermcp", "serena"},
		BlockedServers:   []string{"serena"},
	})
	if len(got) != 1 {
		t.Fatalf("preferred+blocked: expected 1 tool, got %d", len(got))
	}
	if got[0].ServerID != "browsermcp" {
		t.Errorf("expected browsermcp, got %s", got[0].ServerID)
	}
}

func TestFilterToolDefs_FallbackAllowed_WhenPreferredHasNoTools(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("serena", "find_symbol"),
		mcpTool("deepwiki", "ask"),
	}
	got := FilterToolDefs(tools, &Routing{
		PreferredServers: []string{"nonexistent"},
		FallbackAllowed:  boolPtr(true),
	})
	if len(got) != 2 {
		t.Fatalf("fallback allowed: expected 2 tools, got %d", len(got))
	}
}

func TestFilterToolDefs_FallbackDefault_WhenNilFallbackAllowed(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("serena", "find_symbol"),
		mcpTool("deepwiki", "ask"),
	}
	got := FilterToolDefs(tools, &Routing{
		PreferredServers: []string{"nonexistent"},
	})
	if len(got) != 2 {
		t.Fatalf("fallback default (nil): expected 2 tools, got %d", len(got))
	}
}

func TestFilterToolDefs_NoFallback_WhenDisabled(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("serena", "find_symbol"),
		mcpTool("deepwiki", "ask"),
	}
	got := FilterToolDefs(tools, &Routing{
		PreferredServers: []string{"nonexistent"},
		FallbackAllowed:  boolPtr(false),
	})
	if len(got) != 0 {
		t.Fatalf("fallback disabled: expected 0 tools, got %d", len(got))
	}
}

func TestFilterToolDefs_CaseInsensitive_Preferred(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("BrowserMCP", "click"),
		mcpTool("Serena", "find_symbol"),
	}
	got := FilterToolDefs(tools, &Routing{
		PreferredServers: []string{"browsermcp"},
	})
	if len(got) != 1 {
		t.Fatalf("case-insensitive preferred: expected 1 tool, got %d", len(got))
	}
	if got[0].ServerID != "BrowserMCP" {
		t.Errorf("expected BrowserMCP, got %s", got[0].ServerID)
	}
}

func TestFilterToolDefs_CaseInsensitive_Blocked(t *testing.T) {
	tools := []MCPToolDef{
		mcpTool("BrowserMCP", "click"),
		mcpTool("Serena", "find_symbol"),
	}
	got := FilterToolDefs(tools, &Routing{
		BlockedServers: []string{"SERENA"},
	})
	if len(got) != 1 {
		t.Fatalf("case-insensitive blocked: expected 1 tool, got %d", len(got))
	}
	if got[0].ServerID != "BrowserMCP" {
		t.Errorf("expected BrowserMCP, got %s", got[0].ServerID)
	}
}

func TestFilterToolDefs_EmptyToolList(t *testing.T) {
	got := FilterToolDefs(nil, &Routing{
		PreferredServers: []string{"browsermcp"},
	})
	if len(got) != 0 {
		t.Fatalf("empty input: expected 0 tools, got %d", len(got))
	}
}

// ── FilterToolSet tests ──────────────────────────────────────────────

func TestFilterToolSet_BasicPreferredFiltering(t *testing.T) {
	ts := routingToolSet(t,
		aikit.ToolDefinition{Name: QualifiedName("browsermcp", "click"), Description: "click"},
		aikit.ToolDefinition{Name: QualifiedName("browsermcp", "type"), Description: "type"},
		aikit.ToolDefinition{Name: QualifiedName("serena", "find_symbol"), Description: "find_symbol"},
		aikit.ToolDefinition{Name: QualifiedName("deepwiki", "ask"), Description: "ask"},
	)
	got := FilterToolSet(ts, &Routing{
		PreferredServers: []string{"browsermcp"},
	})
	definitions := got.DefinitionsSnapshot()
	if len(definitions) != 2 {
		t.Fatalf("preferred filtering: expected 2 tools, got %d", len(definitions))
	}
	for _, def := range definitions {
		server := ServerFromQualifiedName(def.Name)
		if server != "browsermcp" {
			t.Errorf("expected only browsermcp tools, got server=%s (name=%s)", server, def.Name)
		}
	}
}

func TestFilterToolSet_NilRouting_Passthrough(t *testing.T) {
	ts := routingToolSet(t,
		aikit.ToolDefinition{Name: QualifiedName("browsermcp", "click"), Description: "click"},
		aikit.ToolDefinition{Name: QualifiedName("serena", "find_symbol"), Description: "find_symbol"},
	)
	got := FilterToolSet(ts, nil)
	if got != ts {
		t.Fatal("nil routing should return the same ToolSet pointer")
	}
}

func TestFilterToolSet_NilToolSet(t *testing.T) {
	got := FilterToolSet(nil, &Routing{PreferredServers: []string{"x"}})
	if got != nil {
		t.Fatal("nil toolset should return nil")
	}
}

func TestFilterToolSet_BlockedServer(t *testing.T) {
	ts := routingToolSet(t,
		aikit.ToolDefinition{Name: QualifiedName("browsermcp", "click"), Description: "click"},
		aikit.ToolDefinition{Name: QualifiedName("serena", "find_symbol"), Description: "find_symbol"},
		aikit.ToolDefinition{Name: QualifiedName("deepwiki", "ask"), Description: "ask"},
	)
	got := FilterToolSet(ts, &Routing{
		BlockedServers: []string{"serena"},
	})
	definitions := got.DefinitionsSnapshot()
	if len(definitions) != 2 {
		t.Fatalf("blocked filtering: expected 2 tools, got %d", len(definitions))
	}
	for _, def := range definitions {
		server := ServerFromQualifiedName(def.Name)
		if server == "serena" {
			t.Errorf("blocked server 'serena' should not appear in results")
		}
	}
}

func TestFilterToolSet_FallbackAllowed(t *testing.T) {
	ts := routingToolSet(t,
		aikit.ToolDefinition{Name: QualifiedName("serena", "find_symbol"), Description: "find_symbol"},
		aikit.ToolDefinition{Name: QualifiedName("deepwiki", "ask"), Description: "ask"},
	)
	got := FilterToolSet(ts, &Routing{
		PreferredServers: []string{"nonexistent"},
		FallbackAllowed:  boolPtr(true),
	})
	if got.Len() != 2 {
		t.Fatalf("fallback allowed: expected 2 tools, got %d", got.Len())
	}
}

func TestFilterToolSet_NoFallback(t *testing.T) {
	ts := routingToolSet(t,
		aikit.ToolDefinition{Name: QualifiedName("serena", "find_symbol"), Description: "find_symbol"},
	)
	got := FilterToolSet(ts, &Routing{
		PreferredServers: []string{"nonexistent"},
		FallbackAllowed:  boolPtr(false),
	})
	if got.Len() != 0 {
		t.Fatalf("no fallback: expected 0 tools, got %d", got.Len())
	}
}

func TestFilterToolSet_RetainsMatchingInvokers(t *testing.T) {
	click := QualifiedName("browsermcp", "click")
	ts := routingToolSet(t,
		aikit.ToolDefinition{Name: click, Description: "click"},
		aikit.ToolDefinition{Name: QualifiedName("serena", "find_symbol"), Description: "find_symbol"},
	)

	got := FilterToolSet(ts, &Routing{PreferredServers: []string{"browsermcp"}})
	output, err := got.Invoke(context.Background(), click, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("invoke retained tool: %v", err)
	}
	if string(output) != click {
		t.Fatalf("output = %q, want %q", output, click)
	}
}
