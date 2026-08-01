//go:build ignore

// Command ai-types rejects owned type declarations in the ai facade while
// permitting aliases that re-export contracts owned by lower-level packages.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir := "./ai"
	if len(os.Args) == 2 {
		dir = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: go run .github/checks/ai_types.go [ai-directory]")
		os.Exit(2)
	}

	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", dir, err)
		os.Exit(1)
	}

	failed := false
	for _, pkg := range packages {
		for filename, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					if typeSpec.Assign.IsValid() && isPackageAlias(typeSpec.Type) {
						continue
					}
					pos := fset.Position(typeSpec.Pos())
					fmt.Fprintf(os.Stderr, "%s:%d: ai declares owned type %s; alias a lower-package type instead\n",
						filepath.ToSlash(filename), pos.Line, typeSpec.Name.Name)
					failed = true
				}
			}
		}
	}
	if failed {
		os.Exit(1)
	}
}

func isPackageAlias(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		_, ok := value.X.(*ast.Ident)
		return ok
	case *ast.IndexExpr:
		return isPackageAlias(value.X)
	case *ast.IndexListExpr:
		return isPackageAlias(value.X)
	case *ast.ParenExpr:
		return isPackageAlias(value.X)
	default:
		return false
	}
}
