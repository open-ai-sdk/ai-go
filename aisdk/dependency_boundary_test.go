package aisdk

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestDependencyBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		files = append(files, file)
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			importSpec, ok := node.(*ast.ImportSpec)
			if !ok {
				return true
			}
			path, _ := strconv.Unquote(importSpec.Path.Value)
			if strings.HasPrefix(path, "github.com/open-ai-sdk/ai-go/") &&
				path != "github.com/open-ai-sdk/ai-go/aikit" {
				t.Errorf("aisdk imports forbidden internal package %q", path)
			}
			first := strings.Split(path, "/")[0]
			if strings.Contains(first, ".") && path != "github.com/open-ai-sdk/ai-go/aikit" {
				t.Errorf("aisdk imports non-stdlib dependency %q", path)
			}
			return false
		})
	}
}
