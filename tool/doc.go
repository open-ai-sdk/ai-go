// Package tool provides typed tool authoring, dynamic tool adapters, and
// duplicate-safe tool registries.
//
// Native tools are created with [New]. Their input schema is derived from the
// handler's Go input type when the tool is constructed, while invocation uses
// JSON at the erased runtime boundary. Runtime-described tools such as MCP
// tools use [NewDynamic]. [ResultInvokable] and [ExecutionResult] add ordered
// text/JSON/image output and host-only metadata without changing the released
// [Invokable] API. [Details] produces safe model feedback; operator causes
// remain in the normal Go error chain.
package tool
