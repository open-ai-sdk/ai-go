# Model Context Protocol

The `mcp` package implements an MCP client with streamable HTTP transports, initialization and capability negotiation, tool discovery, and JSON-RPC tool invocation.

Discovered MCP tools are adapted to the same `tool.Set` used by native tools. That means a generation request can treat native and remotely discovered tools uniformly, and the normal typed tool errors and tool-loop controls continue to apply.

Use the `mcp` package when your application needs to discover or invoke tools exposed by an MCP server; use `tool.New` for tools that live directly in your Go process.
