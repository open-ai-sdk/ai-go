package tool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

func TestNewClientMarksDefinitionClientExecuted(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"selector": map[string]any{"type": "string"}},
	}
	client, err := tool.NewClient("readSelection", "Read the user's selection", schema)
	if err != nil {
		t.Fatal(err)
	}

	definition := client.Describe()
	if !definition.ClientExecuted {
		t.Error("NewClient must mark the definition client-executed")
	}
	if definition.Name != "readSelection" || definition.Description == "" {
		t.Errorf("definition = %#v", definition)
	}
	if definition.InputSchema["type"] != "object" {
		t.Errorf("input schema not carried: %#v", definition.InputSchema)
	}

	// The schema is cloned, so a caller mutating its map cannot reach inside.
	schema["type"] = "mutated"
	if client.Describe().InputSchema["type"] != "object" {
		t.Error("input schema must be cloned on the way in")
	}
}

// Having no handler is the mechanism, not a convention: invoking one is an
// error rather than a silent no-op.
func TestNewClientHasNoHandler(t *testing.T) {
	client, err := tool.NewClient("readSelection", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Invoke(context.Background(), []byte(`{}`)); err == nil {
		t.Error("invoking a client tool here must fail")
	} else if !errors.Is(err, tool.ErrExecution) {
		t.Errorf("error = %v, want an execution error", err)
	}
	if _, err := client.InvokeResult(context.Background(), []byte(`{}`)); err == nil {
		t.Error("the rich path must fail too")
	}
}

func TestNewClientRejectsEmptyName(t *testing.T) {
	if _, err := tool.NewClient("", "", nil); err == nil {
		t.Error("an unnamed tool cannot be declared to a model")
	}
}

// A client tool must be usable in an ordinary set alongside executed tools.
func TestNewClientJoinsToolSet(t *testing.T) {
	client, err := tool.NewClient("readSelection", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := tool.New("echo", "Echo", func(_ context.Context, in struct {
		Value string `json:"value"`
	},
	) (string, error) {
		return in.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := tool.NewSet(client, server)
	if err != nil {
		t.Fatal(err)
	}
	definitions := set.DefinitionsSnapshot()
	if len(definitions) != 2 {
		t.Fatalf("definitions = %#v", definitions)
	}
	for _, definition := range definitions {
		if definition.Name == "readSelection" && !definition.ClientExecuted {
			t.Error("the set must preserve ClientExecuted")
		}
		if definition.Name == "echo" && definition.ClientExecuted {
			t.Error("an ordinary tool must not become client-executed")
		}
	}
}
