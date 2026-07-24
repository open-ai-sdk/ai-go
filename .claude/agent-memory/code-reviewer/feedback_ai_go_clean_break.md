---
name: ai-go-clean-break-review-scope
description: In ai-go v7 reviews, never recommend deprecated aliases or shims; verify provider encoders empirically instead of trusting green tests
metadata:
  type: feedback
---

When reviewing the ai-go SDK v7 protocol upgrade, do NOT recommend deprecated aliases, back-compat shims, or
re-adding removed constants. The v7 break is intentional and total.

**Why:** the SDK owner made an explicit design decision: v7 is a clean break with zero deprecated surface. Review
findings proposing aliases get rejected as noise.

**How to apply:** report the breakage and the correct forward fix (change the constructor/encoder), not a
compatibility layer. Also: `go build/vet/test` being green proves nothing about provider wire encoding —
provider encoders are unexported and unit tests only cover the happy path. Verify by standing up an
`httptest` server in the scratchpad with a `replace` directive module and printing the actual request body.

Related: [[project-ai-go-v7-upgrade]]
