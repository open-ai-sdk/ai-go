package main

import (
	"context"
	"iter"
	"log"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/uistream/agui"
)

type exampleModel struct{}

func (exampleModel) ModelID() string { return "multi-protocol-example" }

func (exampleModel) Stream(ctx context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	events := make(chan aikit.StreamEvent, 2)
	go func() {
		defer close(events)
		for _, event := range []aikit.StreamEvent{
			{Type: aikit.StreamEventTextDelta, TextDelta: "Hello from one ai-go Agent"},
			{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop},
		} {
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

func main() {
	assistant, err := agent.New(exampleModel{}).Build()
	if err != nil {
		log.Fatal(err)
	}
	run := func(ctx context.Context, messages []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
		return assistant.Runner().Messages(messages...).Stream(ctx)
	}

	http.Handle("/ai-sdk", aisdkhttp.Handler(run))
	http.Handle("/ag-ui", aisdkhttp.HandlerFor(agui.Protocol(), run))
	log.Println("serving AI SDK v7 on :8787/ai-sdk and AG-UI on :8787/ag-ui")
	server := &http.Server{Addr: ":8787", ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
