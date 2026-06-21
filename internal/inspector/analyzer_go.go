package inspector

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// goAnalyzer analyzes Go source using the standard library AST.
type goAnalyzer struct{}

func (goAnalyzer) Analyze(source []byte) (*FileMetrics, error) {
	return analyzeGoSource(source)
}

func analyzeGoSource(source []byte) (*FileMetrics, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments|parser.SkipObjectResolution)
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
			cx := goFunctionComplexity(n.Body)
			functions = append(functions, FunctionInfo{
				Name:       name,
				Signature:  buildGoFunctionSignature(n.Type),
				Line:       startLine,
				LineCount:  clampLineCount(startLine, endLine),
				Cyclomatic: cx.cyclomatic,
				Cognitive:  cx.cognitive,
				MaxNesting: cx.maxNesting,
				Params:     goParamCount(n.Type),
			})
		case *ast.FuncLit:
			startLine := fset.Position(n.Pos()).Line
			endLine := fset.Position(n.End()).Line
			cx := goFunctionComplexity(n.Body)
			functions = append(functions, FunctionInfo{
				Name:       "<anonymous>",
				Signature:  buildGoFunctionSignature(n.Type),
				Line:       startLine,
				LineCount:  clampLineCount(startLine, endLine),
				Cyclomatic: cx.cyclomatic,
				Cognitive:  cx.cognitive,
				MaxNesting: cx.maxNesting,
				Params:     goParamCount(n.Type),
			})
		}
		return true
	})

	metrics.VariableCount = variableCount
	metrics.Functions = functions

	comments := make([]lineSpan, 0, len(file.Comments))
	for _, group := range file.Comments {
		for _, comment := range group.List {
			start := fset.Position(comment.Pos())
			end := fset.Position(comment.End())
			comments = append(comments, lineSpan{
				startRow: start.Line - 1,
				startCol: start.Column - 1,
				endRow:   end.Line - 1,
				endCol:   end.Column - 1,
			})
			metrics.TodoCount += countTodoMarkers(comment.Text)
		}
	}
	metrics.CodeLines, metrics.CommentLines, metrics.BlankLines = lineClassification(source, comments)

	return metrics, nil
}

// goFunctionComplexity computes cyclomatic, cognitive and nesting metrics over a
// function body, skipping nested function literals (which are enumerated and
// scored as their own functions). Cognitive complexity is an approximation of
// the SonarSource metric: nesting structures cost 1 + current depth, while
// else branches and short-circuit boolean operators add a flat 1.
func goFunctionComplexity(body ast.Node) complexity {
	c := complexity{cyclomatic: 1}
	if body == nil {
		return c
	}

	depth := 0
	addedNesting := make([]bool, 0, 16)

	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			if len(addedNesting) > 0 {
				if addedNesting[len(addedNesting)-1] {
					depth--
				}
				addedNesting = addedNesting[:len(addedNesting)-1]
			}
			return true
		}

		// Nested functions are scored separately; do not descend.
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}

		nests := false
		switch s := n.(type) {
		case *ast.IfStmt:
			c.cyclomatic++
			c.cognitive += 1 + depth
			nests = true
			if _, isBlock := s.Else.(*ast.BlockStmt); isBlock {
				c.cognitive++ // flat else
			}
		case *ast.ForStmt:
			c.cyclomatic++
			c.cognitive += 1 + depth
			nests = true
		case *ast.RangeStmt:
			c.cyclomatic++
			c.cognitive += 1 + depth
			nests = true
		case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			c.cognitive += 1 + depth
			nests = true
		case *ast.CaseClause:
			if len(s.List) > 0 {
				c.cyclomatic++
			}
		case *ast.CommClause:
			if s.Comm != nil {
				c.cyclomatic++
			}
		case *ast.BinaryExpr:
			if s.Op == token.LAND || s.Op == token.LOR {
				c.cyclomatic++
				c.cognitive++
			}
		}

		if nests {
			depth++
			if depth > c.maxNesting {
				c.maxNesting = depth
			}
			addedNesting = append(addedNesting, true)
		} else {
			addedNesting = append(addedNesting, false)
		}
		return true
	})

	return c
}

func goParamCount(fnType *ast.FuncType) int {
	if fnType == nil || fnType.Params == nil {
		return 0
	}
	count := 0
	for _, field := range fnType.Params.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
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
