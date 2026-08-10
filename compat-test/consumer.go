// Package compattest is an external consumer of github.com/open-ai-sdk/ai-go.
// Every type it names comes from public packages; it imports nothing under
// internal/. The compile guard covers both the aikit vocabulary and the public
// agent runtime.
package compattest

import (
	"context"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/anthropic"
	"github.com/open-ai-sdk/ai-go/provider/gemini"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

// fakeModel is a hand-written llm.Model built entirely from the public
// surface. The compile-time interface assertion below is the proof.
type fakeModel struct{}

func (fakeModel) ModelID() string { return "fake" }

func (fakeModel) Stream(_ context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	ch := make(chan aikit.StreamEvent, 2)
	ch <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: "hello"}
	ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
	close(ch)
	return ch, nil
}

// Compile-time proof an external consumer can implement the model interface.
var _ llm.Model = fakeModel{}

// nativeFake proves providers can add the optional native completion
// capability without changing the minimal streaming model contract.
type nativeFake struct{ fakeModel }

type rawResponse struct{ RequestID string }

func (nativeFake) Complete(_ context.Context, _ llm.Request) (*llm.CompletionResponse, error) {
	raw := rawResponse{RequestID: "req_external"}
	return &llm.CompletionResponse{
		Message:     aikit.AssistantMessage("native"),
		MessageID:   "msg_external",
		Text:        "native",
		RawResponse: raw,
	}, nil
}

var _ ai.CompletionModel = nativeFake{}

// NativeCompletion exercises native payload access through public APIs only.
func NativeCompletion(ctx context.Context) (string, bool, error) {
	response, err := ai.NewCompletion(nativeFake{}, "hello").Send(ctx)
	if err != nil {
		return "", false, err
	}
	_, typed := ai.RawResponseAs[rawResponse](response)
	return response.MessageID, typed, nil
}

type fakeEmbeddingModel struct{}

func (fakeEmbeddingModel) ModelID() string { return "fake-embedding" }
func (fakeEmbeddingModel) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 2, 3}, nil
}

func (fakeEmbeddingModel) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for index := range result {
		result[index] = []float32{float32(index)}
	}
	return result, nil
}

var _ llm.EmbeddingModel = fakeEmbeddingModel{}

// Embed exercises the embedding façade with llm-owned request/result types.
func Embed(ctx context.Context) ([]float32, error) {
	result, err := ai.Embed(ctx, llm.EmbedRequest{Model: fakeEmbeddingModel{}, Text: "hello"})
	return result.Embedding, err
}

// RunAgent proves an external module can build and execute the canonical
// Agent/Runner API without importing the facade or any internal package.
func RunAgent(ctx context.Context) (*agent.Result, error) {
	configured, err := agent.New(fakeModel{}).Build()
	if err != nil {
		return nil, err
	}
	return configured.Runner().Prompt("hello").Run(ctx)
}

type externalCompat struct{}

func (externalCompat) BaseURL() string { return "https://example.com/v1" }
func (externalCompat) AuthHeader(key string) (string, string) {
	return "Authorization", "Bearer " + key
}

var _ openaicompat.Compat = externalCompat{}

// CompatibleModel proves a separate module can construct the generic provider
// without importing any internal package.
func CompatibleModel() llm.Model {
	return openaicompat.NewModel(openaicompat.Config{
		Provider: externalCompat{},
		ModelID:  "external-model",
		APIKey:   "key",
	})
}

var (
	_ llm.LanguageProvider = anthropic.NewProvider(anthropic.Config{})
	_ llm.LanguageProvider = gemini.NewProvider(gemini.Config{})
	_ llm.ImageProvider    = gemini.NewProvider(gemini.Config{})
)

// RegistryModels proves an external module can register providers and resolve
// only the capabilities they expose.
func RegistryModels() (llm.Model, llm.ImageModel, error) {
	registry := ai.NewRegistry()
	if err := registry.Register(gemini.NewProvider(gemini.Config{})); err != nil {
		return nil, nil, err
	}
	language, err := registry.LanguageModel("gemini", "gemini-language")
	if err != nil {
		return nil, nil, err
	}
	image, err := registry.ImageModel("gemini", "gemini-image")
	return language, image, err
}
