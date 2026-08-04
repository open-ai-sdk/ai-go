package mcp

import "encoding/json"

// SupportedProtocolVersions lists all MCP protocol versions this client accepts.
var SupportedProtocolVersions = []string{
	LatestProtocolVersion,
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

// Implementation identifies an MCP client or server.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities describes what the server supports.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
}

// ToolsCapability indicates server support for tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates server support for resources.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCapability indicates server support for prompts.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// InitializeResult is the server's response to an initialize request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	Instructions    string             `json:"instructions,omitempty"`
}

// MCPToolDef is a tool definition returned by a server's tools/list response.
type MCPToolDef struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
	// ServerID identifies which MCP server this tool came from.
	// Set by the caller when aggregating tools from multiple servers.
	ServerID string `json:"serverID,omitempty"`
}

// ListToolsResult is the server's response to a tools/list request.
type ListToolsResult struct {
	Tools      []MCPToolDef `json:"tools"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

// CallToolResult is the server's response to a tools/call request.
type CallToolResult struct {
	Content           []ContentPart   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	Meta              map[string]any  `json:"_meta,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

// ContentPart is a single piece of content within a tool result.
type ContentPart struct {
	Type      string `json:"type"`               // "text", "image", "resource"
	Text      string `json:"text,omitempty"`     // for type "text"
	Data      string `json:"data,omitempty"`     // for type "image" (base64)
	MediaType string `json:"mimeType,omitempty"` // for type "image"
	// JSON retains structured/future variants without converting them to a
	// lossy string placeholder. It contains an independently owned raw block.
	JSON json.RawMessage `json:"-"`
}

// UnmarshalJSON preserves the original block in addition to known fields.
func (p *ContentPart) UnmarshalJSON(value []byte) error {
	type wire ContentPart
	var decoded wire
	if err := json.Unmarshal(value, &decoded); err != nil {
		return err
	}
	*p = ContentPart(decoded)
	p.JSON = append(json.RawMessage(nil), value...)
	return nil
}

// Clone returns an independently owned content block.
func (p ContentPart) Clone() ContentPart { p.JSON = append(json.RawMessage(nil), p.JSON...); return p }
