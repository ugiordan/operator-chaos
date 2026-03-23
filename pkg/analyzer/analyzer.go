package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// AnalyzeFile parses a single Go source file and returns findings for
// potential fault-injection points such as ignored errors, goroutine
// launches, network calls, database calls, and Kubernetes API calls.
func AnalyzeFile(path string) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return inspectAST(fset, path, file), nil
}

// AnalyzeDirectory parses all Go files in the given directory and
// returns aggregated findings.
func AnalyzeDirectory(dir string) ([]Finding, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments) //nolint:staticcheck // TODO: migrate to go/packages
	if err != nil {
		return nil, fmt.Errorf("parsing directory %s: %w", dir, err)
	}

	var allFindings []Finding
	for _, pkg := range pkgs {
		for filePath, file := range pkg.Files {
			allFindings = append(allFindings, inspectAST(fset, filePath, file)...)
		}
	}

	return allFindings, nil
}

// inspectAST walks the AST of a single file and collects findings for
// ignored errors, goroutine launches, and notable call patterns.
func inspectAST(fset *token.FileSet, path string, file *ast.File) []Finding {
	var findings []Finding

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			findings = append(findings, detectIgnoredErrors(fset, path, node)...)

		case *ast.GoStmt:
			findings = append(findings, Finding{
				Type:     PatternGoroutineLaunch,
				File:     path,
				Line:     fset.Position(node.Pos()).Line,
				Detail:   "goroutine launched",
				Severity: "info",
			})

		case *ast.CallExpr:
			findings = append(findings, detectCallPatterns(fset, path, node)...)
		}

		return true
	})

	return findings
}

// detectIgnoredErrors checks whether an assignment has any blank
// identifier (_) on the left-hand side, which may indicate an ignored
// error return value.
func detectIgnoredErrors(fset *token.FileSet, path string, stmt *ast.AssignStmt) []Finding {
	var findings []Finding
	for _, lhs := range stmt.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "_" {
			findings = append(findings, Finding{
				Type:     PatternIgnoredError,
				File:     path,
				Line:     fset.Position(stmt.Pos()).Line,
				Detail:   "error assigned to blank identifier",
				Severity: "warning",
			})
		}
	}
	return findings
}

// detectCallPatterns inspects a function call expression and returns
// findings for network calls, database calls, and Kubernetes API calls.
func detectCallPatterns(fset *token.FileSet, path string, call *ast.CallExpr) []Finding {
	var findings []Finding

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	method := sel.Sel.Name

	// Case 1: direct package-level call, e.g. http.Get(...)
	if ident, ok := sel.X.(*ast.Ident); ok {
		pkg := ident.Name

		// Network calls
		if pkg == "http" && isNetworkMethod(method) {
			findings = append(findings, Finding{
				Type:     PatternNetworkCall,
				File:     path,
				Line:     fset.Position(call.Pos()).Line,
				Detail:   fmt.Sprintf("%s.%s call detected", pkg, method),
				Severity: "info",
			})
		}

		// Database calls via direct identifier (e.g. db.Query)
		if isDatabaseReceiver(pkg) {
			findings = append(findings, Finding{
				Type:     PatternDatabaseCall,
				File:     path,
				Line:     fset.Position(call.Pos()).Line,
				Detail:   fmt.Sprintf("%s.%s call detected", pkg, method),
				Severity: "info",
			})
		}

		// K8s API calls
		if pkg == "client" && isK8sMethod(method) {
			findings = append(findings, Finding{
				Type:     PatternK8sAPICall,
				File:     path,
				Line:     fset.Position(call.Pos()).Line,
				Detail:   fmt.Sprintf("K8s %s.%s call detected", pkg, method),
				Severity: "info",
			})
		}
	}

	// Case 2: chained selector, e.g. r.db.Query(...)
	// sel.X is another *ast.SelectorExpr where the field name gives us
	// the receiver (e.g. "db").
	if innerSel, ok := sel.X.(*ast.SelectorExpr); ok {
		fieldName := innerSel.Sel.Name

		if isDatabaseReceiver(fieldName) {
			findings = append(findings, Finding{
				Type:     PatternDatabaseCall,
				File:     path,
				Line:     fset.Position(call.Pos()).Line,
				Detail:   fmt.Sprintf("%s.%s call detected", fieldName, method),
				Severity: "info",
			})
		}
	}

	return findings
}

func isNetworkMethod(method string) bool {
	switch method {
	case "Get", "Post", "Do", "NewRequest":
		return true
	}
	return false
}

func isDatabaseReceiver(name string) bool {
	if name == "db" || name == "sql" || name == "DB" {
		return true
	}
	return strings.HasSuffix(name, "DB") || strings.HasSuffix(name, "Db")
}

func isK8sMethod(method string) bool {
	switch method {
	case "Get", "Create", "Update", "Delete", "Patch", "List":
		return true
	}
	return false
}
