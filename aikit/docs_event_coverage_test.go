package aikit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The streaming docs list both event vocabularies. Rig's own streaming page
// under-lists its variants, and that is the drift mode this catches: a new
// StreamEventType or StepEventType constant that nobody documented.
func TestStreamingDocsMentionEveryEventConstant(t *testing.T) {
	const docPath = "../docs/core/streaming.md"
	page, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	text := string(page)

	for _, source := range []struct{ file, enum string }{
		{"stream_event.go", "StreamEventType"},
		{"step_event.go", "StepEventType"},
	} {
		for _, name := range constantsOfType(t, source.file, source.enum) {
			if !strings.Contains(text, name) {
				t.Errorf("%s declares %s but %s never mentions it", source.file, name, docPath)
			}
		}
	}
}

// constantsOfType returns the names in the const block whose first spec is
// typed enum, which is how both event enums are declared.
func constantsOfType(t *testing.T, file, enum string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	var names []string
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		first, ok := general.Specs[0].(*ast.ValueSpec)
		if !ok {
			continue
		}
		identifier, ok := first.Type.(*ast.Ident)
		if !ok || identifier.Name != enum {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	if len(names) == 0 {
		t.Fatalf("no %s constants found in %s — the parser no longer matches the declaration", enum, file)
	}
	return names
}
