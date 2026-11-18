package llm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// VideoModel generates videos. Video generation is long-running, so results
// are returned as provider-hosted URLs rather than downloaded bytes.
type VideoModel interface {
	ModelID() string
	GenerateVideo(context.Context, GenerateVideoRequest) (*GenerateVideoResult, error)
}

// GenerateVideoRequest is the input to [VideoModel.GenerateVideo].
type GenerateVideoRequest struct {
	Model  VideoModel
	Prompt string
	// ImageURLs are optional reference or first-frame image URLs.
	ImageURLs []string
	// Duration is the requested clip length; zero leaves the provider default.
	Duration time.Duration
	// AspectRatio is a provider-defined ratio such as "16:9"; empty leaves the
	// provider default.
	AspectRatio     string
	ProviderOptions map[string]any
}

// GenerateVideoResult holds a video-generation result.
type GenerateVideoResult struct {
	// URLs are the provider-hosted result video URLs.
	URLs     []string
	Usage    *aikit.Usage
	Warnings []aikit.Warning
	// Raw retains the untranslated successful provider response for diagnostics.
	// It may contain sensitive provider data and is never logged automatically.
	Raw json.RawMessage `json:"-"`
}
