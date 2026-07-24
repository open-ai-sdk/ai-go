package ai

import (
	"context"
	"testing"
)

// TestGenerateObject_NonStructTypeReturnsError verifies a non-struct type
// argument yields an error before any model call, rather than panicking inside
// the reflection-based schema derivation.
func TestGenerateObject_NonStructTypeReturnsError(t *testing.T) {
	if _, err := GenerateObject[[]int](context.Background(), GenerateObjectRequest{}); err == nil {
		t.Error("slice type argument must return an error, not panic")
	}
	if _, err := GenerateObject[int](context.Background(), GenerateObjectRequest{}); err == nil {
		t.Error("primitive type argument must return an error, not panic")
	}
	if _, err := GenerateObject[any](context.Background(), GenerateObjectRequest{}); err == nil {
		t.Error("interface type argument must return an error, not panic")
	}
}
