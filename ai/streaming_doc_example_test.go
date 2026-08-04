package ai_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

// docFacadeStreamSend is the streaming snippet docs/core/completions.md
// publishes. aikit/docs_snippet_sync_test.go asserts the page still matches it.
func docFacadeStreamSend(ctx context.Context, model ai.LanguageModel) error {
	stream, err := ai.NewCompletion(model, "Explain Go interfaces").StreamSend(ctx)
	if err != nil {
		return err
	}

	for event, err := range stream.Events() {
		if err != nil {
			return err
		}
		if event.Type == ai.StreamEventTextDelta {
			fmt.Print(event.TextDelta)
		}
	}

	response, err := stream.Response()
	if err != nil {
		return err
	}
	fmt.Printf("\n%d tokens\n", response.Usage.TotalTokens)
	return nil
}

func TestDocumentedFacadeSnippetsRun(t *testing.T) {
	if err := docFacadeStreamSend(context.Background(), facadeStreamModel{}); err != nil {
		t.Errorf("docFacadeStreamSend() error = %v", err)
	}
}
