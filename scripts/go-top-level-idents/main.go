// Command go-top-level-idents prints every top-level identifier (func,
// method, type, var, const, including grouped blocks) a single Go file
// declares, one per line. Used by scripts/affected-api-tests.sh's safety
// gate, which needs the real set, not a regex guess at it.
//
// Usage: go run ./scripts/go-top-level-idents <file.go>
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go-top-level-idents <file.go>")
		os.Exit(2)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, os.Args[1], nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-top-level-idents: %v\n", err)
		os.Exit(1)
	}

	seen := make(map[string]bool)
	emit := func(name string) {
		if name == "" || name == "_" || seen[name] {
			return
		}
		seen[name] = true
		fmt.Println(name)
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			emit(d.Name.Name)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					emit(s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						emit(n.Name)
					}
				}
			}
		}
	}
}
