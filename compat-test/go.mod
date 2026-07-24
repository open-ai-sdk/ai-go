// Separate module (own go.mod), deliberately OUTSIDE the ai-go/internal subtree,
// so it imports the ai-go public surface exactly as a third-party consumer would.
// A module inside internal/ could import internal/... freely and would prove
// nothing; this one cannot, so its compilation is the regression guard that the
// whole consumer-facing surface is nameable and implementable from outside.
module github.com/open-ai-sdk/ai-go-compat-test

go 1.25.0

require github.com/open-ai-sdk/ai-go v0.0.0

replace github.com/open-ai-sdk/ai-go => ../
