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

## OpenAI-compatible endpoints

`openaicompat.NewEmbeddingModel` calls `POST {baseURL}/embeddings` on any
OpenAI-compatible service, including Ollama (`http://localhost:11434/v1`):

```go
model := openaicompat.NewEmbeddingModel(openaicompat.EmbeddingConfig{
  Provider:   myCompat, // implements BaseURL and AuthHeader
  ModelID:    "nomic-embed-text",
  Dimensions: 768, // optional; omitted from the request when zero
  BatchSize:  64,  // optional; zero sends one request
})
```

Results are placed by the response `index`. `EmbedBatchWithUsage` also returns
token usage summed across batches.

The default request includes `encoding_format: "float"`. For endpoints that
reject this optional field, set `EmbeddingConfig.OmitEncodingFormat = true`;
this leaves the response decoding as float vectors.

By default duplicate indices and empty vectors are errors, and a row without an
index keeps its response position. Endpoints with different documented contracts
can opt into independent controls:

```go
model := openaicompat.NewEmbeddingModel(openaicompat.EmbeddingConfig{
  Provider: myCompat,
  ModelID: "embedding-model",
  OmitEncodingFormat: true,
  AllowDuplicateIndices: true, // the last row at each index wins
  AllowEmptyVectors: true,     // accept []; missing/null vectors remain errors
  RequireIndices: true,       // every response row must carry an index
})
```

Every input must still have a result, and negative or out-of-range indices remain
errors. These controls do not enable batching: `BatchSize` remains the only
automatic splitting control, and its zero value sends the entire input at once.
