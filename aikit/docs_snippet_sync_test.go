package aikit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// Every Go snippet the streaming page publishes is the body of a doc* function
// in a package test, so the build fails when a snippet stops compiling. This
// closes the other half: it fails when the published text drifts from the
// function it was copied from.
func TestStreamingDocsSnippetsMatchCompilingSource(t *testing.T) {
	const docPath = "../docs/core/streaming.md"
	page, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	published := string(page)

	sources := []string{
		"../llm/streaming_doc_example_test.go",
		"../agent/streaming_doc_example_test.go",
	}
	found := 0
	for _, source := range sources {
		for name, body := range docFunctionBodies(t, source) {
			found++
			if !strings.Contains(published, body) {
				t.Errorf("%s: %s body is not published verbatim in %s.\n--- want to find ---\n%s",
					source, name, docPath, body)
			}
		}
	}
	if found == 0 {
		t.Fatalf("no doc* functions found in %v — the snippet convention moved", sources)
	}
}

// docFunctionBodies returns the dedented body of every function named doc*,
// keyed by name.
func docFunctionBodies(t *testing.T, path string) map[string]string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, source, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	bodies := make(map[string]string)
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || !strings.HasPrefix(function.Name.Name, "doc") {
			continue
		}
		// Between the braces, exclusive, then dedented by the one tab every
		// statement in a top-level function body carries.
		open := fileSet.Position(function.Body.Lbrace).Offset
		close := fileSet.Position(function.Body.Rbrace).Offset
		bodies[function.Name.Name] = dedent(string(source[open+1 : close]))
	}
	return bodies
}

func dedent(block string) string {
	lines := strings.Split(strings.Trim(block, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "\t")
	}
	return strings.Join(lines, "\n")
}
