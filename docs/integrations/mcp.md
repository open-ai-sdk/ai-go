# Model Context Protocol

The `mcp` package initializes MCP clients, invokes remote tools, and adapts a
one-shot discovery snapshot to the same immutable `tool.Set` used by native
tools.

Use `mcp.ToolSetFromClientsContext(ctx, clients)` when discovery must respect a
caller deadline. It follows every `nextCursor`, preserves server/page order
(servers are sorted by name), and rejects cursor loops. The returned Set is a
snapshot: later server list changes do not alter it, and notification-driven
live refresh is intentionally deferred.

MCP text, images, structured content, resources, and future blocks retain
their ordered typed representation. Unsupported variants remain raw structured
content rather than placeholder strings. Response metadata and transport
details are host-only. A remote `isError` uses server-supplied content as its
safe model presentation while its operator cause remains available through
normal Go error unwrapping.

Remote arguments must be an object, `null`, or empty; scalar and array
arguments fail as `tool.InputError`. Native and MCP-discovered tools share
normal hook, approval, context, and immutable-registry behavior.
