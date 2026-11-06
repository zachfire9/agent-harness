package version

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// timeoutlessHTTPHelpers use http.DefaultClient, which has no timeout.
var timeoutlessHTTPHelpers = map[string]bool{
	"DefaultClient": true, "Get": true, "Head": true, "Post": true, "PostForm": true,
}

func TestProductionCodeDoesNotUseTimeoutlessDefaultHTTPClient(t *testing.T) {
	for _, dir := range []string{"cmd", "internal"} {
		root := filepath.Join("..", "..", dir)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			httpName := ""
			for _, spec := range file.Imports {
				if spec.Path.Value == `"net/http"` {
					httpName = "http"
					if spec.Name != nil {
						httpName = spec.Name.Name
					}
				}
			}
			if httpName == "" {
				return nil
			}
			ast.Inspect(file, func(node ast.Node) bool {
				sel, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == httpName && timeoutlessHTTPHelpers[sel.Sel.Name] {
					t.Errorf("%s: uses %s.%s, which has no timeout; use an *http.Client with Timeout set", fset.Position(sel.Pos()), httpName, sel.Sel.Name)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
