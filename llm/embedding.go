package llm

import "context"

// EmbeddingModel generates vector embeddings.
type EmbeddingModel interface {
	ModelID() string
	Embed(context.Context, string) ([]float32, error)
	EmbedBatch(context.Context, []string) ([][]float32, error)
}

// EmbedRequest is the input for embedding a single text.
type EmbedRequest struct {
	Model EmbeddingModel
	Text  string
}

// EmbedManyRequest is the input for embedding multiple texts.
type EmbedManyRequest struct {
	Model EmbeddingModel
	Texts []string
}

// EmbedResult holds one embedding.
type EmbedResult struct {
	Embedding []float32
	ModelID   string
}

// EmbedManyResult holds embeddings in input order.
type EmbedManyResult struct {
	Embeddings [][]float32
	ModelID    string
}

// Embed generates an embedding for one text.
func Embed(ctx context.Context, request EmbedRequest) (EmbedResult, error) {
	embedding, err := request.Model.Embed(ctx, request.Text)
	if err != nil {
		return EmbedResult{}, err
	}
	return EmbedResult{Embedding: embedding, ModelID: request.Model.ModelID()}, nil
}

// EmbedMany generates embeddings for multiple texts.
func EmbedMany(ctx context.Context, request EmbedManyRequest) (EmbedManyResult, error) {
	embeddings, err := request.Model.EmbedBatch(ctx, request.Texts)
	if err != nil {
		return EmbedManyResult{}, err
	}
	return EmbedManyResult{Embeddings: embeddings, ModelID: request.Model.ModelID()}, nil
}
