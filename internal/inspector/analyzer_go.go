package inspector

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func analyzeGoSource(source []byte) (*FileMetrics, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	metrics := &FileMetrics{Language: "go"}
	metrics.ImportCount = len(file.Imports)

	functions := make([]FunctionInfo, 0)
	variableCount := 0

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.ValueSpec:
			for _, name := range n.Names {
				if name != nil && name.Name != "_" {
					variableCount++
				}
			}
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, lhs := range n.Lhs {
					variableCount += countDefinedIdentifiers(lhs)
				}
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				variableCount += countDefinedIdentifiers(n.Key)
				variableCount += countDefinedIdentifiers(n.Value)
			}
		case *ast.FuncDecl:
			name := n.Name.Name
			if n.Recv != nil && len(n.Recv.List) > 0 {
				receiver := renderGoExpr(n.Recv.List[0].Type)
				name = receiver + "." + name
			}
			startLine := fset.Position(n.Pos()).Line
			endLine := fset.Position(n.End()).Line
			functions = append(functions, FunctionInfo{
				Name:      name,
				Signature: buildGoFunctionSignature(n.Type),
				Line:      startLine,
				LineCount: clampLineCount(startLine, endLine),
			})
		case *ast.FuncLit:
			startLine := fset.Position(n.Pos()).Line
			endLine := fset.Position(n.End()).Line
			functions = append(functions, FunctionInfo{
				Name:      "<anonymous>",
				Signature: buildGoFunctionSignature(n.Type),
				Line:      startLine,
				LineCount: clampLineCount(startLine, endLine),
			})
		}
		return true
	})

	metrics.VariableCount = variableCount
	metrics.Functions = functions
	return metrics, nil
}

func countDefinedIdentifiers(expr ast.Expr) int {
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name != "" && e.Name != "_" {
			return 1
		}
	case *ast.ParenExpr:
		return countDefinedIdentifiers(e.X)
	}
	return 0
}

func buildGoFunctionSignature(fnType *ast.FuncType) string {
	params := renderGoFieldList(fnType.Params)
	results := renderGoFieldList(fnType.Results)

	signature := "(" + strings.Join(params, ", ") + ")"
	if len(results) == 1 {
		signature += " " + results[0]
	} else if len(results) > 1 {
		signature += " (" + strings.Join(results, ", ") + ")"
	}
	return signature
}

func renderGoFieldList(list *ast.FieldList) []string {
	if list == nil || len(list.List) == 0 {
		return nil
	}

	result := make([]string, 0, len(list.List))
	for _, field := range list.List {
		fieldType := renderGoExpr(field.Type)
		if len(field.Names) == 0 {
			result = append(result, fieldType)
			continue
		}
		for _, name := range field.Names {
			if name == nil || name.Name == "_" {
				continue
			}
			result = append(result, name.Name+" "+fieldType)
		}
	}
	return result
}

func renderGoExpr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + renderGoExpr(t.X)
	case *ast.SelectorExpr:
		return renderGoExpr(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + renderGoExpr(t.Elt)
	case *ast.MapType:
		return "map[" + renderGoExpr(t.Key) + "]" + renderGoExpr(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func" + buildGoFunctionSignature(t)
	case *ast.ChanType:
		return "chan " + renderGoExpr(t.Value)
	case *ast.IndexExpr:
		return renderGoExpr(t.X)
	case *ast.IndexListExpr:
		return renderGoExpr(t.X)
	default:
		return "unknown"
	}
}

func clampLineCount(startLine, endLine int) int {
	if startLine <= 0 || endLine < startLine {
		return 1
	}
	return endLine - startLine + 1
}
