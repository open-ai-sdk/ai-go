// Package openai implements OpenAI Responses, Chat Completions, Images, and
// file upload integrations.
//
// NewProvider constructs the registry-compatible provider entry point.
// NewClient eagerly validates configuration and creates reusable model handles
// with CompletionModel, ChatModel, and ImageModel. Legacy model constructors
// remain available for compatibility.
package openai
