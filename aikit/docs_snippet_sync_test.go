package aikit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Pages whose Go snippets are published from compiling source.
var snippetPages = []string{
	"../docs/core/streaming.md",
	"../docs/core/completions.md",
	"../docs/core/agent-runner.md",
}

// Package tests holding those snippets as doc* functions.
var snippetSources = []string{
	"../llm/streaming_doc_example_test.go",
	"../agent/streaming_doc_example_test.go",
	"../ai/streaming_doc_example_test.go",
}

// Blocks on the streaming page that deliberately do not compile, keyed by a
// distinctive line. Only the migration "before" snippet qualifies: it calls the
// method this release removed, which is the whole point of showing it.
var uncompilableBlocks = []string{
	`.Stream(ctx)`,
}

// Forward direction: every doc* function body is published verbatim somewhere.
// The build already proves those functions compile; this proves the pages did
// not drift from them.
func TestStreamingDocsSnippetsMatchCompilingSource(t *testing.T) {
	pages := make(map[string]string, len(snippetPages))
	for _, path := range snippetPages {
		page, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		pages[path] = string(page)
	}

	found := 0
	for _, source := range snippetSources {
		bodies := docFunctionBodies(t, source)
		if len(bodies) == 0 {
			t.Errorf("%s declares no doc* functions — the snippet convention moved", source)
		}
		for name, body := range bodies {
			if strings.TrimSpace(body) == "" {
				t.Errorf("%s: %s has an empty body, which would match any page vacuously", source, name)
				continue
			}
			found++
			if !publishedAnywhere(pages, body) {
				t.Errorf("%s: %s is not published verbatim in any of %v.\n--- want to find ---\n%s",
					source, name, snippetPages, body)
			}
		}
	}
	if found == 0 {
		t.Fatalf("no doc* functions found in %v — the snippet convention moved", snippetSources)
	}
}

// Reverse direction, for the page this release rewrote end to end: every Go
// block on it comes from a doc* function, or is explicitly listed as
// uncompilable. Without this, a hand-written snippet can be added to the page
// and no test notices.
func TestStreamingPageHasNoUncheckedGoSnippets(t *testing.T) {
	const path = "../docs/core/streaming.md"
	page, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	bodies := map[string]string{}
	for _, source := range snippetSources {
		for name, body := range docFunctionBodies(t, source) {
			bodies[source+":"+name] = body
		}
	}

	for i, block := range goCodeBlocks(string(page)) {
		if matchesAnyBody(bodies, block) || isUncompilable(block) {
			continue
		}
		t.Errorf("%s: Go block %d is neither copied from a doc* function nor listed as "+
			"uncompilable.\n--- block ---\n%s", path, i+1, block)
	}
}

func publishedAnywhere(pages map[string]string, body string) bool {
	for _, page := range pages {
		if strings.Contains(page, body) {
			return true
		}
	}
	return false
}

func matchesAnyBody(bodies map[string]string, block string) bool {
	for _, body := range bodies {
		if strings.TrimSpace(body) != "" && strings.Contains(body, strings.TrimSpace(block)) {
			return true
		}
	}
	return false
}

func isUncompilable(block string) bool {
	for _, marker := range uncompilableBlocks {
		if strings.Contains(block, marker) {
			return true
		}
	}
	return false
}

var goFence = regexp.MustCompile("(?s)```go\n(.*?)\n```")

func goCodeBlocks(page string) []string {
	matches := goFence.FindAllStringSubmatch(page, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, match[1])
	}
	return blocks
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
		shut := fileSet.Position(function.Body.Rbrace).Offset
		bodies[function.Name.Name] = dedent(string(source[open+1 : shut]))
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
