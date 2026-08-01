// Separate module (own go.mod), deliberately OUTSIDE the ai-go/internal subtree,
// so it imports the ai-go public surface exactly as a third-party consumer would.
// A module inside internal/ could import internal/... freely and would prove
// nothing; this one cannot, so its compilation is the regression guard that the
// whole consumer-facing surface is nameable and implementable from outside.
module github.com/open-ai-sdk/ai-go-compat-test

go 1.25.0

require github.com/open-ai-sdk/ai-go v0.0.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gowebpki/jcs v1.0.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/open-ai-sdk/ai-go => ../
