// Package aikit defines the shared vocabulary used by models, the tool-loop
// engine, providers, and stream consumers.
//
// # Leaf-package contract
//
// Aikit depends only on the Go standard library. It owns data contracts such as
// [Message], [ModelRequest], [StreamEvent], [StepEvent], [Usage], and tool
// definitions so adjoining packages can exchange values without conversion
// layers.
//
// Types in this package are public API. Adding a field is normally additive;
// removing a field, changing its meaning, or changing an exported method
// signature is a breaking change.
package aikit
