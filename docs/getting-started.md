# Get started

`ai-go` requires Go 1.25 or newer. Install it in an existing module:

```sh
go get github.com/open-ai-sdk/ai-go
```

Set an OpenAI API key, create one provider client, and derive a Responses API
model. Package `llm` owns direct model calls; the provider client owns
credentials and reusable HTTP resources.

```go
package main

import (
  "context"
  "fmt"
  "os"

  "github.com/open-ai-sdk/ai-go/llm"
  "github.com/open-ai-sdk/ai-go/provider/openai"
)

func main() {
  client, err := openai.NewClient(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
  })
  if err != nil { panic(err) }

  model := client.CompletionModel("gpt-5")

  result, err := llm.NewCompletion(model, "Why is the sky blue?").
    Instructions("Answer concisely.").
    Send(context.Background())
  if err != nil { panic(err) }

  fmt.Println(result.Text)
}
```

Next, learn the [provider/client model](/core/providers-and-clients) and
[direct completions](/core/completions). Then build a reusable
[Agent](/core/agents) and execute through an [Agent Runner](/core/agent-runner), or select
another [provider integration](/providers/).
