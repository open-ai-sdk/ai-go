# Embeddings

Embeddings map text to numeric vectors for similarity search, clustering, and
retrieval. Providers implement the small `ai.EmbeddingModel` contract:

```go
type EmbeddingModel interface {
  ModelID() string
  Embed(context.Context, string) ([]float32, error)
  EmbedBatch(context.Context, []string) ([][]float32, error)
}
```

Use `ai.Embed` for one input:

```go
result, err := ai.Embed(ctx, ai.EmbedRequest{
  Model: model,
  Text:  "Go interfaces describe behavior.",
})
if err != nil {
  return err
}

fmt.Println(result.ModelID, len(result.Embedding))
```

Use `ai.EmbedMany` to let a provider batch several inputs. Returned vectors
remain in the same order as the input texts:

```go
result, err := ai.EmbedMany(ctx, ai.EmbedManyRequest{
  Model: model,
  Texts: []string{"first document", "second document"},
})
```

Vector dimensions, limits, normalization, and billing are model-specific.
Create the embedding model through the relevant provider integration and keep
index configuration consistent with that model.
