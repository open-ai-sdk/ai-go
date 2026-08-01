# Get started

`ai-go` requires Go 1.25 or newer. Install it in an existing module:

```sh
go get github.com/open-ai-sdk/ai-go
```

Set an OpenAI API key and create a Responses API model. The `ai` facade owns the common generation API; the provider package owns the provider-specific constructor and options.

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
  model := openai.NewLanguageModel("gpt-5", openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
  })

  result, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
    Model:        model,
    Instructions: "Answer concisely.",
    Messages:     []ai.Message{ai.UserMessage("Why is the sky blue?")},
  })
  if err != nil { panic(err) }

  fmt.Println(result.Text)
}
```

Continue with [text generation](/core/generate-text), or replace the provider with one from the [provider guide](/providers/).
