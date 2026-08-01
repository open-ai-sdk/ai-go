package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/tool"
)

func invokeRemoteTool(
	ctx context.Context,
	client *Client,
	serverName, remoteName, canonicalName string,
	input json.RawMessage,
) (json.RawMessage, error) {
	var args map[string]any
	if len(input) > 0 && string(input) != "{}" {
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, &tool.InputError{
				ToolName: canonicalName,
				Input:    append(json.RawMessage(nil), input...),
				Cause:    err,
			}
		}
	}

	result, err := client.CallTool(ctx, remoteName, args)
	if err != nil {
		return nil, &tool.ExecutionError{
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
		return nil, &tool.ExecutionError{
			ToolName: canonicalName,
			Cause: fmt.Errorf(
				"mcp: remote tool returned error: %s",
				contentToString(result.Content),
			),
		}
	}
	return json.RawMessage(contentToString(result.Content)), nil
}

// ToolSetFromClients creates a tool.Set from multiple named MCP server
// clients. Each tool is given a server-qualified canonical name using the
// format sanitize(serverName) + "_" + sanitize(toolName). The returned
// tools route calls back to the correct server.
func ToolSetFromClients(clients map[string]*Client) (*tool.Set, error) {
	seen := make(map[string]struct{})
	var tools []tool.Invokable

	for serverName, client := range clients {
		res, err := client.ListTools(context.Background())
		if err != nil {
			return nil, fmt.Errorf("mcp.ToolSetFromClients: list tools from %q: %w", serverName, err)
		}

		for _, remoteTool := range res.Tools {
			canonical := QualifiedName(serverName, remoteTool.Name)

			if _, exists := seen[canonical]; exists {
				return nil, fmt.Errorf("mcp.ToolSetFromClients: duplicate tool name %q", canonical)
			}
			seen[canonical] = struct{}{}

			remote, err := tool.NewDynamic(
				canonical,
				remoteTool.Description,
				remoteTool.InputSchema,
				func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
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
	return ToolSetFromClients(map[string]*Client{serverName: client})
}

// contentToString converts MCP content parts into a single string.
// Text parts are joined with newlines; non-text parts are noted with
// a placeholder.
func contentToString(parts []ContentPart) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 && parts[0].Type == "text" {
		return parts[0].Text
	}

	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString("\n")
		}
		switch p.Type {
		case "text":
			b.WriteString(p.Text)
		case "image":
			fmt.Fprintf(&b, "[image: %s]", p.MediaType)
		case "resource":
			b.WriteString("[embedded resource]")
		default:
			fmt.Fprintf(&b, "[%s content]", p.Type)
		}
	}
	return b.String()
}
