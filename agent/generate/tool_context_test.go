package generate

import "testing"

func TestValidateToolsContextRequiredField(t *testing.T) {
	req := GenerateTextRequest{
		Tools: &ToolSet{
			Definitions: []ToolDefinition{{Name: "lookup", ContextSchema: map[string]any{"required": []any{"tenant"}}}},
		},
		ToolsContext: ToolsContext{"lookup": map[string]any{}},
	}
	if err := validateToolsContext(req); err == nil {
		t.Fatal("expected required context validation error")
	}
	req.ToolsContext["lookup"] = map[string]any{"tenant": "acme"}
	if err := validateToolsContext(req); err != nil {
		t.Fatal(err)
	}
}
