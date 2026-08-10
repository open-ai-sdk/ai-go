package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// ImageModel generates complete image blobs synchronously.
type ImageModel interface {
	ModelID() string
	Generate(context.Context, GenerateImageRequest) (*GenerateImageResult, error)
}

// GenerateImageRequest is the input to [ImageModel.Generate].
type GenerateImageRequest struct {
	Model           ImageModel
	Prompt          string
	N               int
	AspectRatio     string
	Size            string
	Seed            *int
	Images          []ImageInput
	ProviderOptions map[string]any
}

// ImageInput represents an image used for editing.
type ImageInput struct {
	Data      []byte
	MediaType string
	URL       string
}

// GenerateImageResult holds an image-generation result.
type GenerateImageResult struct {
	Images   []GeneratedImage
	Usage    *aikit.Usage
	Warnings []aikit.Warning
	// Raw retains the untranslated successful provider response for diagnostics.
	// It may contain sensitive provider data and is never logged automatically.
	Raw json.RawMessage
}

// GeneratedImage holds one generated image.
type GeneratedImage struct {
	Data      []byte
	MediaType string
}

// Base64 returns the base64-encoded image data.
func (image GeneratedImage) Base64() string {
	return base64.StdEncoding.EncodeToString(image.Data)
}

// Bytes returns the raw image bytes.
func (image GeneratedImage) Bytes() []byte {
	return image.Data
}
