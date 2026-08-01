// Package aisdk owns the frozen AI SDK v7 UI-message-stream wire protocol.
//
// It translates aikit step events into validated chunks, writes SSE framing
// and the [DONE] terminator, parses useChat request messages, and implements
// tool-approval signatures. It deliberately depends only on aikit and the Go
// standard library so protocol tests never require a model or provider.
package aisdk
