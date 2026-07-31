# Provider LOC after generic-client retargeting

Physical lines were counted with:

```sh
find provider -type f -name '*.go' ! -name '*_test.go' -print0 |
  xargs -0 wc -l
```

| Package | Original baseline | After Phase 04 | After Phase 05 |
|---|---:|---:|---:|
| anthropic | 715 | 738 | 738 |
| gemini | 2,120 | 2,142 | 1,760 |
| kie | 1,016 | 1,036 | 1,070 |
| openai | 1,405 | 1,413 | 1,225 |
| openai_compatible | 91 | 94 | 57 |
| openaichat / openaicompat | 892 | 874 | 861 |
| **Total** | **6,239** | **6,297** | **5,711** |

Phase 05 removes 586 lines from the actual post-Phase-04 tree and 528 lines
from the original 6,239-line baseline. The planned 1,200-line reduction is not
met.

The remaining large files are provider wire codecs: Anthropic Messages,
OpenAI Responses, native Gemini, and the public Chat Completions codec.
Reaching 5,039 lines in this phase would require deleting supported native
wire behavior or moving provider codecs outside `provider/`, neither of which
is an architectural reduction.

The consolidation still removes the duplicate/unreachable Gemini compatible
stack, makes `openai_compatible` smaller than 60 lines, removes OpenAI's unused
provider-level non-stream result adapter, and centralizes every in-origin HTTP
request through `transport.Client`. Kie's cross-origin result-image download
intentionally uses the configured raw `Doer` so provider credentials cannot be
attached to third-party URLs.
