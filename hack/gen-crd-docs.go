//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
)

func main() {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "api/v1alpha1/types.go", nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("# CRD Schema Reference")
	fmt.Println()
	fmt.Println("Auto-generated from `api/v1alpha1/types.go`.")
	fmt.Println()

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			name := typeSpec.Name.Name
			if !ast.IsExported(name) {
				continue
			}

			if genDecl.Doc != nil {
				fmt.Printf("## %s\n\n", name)
				fmt.Println(strings.TrimSpace(genDecl.Doc.Text()))
				fmt.Println()
			} else {
				fmt.Printf("## %s\n\n", name)
			}

			fmt.Println("| Field | Type | JSON | Description |")
			fmt.Println("|-------|------|------|-------------|")

			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				fieldName := field.Names[0].Name
				if !ast.IsExported(fieldName) {
					continue
				}

				fieldType := exprToString(field.Type)
				jsonTag := ""
				if field.Tag != nil {
					tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
					jsonTag = tag.Get("json")
					if idx := strings.Index(jsonTag, ","); idx != -1 {
						jsonTag = jsonTag[:idx]
					}
				}

				comment := ""
				if field.Doc != nil {
					comment = strings.TrimSpace(field.Doc.Text())
					comment = strings.ReplaceAll(comment, "\n", " ")
				} else if field.Comment != nil {
					comment = strings.TrimSpace(field.Comment.Text())
					comment = strings.ReplaceAll(comment, "\n", " ")
				}

				fmt.Printf("| `%s` | `%s` | `%s` | %s |\n", fieldName, fieldType, jsonTag, comment)
			}
			fmt.Println()
		}
	}
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	default:
		return "unknown"
	}
}
