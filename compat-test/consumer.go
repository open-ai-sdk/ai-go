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
	"github.com/open-ai-sdk/ai-go/aisdk"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openaicompat"
)

// fakeModel is a hand-written ai.LanguageModel built entirely from the public
// surface. The compile-time interface assertion below is the proof.
type fakeModel struct{}

func (fakeModel) ModelID() string { return "fake" }

func (fakeModel) Stream(_ context.Context, req ai.LanguageModelRequest) (<-chan ai.StreamEvent, error) {
	ch := make(chan ai.StreamEvent, 2)
	text := "hello"
	if req.Output != nil {
		text = `{"value":"ok"}`
	}
	ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: text}
	ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
	close(ch)
	return ch, nil
}

// Compile-time proof an external consumer can implement the model interface.
var _ ai.LanguageModel = fakeModel{}

// GenerateText exercises the ergonomic blocking façade with public request
// and result contracts only.
func GenerateText(ctx context.Context) (string, error) {
	result, err := ai.GenerateText(ctx, ai.GenerateTextRequest{
		Model:    fakeModel{},
		Messages: []aikit.Message{{Role: aikit.RoleUser}},
	})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// StreamText exercises the live façade and its aggregate result view.
func StreamText(ctx context.Context) (string, error) {
	result, err := ai.StreamText(ctx, ai.GenerateTextRequest{
		Model:    fakeModel{},
		Messages: []aikit.Message{{Role: aikit.RoleUser}},
	}).Consume()
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

type generatedObject struct {
	Value string `json:"value"`
}

// GenerateObject exercises the generic structured-output façade.
func GenerateObject(ctx context.Context) (string, error) {
	result, err := ai.GenerateObject[generatedObject](ctx, ai.GenerateObjectRequest{
		Model:    fakeModel{},
		Messages: []aikit.Message{{Role: aikit.RoleUser}},
	})
	if err != nil {
		return "", err
	}
	return result.Object.Value, nil
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

// RunAgent proves an external module can configure and execute the public
// runtime directly without importing the ai facade or any internal package.
func RunAgent(ctx context.Context) <-chan aikit.StepEvent {
	return agent.Run(ctx, agent.RunParams{
		Model:   fakeModel{},
		Request: llm.NewRequest("hello").Build(),
	})
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

// fakeStreamEventer implements aisdk.StreamEventer from outside the module,
// proving the UI-stream surface is mockable — its Stream() returns the public
// aikit.StepEvent, not an unnameable internal type.
type fakeStreamEventer struct{ ch <-chan aikit.StepEvent }

func (f fakeStreamEventer) Stream() <-chan aikit.StepEvent { return f.ch }
func (fakeStreamEventer) DrainUnused()                     {}

// Compile-time proof an external consumer can implement the aisdk surface.
var _ aisdk.StreamEventer = fakeStreamEventer{}

// ConsumeFakeStream builds a StepEvent channel by hand and drives it through
// ai.NewStreamResult — both the channel element type and NewStreamResult's
// parameter were unnameable outside the module before aikit was made public.
func ConsumeFakeStream() (string, error) {
	ch := make(chan aikit.StepEvent, 3)
	ch <- aikit.StepEvent{Type: aikit.StepEventStepStart}
	ch <- aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hi"}
	ch <- aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop}
	close(ch)

	sr := ai.NewStreamResult(ch) // parameter type is aikit.StepEvent via the ai.StepEvent alias
	res, err := sr.Consume()
	if err != nil {
		return "", err
	}
	return res.Text, nil
}
