package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

func invokeRemoteTool(
	ctx context.Context,
	client *Client,
	serverName, remoteName, canonicalName string,
	input json.RawMessage,
) (tool.ExecutionResult, error) {
	var args map[string]any
	if len(input) > 0 && string(input) != "{}" && string(input) != "null" {
		if err := json.Unmarshal(input, &args); err != nil {
			return tool.ExecutionResult{}, &tool.InputError{
				ToolName: canonicalName,
				Input:    append(json.RawMessage(nil), input...),
				Cause:    err,
			}
		}
	}

	result, err := client.CallTool(ctx, remoteName, args)
	if err != nil {
		return tool.ExecutionResult{}, &tool.ExecutionError{
			ToolName: canonicalName,
			Cause: fmt.Errorf(
				"mcp: call %q on server %q: %w",
				remoteName,
				serverName,
				err,
			),
		}
	}
	if result.IsError {
		return tool.ExecutionResult{}, &remoteToolError{
			toolName: canonicalName,
			output:   outputFromMCP(result.Content),
			cause:    fmt.Errorf("mcp: remote tool %q on server %q returned an error", remoteName, serverName),
		}
	}
	output := outputFromMCP(result.Content)
	if len(result.StructuredContent) > 0 && json.Valid(result.StructuredContent) {
		part := aikit.JSONToolResultContent(result.StructuredContent)
		combined, err := tool.Content(append(output.Parts(), part)...)
		if err != nil {
			return tool.ExecutionResult{}, err
		}
		output = combined
	}
	return tool.ExecutionResult{
		Output: output,
		Metadata: map[string]any{"mcp": map[string]any{
			"server": serverName, "tool": remoteName,
			"structuredContent": append(json.RawMessage(nil), result.StructuredContent...),
			"meta":              result.Meta,
		}},
	}, nil
}

// ToolSetFromClients creates a tool.Set from multiple named MCP server
// clients. Each tool is given a server-qualified canonical name using the
// format sanitize(serverName) + "_" + sanitize(toolName). The returned
// tools route calls back to the correct server.
func ToolSetFromClients(clients map[string]*Client) (*tool.Set, error) {
	return ToolSetFromClientsContext(context.Background(), clients)
}

// ToolSetFromClientsContext creates a one-shot immutable rich tool snapshot,
// respecting cancellation while it discovers all server pages.
func ToolSetFromClientsContext(ctx context.Context, clients map[string]*Client) (*tool.Set, error) {
	seen := make(map[string]struct{})
	var tools []tool.Invokable

	serverNames := make([]string, 0, len(clients))
	for serverName := range clients {
		serverNames = append(serverNames, serverName)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		client := clients[serverName]
		remoteTools, err := client.ListAllTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("mcp.ToolSetFromClients: list tools from %q: %w", serverName, err)
		}

		for _, remoteTool := range remoteTools {
			canonical := QualifiedName(serverName, remoteTool.Name)

			if _, exists := seen[canonical]; exists {
				return nil, fmt.Errorf("mcp.ToolSetFromClients: duplicate tool name %q", canonical)
			}
			seen[canonical] = struct{}{}

			remote, err := tool.NewDynamicResult(
				canonical,
				remoteTool.Description,
				remoteTool.InputSchema,
				func(ctx context.Context, input json.RawMessage) (tool.ExecutionResult, error) {
					return invokeRemoteTool(
						ctx,
						client,
						serverName,
						remoteTool.Name,
						canonical,
						input,
					)
				},
			)
			if err != nil {
				return nil, fmt.Errorf("mcp.ToolSetFromClients: create tool %q: %w", canonical, err)
			}
			tools = append(tools, remote)
		}
	}

	return tool.NewSet(tools...)
}

// ToolSetFromClient creates a tool.Set from a single named MCP client.
func ToolSetFromClient(serverName string, client *Client) (*tool.Set, error) {
	return ToolSetFromClientContext(context.Background(), serverName, client)
}

// ToolSetFromClientContext is the cancellation-aware single-client form.
func ToolSetFromClientContext(ctx context.Context, serverName string, client *Client) (*tool.Set, error) {
	return ToolSetFromClientsContext(ctx, map[string]*Client{serverName: client})
}

// contentToString converts MCP content parts into a single string.
// Text parts are joined with newlines; non-text parts are noted with
// a placeholder.
func outputFromMCP(parts []ContentPart) tool.Output {
	content := make([]aikit.ToolResultContent, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			content = append(content, aikit.TextToolResultContent(part.Text))
		case "image":
			data, err := base64.StdEncoding.DecodeString(part.Data)
			if err == nil {
				content = append(content, aikit.ImageToolResultContent(data, part.MediaType))
				continue
			}
			content = append(content, aikit.JSONToolResultContent(part.JSON))
		default:
			// Future/resource/audio blocks remain structured host/model content,
			// never a fabricated human-readable placeholder.
			raw := part.JSON
			if !json.Valid(raw) {
				encoded, err := json.Marshal(part)
				if err != nil {
					return tool.Text("MCP returned unsupported content.")
				}
				raw = encoded
			}
			content = append(content, aikit.JSONToolResultContent(raw))
		}
	}
	output, err := tool.Content(content...)
	if err != nil {
		return tool.Text("MCP returned unsupported content.")
	}
	return output
}

type remoteToolError struct {
	toolName string
	output   tool.Output
	cause    error
}

func (e *remoteToolError) Error() string        { return e.cause.Error() }
func (e *remoteToolError) Unwrap() error        { return e.cause }
func (e *remoteToolError) Is(target error) bool { return target == tool.ErrExecution }
func (e *remoteToolError) ToolErrorDetails() tool.ErrorDetails {
	return tool.ErrorDetails{Kind: tool.ErrorKindExecution, ModelOutput: e.output}
}
