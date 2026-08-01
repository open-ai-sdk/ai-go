// Package aisdkgin adapts aisdkhttp handlers to gin without adding gin to the
// core ai-go module.
package aisdkgin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes a net/http handler as a gin handler.
func Handler(handler http.Handler) gin.HandlerFunc {
	return gin.WrapH(handler)
}

// Wrap exposes any net/http handler as a gin handler.
func Wrap(handler http.Handler) gin.HandlerFunc {
	return Handler(handler)
}
