// Package tracing defines the provider-neutral span interface used internally
// by the agent runtime.
//
// The core runtime supplies only a no-op implementation. Applications opt into
// a backend through agent.Tracer; the optional otelagent package provides the
// OpenTelemetry adapter.
package tracing
