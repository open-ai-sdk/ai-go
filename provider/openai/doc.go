// Package openai implements OpenAI Responses, Chat Completions, and file
// upload integrations.
//
// NewClient constructs the provider entry point and validates its configuration
// eagerly. A Client owns credentials and reusable HTTP resources, and creates
// lightweight capability-specific model handles with CompletionModel and
// ChatModel. Legacy model constructors remain available for compatibility.
package openai
