// Package jsonclone provides defensive JSON-compatible value cloning for
// request, event, and callback isolation inside ai-go.
//
// Common JSON container types use typed clone paths. Named and arbitrary typed
// containers fall back to reflection so concrete types, aliases, and cycles are
// preserved. Cloning intentionally does not use a JSON encode/decode round trip:
// doing so would reject cycles and erase Go-specific type and ownership details.
package jsonclone
