# otelagent

Optional OpenTelemetry adapter for `github.com/open-ai-sdk/ai-go/agent.Tracer`.

This is a regular package in the `github.com/open-ai-sdk/ai-go` module. Import it
only when the application wants to adapt an OpenTelemetry tracer to the core
runtime's provider-neutral tracing seam.
