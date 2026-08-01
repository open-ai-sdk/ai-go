// Separate module (own go.mod), deliberately OUTSIDE the ai-go/internal subtree,
// so it imports the ai-go public surface exactly as a third-party consumer would.
// A module inside internal/ could import internal/... freely and would prove
// nothing; this one cannot, so its compilation is the regression guard that the
// whole consumer-facing surface is nameable and implementable from outside.
module github.com/open-ai-sdk/ai-go-compat-test

go 1.25.0

require github.com/open-ai-sdk/ai-go v0.0.0

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/open-ai-sdk/ai-go => ../
