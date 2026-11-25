# fal

`provider/fal` talks to [fal.ai](https://fal.ai) through the
[fal queue API](https://docs.fal.ai/model-apis/model-endpoints/queue). Any
endpoint id is accepted, for example `fal-ai/kling-video/o3/pro/image-to-video`
or `bytedance/seedance-2.0/fast/reference-to-video`. Per-model input schemas
are published at `https://fal.ai/models/<endpoint id>/llms.txt`.

## Construct a model

```go
provider := fal.NewProvider(os.Getenv("FAL_KEY"), fal.WithConfig(fal.Config{
    Timeout:      60 * time.Second,
    PollInterval: 3 * time.Second,
    PollTimeout:  15 * time.Minute,
}))
model := provider.Video("fal-ai/kling-video/o3/pro/image-to-video")
```

`NewFromEnv()` and an empty key both read `FAL_KEY`. The key is sent as
`Authorization: Key <key>`. `BaseURL` defaults to `https://queue.fal.run`;
paths are appended beneath it, preserving any base path. `Provider` implements
`llm.VideoProvider`, so it can be registered with `ai.Registry`.

Endpoint ids need at least `owner/app` and no empty, `.` or `..` segments; an
invalid id is reported by every call. Path segments and request ids are
escaped.

## Generate a video

```go
result, err := model.GenerateVideo(ctx, llm.GenerateVideoRequest{
    Prompt:      "A paper boat drifting down a rainy street",
    ImageURLs:   []string{"https://example.com/first-frame.png"},
    Duration:    5 * time.Second,
    AspectRatio: "16:9",
    ProviderOptions: map[string]any{"fal": fal.VideoOptions{
        RawInput: map[string]any{"generate_audio": true},
    }},
})
```

The model input starts from `VideoOptions.RawInput` (JSON-map key `rawInput`).
Request fields are added only when `RawInput` does not set the key:

| Request field | Input key |
| --- | --- |
| `Prompt` | `prompt` |
| one `ImageURLs` entry | `image_url` |
| several `ImageURLs` entries | `image_urls` |
| `Duration` | `duration`, whole seconds as a string |
| `AspectRatio` | `aspect_ratio` |

Setting either image key in `RawInput` suppresses both. Models whose schema
differs (for example reference images under another key) should receive those
fields through `RawInput`.

`GenerateVideo` submits once, polls the status route every `PollInterval`
until the request completes or `PollTimeout` elapses, then fetches the result.
It returns the https `video.url` and the untranslated result body as `Raw`.
`SubmitVideo` only enqueues and returns the request id.

## Durable queue workflows

Applications that persist requests can split the lifecycle across processes.
Each raw method performs exactly one HTTP exchange and never retries:

| Method | Request |
| --- | --- |
| `SubmitReply(ctx, input)` | `POST {base}/{endpoint id}` with `input` as the JSON body |
| `StatusReply(ctx, id)` | `GET {base}/{owner}/{app}/requests/{id}/status` |
| `ResultReply(ctx, id)` | `GET {base}/{owner}/{app}/requests/{id}` |

Status and result routes use only the first two endpoint segments, as fal
requires. `QueueReply` exposes the received `StatusCode`, untranslated `Body`,
and a separate `ReadError`. `MaxReplyBytes` bounds each body,
`RequireUTF8Reply` rejects invalid UTF-8, and `DisableRedirects` returns
redirects unfollowed (an injected `HTTPClient` is copied, not mutated).

The pure decoders classify those replies:

- `DecodeSubmission` returns the `request_id`. A `*SubmitError` with
  `Rejected: true` means fal answered 4xx (including 429) and did not accept
  the request. 5xx, read errors, non-JSON bodies and a missing `request_id` are
  ambiguous (`Rejected: false`): the request may exist, so do not resubmit
  blindly.
- `DecodeStatus` reports `pending` (`IN_QUEUE`, `IN_PROGRESS`) or `completed`,
  with `Error` set when a completed request failed. It returns a
  `*QueryError` with `Contract: true` for a 404 (unknown request) or an unknown
  status value; 429, 5xx, other non-2xx answers, read errors and non-JSON
  bodies are transient.
- `DecodeVideoResult` reports `succeeded` with an https `VideoURL`, or
  `failed` for a 4xx other than 429, with a `Message` preview of at most 300
  characters (preferring the `detail` field). A 2xx reply without an https
  `video.url` is a contract error; 429, 5xx and read errors are transient.
  Fetch the result only after the status is `completed`, because fal also
  answers 4xx for a request that is still running.
