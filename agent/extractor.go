package agent

import (
	"context"
	"errors"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// ExtractorOption configures a reusable typed extractor.
type ExtractorOption func(*extractorConfig)

type extractorConfig struct {
	instructions    string
	context         string
	settings        llm.CallSettings
	providerOptions map[string]any
	retries         int
}

// WithExtractorContext adds durable domain context to each extraction prompt.
func WithExtractorContext(value string) ExtractorOption {
	return func(config *extractorConfig) { config.context = value }
}

// WithExtractorInstructions sets the system instructions used for each attempt.
func WithExtractorInstructions(value string) ExtractorOption {
	return func(config *extractorConfig) { config.instructions = value }
}

// WithExtractorRetries sets the number of retries after the initial attempt.
func WithExtractorRetries(value int) ExtractorOption {
	return func(config *extractorConfig) { config.retries = value }
}

// WithExtractorSettings sets common call settings for each attempt.
func WithExtractorSettings(value llm.CallSettings) ExtractorOption {
	return func(config *extractorConfig) { config.settings = value }
}

// WithExtractorProviderOptions sets provider-specific options for each attempt.
func WithExtractorProviderOptions(value map[string]any) ExtractorOption {
	return func(config *extractorConfig) { config.providerOptions = snapshotJSONMap(value) }
}

// Extractor builds a strict output schema once and reuses it for many inputs.
type Extractor[T any] struct {
	model  llm.Model
	schema map[string]any
	config extractorConfig
}

// ExtractionResult is the decoded output and observability data for an extract.
type ExtractionResult[T any] struct {
	Object          T
	Usage           aikit.Usage
	Attempts        int
	OutputToolCalls int
}

// ExtractionError retains metering information when all attempts fail.
type ExtractionError struct {
	Kind     StructuredOutputErrorKind
	Attempts int
	Usage    aikit.Usage
	Cause    error
}

func (e *ExtractionError) Error() string {
	if e == nil {
		return "agent: extraction failed"
	}
	return "agent: extraction failed: " + e.Cause.Error()
}

func (e *ExtractionError) Unwrap() error { return e.Cause }

// NewExtractor derives and retains T's strict output schema.
func NewExtractor[T any](model llm.Model, options ...ExtractorOption) (*Extractor[T], error) {
	if model == nil {
		return nil, &StructuredOutputError{Kind: StructuredOutputErrorKindPrompt, Reason: "model is nil"}
	}
	schema, err := tool.StrictSchema[T]()
	if err != nil {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindPrompt,
			Reason: "invalid output type",
			Cause:  err,
		}
	}
	config := extractorConfig{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if config.retries < 0 {
		return nil, &StructuredOutputError{
			Kind:   StructuredOutputErrorKindPrompt,
			Reason: "retries must not be negative",
		}
	}
	return &Extractor[T]{model: model, schema: schema, config: config}, nil
}

// Extract returns only the decoded object.
func (e *Extractor[T]) Extract(ctx context.Context, text string) (T, error) {
	result, err := e.ExtractWithUsage(ctx, text)
	return result.Object, err
}

// ExtractWithUsage extracts one input and returns accumulated usage.
func (e *Extractor[T]) ExtractWithUsage(ctx context.Context, text string) (ExtractionResult[T], error) {
	return e.ExtractWithHistory(ctx, text, nil)
}

// ExtractWithHistory appends text after the supplied history for each attempt.
func (e *Extractor[T]) ExtractWithHistory(
	ctx context.Context,
	text string,
	history []Message,
) (ExtractionResult[T], error) {
	var result ExtractionResult[T]
	if e == nil || e.model == nil {
		return result, &ExtractionError{Kind: StructuredOutputErrorKindPrompt, Cause: errors.New("extractor is nil")}
	}
	var last error
	for attempt := 0; attempt <= e.config.retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		messages := append(cloneMessages(history), aikit.UserMessage(text))
		response, err := llm.Complete(ctx, e.model, llm.Request{
			Instructions: extractorInstructions(e.config), Messages: messages,
			Output:   &llm.OutputSchema{Type: "object", Schema: e.schema},
			Settings: e.config.settings, ProviderOptions: snapshotJSONMap(e.config.providerOptions),
		})
		result.Attempts++
		if response != nil {
			result.Usage.Accumulate(response.Usage)
		}
		if err == nil && response != nil {
			result.Object, err = llm.DecodeStructured[T](
				response.Text,
				&llm.OutputSchema{Type: "object", Schema: e.schema},
			)
		}
		if err == nil {
			return result, nil
		}
		last = err
		var structured *StructuredOutputError
		if !errors.As(err, &structured) || structured.Kind == StructuredOutputErrorKindPrompt {
			break
		}
	}
	var structured *StructuredOutputError
	kind := StructuredOutputErrorKindPrompt
	if errors.As(last, &structured) {
		kind = structured.Kind
	}
	return result, &ExtractionError{Kind: kind, Attempts: result.Attempts, Usage: result.Usage, Cause: last}
}

func extractorInstructions(config extractorConfig) string {
	if config.context == "" {
		return config.instructions
	}
	if config.instructions == "" {
		return "Context:\n" + config.context
	}
	return config.instructions + "\n\nContext:\n" + config.context
}
