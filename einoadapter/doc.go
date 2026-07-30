// Package einoadapter bridges an Eino agent to the AI SDK v7 UI message stream.
//
// It is the only part of ai-go that imports Eino. The protocol itself lives in
// aisdk, which has zero Eino dependency — CI asserts that with
// `go list -deps ./aisdk/...`, so a consumer importing only aisdk never compiles
// Eino or sonic.
//
// Contents arrive across later phases:
//
//	from-agentic-message.go  TypedAgentEvent[*schema.AgenticMessage] → the
//	                         normalized StepEvent vocabulary in aisdk
//	agent-ui-stream.go       the chunk producer and its stream lifecycle
//	approval-gate.go         the human-in-the-loop approval gate
//	approval-resume.go       stateless approval round-trip
//	tool-dispatch.go         execute a tool.BaseTool by name
//	from-message.go          the flat *schema.Message path
//
// Every file here that imports Eino belongs to a named allowlist that CI diffs, so
// widening the Eino surface is a deliberate review event rather than a drive-by
// import.
package einoadapter
