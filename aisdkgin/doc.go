// Package aisdkgin exposes the AI SDK v7 stream handler as a gin.HandlerFunc.
//
// A thin adapter over aisdkhttp, kept in its own package so that importing ai-go
// does not pull Gin into a net/http-only consumer's build.
//
// Arrives in Phase 09.
package aisdkgin
