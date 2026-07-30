// Package aisdkhttp serves an AI SDK v7 UI message stream over net/http.
//
// net/http only — no router dependency. aisdkgin adapts this for Gin, and the same
// shape works for any router.
//
// Arrives in Phase 09. Two responsibilities that the spike showed cannot live
// anywhere else:
//
//   - The `finish` chunk is emitted from this handler's defer, not from the event
//     consume loop. Eino's AsyncIterator applies no back-pressure — an abandoned
//     consumer cannot stop the run — so the consume loop is not a reliable place to
//     observe the end of a stream.
//   - The run is bounded by an event/byte budget cancelled through ctx, since
//     consumer back-pressure will never bound it.
//
// Every browser-bound error path here goes through aisdk's redaction, so a raw
// provider error cannot leak org/project/request identifiers or attacker-echoed
// prompt text to the client.
package aisdkhttp
