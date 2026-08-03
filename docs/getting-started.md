# Get started

`ai-go` requires Go 1.25 or newer. Install it in an existing module:

```sh
go get github.com/open-ai-sdk/ai-go
```

Set an OpenAI API key, create one provider client, and derive a Responses API
model. The `ai` facade owns the common generation API; the provider client owns
credentials and reusable HTTP resources.

```go
package main

import (
  "context"
  "fmt"
  "os"

  "github.com/open-ai-sdk/ai-go/ai"
  "github.com/open-ai-sdk/ai-go/provider/openai"
)

func main() {
  client, err := openai.NewClient(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
  })
  if err != nil { panic(err) }

  model := client.CompletionModel("gpt-5")

  result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
    Model:        model,
    Instructions: "Answer concisely.",
    Messages:     []ai.Message{ai.UserMessage("Why is the sky blue?")},
  })
  if err != nil { panic(err) }

  fmt.Println(result.Text)
}
```

Continue with [agents](/core/agents), learn the
[provider/client model](/core/providers-and-clients), or select another
[provider integration](/providers/).
