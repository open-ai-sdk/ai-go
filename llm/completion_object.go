package llm

// CompletionObjectResult is the decoded value and provider response from one
// direct structured completion.
type CompletionObjectResult[T any] struct {
	Object   T
	Response *CompletionResponse
}
