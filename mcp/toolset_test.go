package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

type toolSetTransport struct {
	tools      []MCPToolDef
	callResult CallToolResult
	onMessage  func(JSONRPCMessage)
	calledName string
	calledArgs map[string]any
}

func (t *toolSetTransport) Start(context.Context) error { return nil }
func (t *toolSetTransport) Close() error                { return nil }

func (t *toolSetTransport) SetHandlers(
	onMessage func(JSONRPCMessage),
	_ func(),
	_ func(error),
) {
	t.onMessage = onMessage
}

func (t *toolSetTransport) Send(message JSONRPCMessage) error {
	if message.Request == nil {
		return nil
	}

	var result any
	switch message.Request.Method {
	case "tools/list":
		result = ListToolsResult{Tools: t.tools}
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(message.Request.Params, &params); err != nil {
			return err
		}
		t.calledName = params.Name
		t.calledArgs = params.Arguments
		result = t.callResult
	default:
		return fmt.Errorf("unexpected method %q", message.Request.Method)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	t.onMessage(NewResponse(message.Request.ID, raw))
	return nil
}

func TestToolSetFromClient_CreatesInvokableRemoteTool(t *testing.T) {
	transport := &toolSetTransport{
		tools: []MCPToolDef{{
			Name:        "search",
			Description: "search remotely",
			InputSchema: map[string]any{
				"type": "object",
			},
		}},
		callResult: CallToolResult{Content: []ContentPart{
			{Type: "text", Text: "first"},
			{Type: "text", Text: "second"},
		}},
	}
	client := NewClient(ClientConfig{Transport: transport})

	set, err := ToolSetFromClient("remote", client)
	if err != nil {
		t.Fatalf("ToolSetFromClient: %v", err)
	}
	canonical := QualifiedName("remote", "search")
	if _, ok := set.Invoker(canonical); !ok {
		t.Fatal("remote tool should be registered as an invoker")
	}
	definition, ok := set.Lookup(canonical)
	if !ok {
		t.Fatalf("missing definition %q", canonical)
	}
	if definition.Description != "search remotely" {
		t.Fatalf("description = %q, want %q", definition.Description, "search remotely")
	}

	output, err := set.Invoke(
		context.Background(),
		canonical,
		json.RawMessage(`{"query":"sdk"}`),
	)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if string(output) != "first\nsecond" {
		t.Fatalf("output = %q, want raw string output %q", output, "first\nsecond")
	}
	if transport.calledName != "search" {
		t.Fatalf("remote name = %q, want %q", transport.calledName, "search")
	}
	if transport.calledArgs["query"] != "sdk" {
		t.Fatalf("remote args = %#v, want query=sdk", transport.calledArgs)
	}
}

func TestToolSetFromClient_PreservesDuplicateNameError(t *testing.T) {
	transport := &toolSetTransport{tools: []MCPToolDef{
		{Name: "same.name"},
		{Name: "same/name"},
	}}
	client := NewClient(ClientConfig{Transport: transport})

	_, err := ToolSetFromClient("remote", client)
	want := `mcp.ToolSetFromClients: duplicate tool name "remote_same_name"`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestRemoteToolClassifiesInputAndExecutionErrors(t *testing.T) {
	transport := &toolSetTransport{
		tools: []MCPToolDef{{Name: "search"}},
		callResult: CallToolResult{
			IsError: true,
			Content: []ContentPart{{Type: "text", Text: "remote failed"}},
		},
	}
	client := NewClient(ClientConfig{Transport: transport})
	set, err := ToolSetFromClient("remote", client)
	if err != nil {
		t.Fatal(err)
	}
	name := QualifiedName("remote", "search")

	if _, err := set.Invoke(context.Background(), name, json.RawMessage(`[]`)); !errors.Is(err, tool.ErrInput) {
		t.Fatalf("array input error = %v, want ErrInput", err)
	}
	if _, err := set.Invoke(context.Background(), name, json.RawMessage(`{}`)); !errors.Is(err, tool.ErrExecution) {
		t.Fatalf("remote failure = %v, want ErrExecution", err)
	}
}
