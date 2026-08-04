// Package extractor provides the reusable, typed structured-data extraction
// lifecycle. It intentionally owns extraction ergonomics rather than mixing
// them into direct Completion or Agent Runner APIs.
package extractor

import (
	"context"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// Builder configures a reusable Extractor. It is a value builder: each method
// returns an independent configuration.
type Builder[T any] struct {
	model           llm.Model
	instructions    string
	context         string
	settings        llm.CallSettings
	providerOptions map[string]any
	retries         int
}

// New starts building an extractor for T.
func New[T any](model llm.Model) Builder[T] { return Builder[T]{model: model} }

func (b Builder[T]) Instructions(value string) Builder[T]       { b.instructions = value; return b }
func (b Builder[T]) Context(value string) Builder[T]            { b.context = value; return b }
func (b Builder[T]) Settings(value llm.CallSettings) Builder[T] { b.settings = value; return b }
func (b Builder[T]) ProviderOptions(value map[string]any) Builder[T] {
	b.providerOptions = cloneOptions(value)
	return b
}
func (b Builder[T]) Retries(value int) Builder[T] { b.retries = value; return b }

// Build derives T's strict schema once and returns the reusable extractor.
func (b Builder[T]) Build() (*Extractor[T], error) {
	inner, err := agent.NewExtractor[T](b.model,
		agent.WithExtractorInstructions(b.instructions),
		agent.WithExtractorContext(b.context),
		agent.WithExtractorSettings(b.settings),
		agent.WithExtractorProviderOptions(b.providerOptions),
		agent.WithExtractorRetries(b.retries),
	)
	if err != nil {
		return nil, err
	}
	return &Extractor[T]{inner: inner}, nil
}

// Extractor extracts many inputs using one prepared schema and configuration.
type Extractor[T any] struct{ inner *agent.Extractor[T] }

// Result contains one extracted object and its billed usage.
type Result[T any] struct {
	Object          T
	Usage           aikit.Usage
	Attempts        int
	OutputToolCalls int
}

// ExtractionError is returned after every configured attempt fails.
type ExtractionError = agent.ExtractionError

func (e *Extractor[T]) Extract(ctx context.Context, text string) (T, error) {
	return e.inner.Extract(ctx, text)
}

func (e *Extractor[T]) ExtractWithUsage(ctx context.Context, text string) (Result[T], error) {
	return resultOf(e.inner.ExtractWithUsage(ctx, text))
}

func (e *Extractor[T]) ExtractWithHistory(
	ctx context.Context,
	text string,
	history []aikit.Message,
) (Result[T], error) {
	return resultOf(e.inner.ExtractWithHistory(ctx, text, history))
}

func resultOf[T any](value agent.ExtractionResult[T], err error) (Result[T], error) {
	return Result[T]{
		Object:          value.Object,
		Usage:           value.Usage,
		Attempts:        value.Attempts,
		OutputToolCalls: value.OutputToolCalls,
	}, err
}

func cloneOptions(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
