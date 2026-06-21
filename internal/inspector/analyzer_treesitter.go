package inspector

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// cogKind classifies how a node contributes to cognitive complexity.
type cogKind int

const (
	cogNone    cogKind = iota
	cogFlat            // flat +1 (e.g. else, boolean operator)
	cogNesting         // +1 + current depth, and increases nesting
)

// tsSpec describes how to extract metrics from one tree-sitter grammar. The
// engine in treeSitterAnalyzer is language-agnostic; all per-language knowledge
// lives in these closures.
type tsSpec struct {
	language     string
	grammar      *sitter.Language
	commentType  string
	isFunction   func(nodeType string) bool
	functionName func(n *sitter.Node, src []byte) (name, signature string)
	paramCount   func(n *sitter.Node) int
	importDelta  func(n *sitter.Node, src []byte) int
	varBindings  func(n *sitter.Node, src []byte) int
	decision     func(n *sitter.Node, src []byte) int // cyclomatic delta
	cognitive    func(n *sitter.Node, src []byte) cogKind
}

type treeSitterAnalyzer struct {
	spec tsSpec
}

func newTreeSitterAnalyzer(spec tsSpec) treeSitterAnalyzer {
	return treeSitterAnalyzer{spec: spec}
}

func (a treeSitterAnalyzer) Analyze(source []byte) (*FileMetrics, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(a.spec.grammar)

	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	metrics := &FileMetrics{Language: a.spec.language}
	comments := make([]lineSpan, 0)
	a.collect(tree.RootNode(), source, metrics, &comments)

	metrics.CodeLines, metrics.CommentLines, metrics.BlankLines = lineClassification(source, comments)
	return metrics, nil
}

// collect walks the whole tree once, accumulating file-level counts and
// enumerating functions.
func (a treeSitterAnalyzer) collect(n *sitter.Node, src []byte, metrics *FileMetrics, comments *[]lineSpan) {
	t := n.Type()

	if t == a.spec.commentType {
		metrics.TodoCount += countTodoMarkers(n.Content(src))
		start := n.StartPoint()
		end := n.EndPoint()
		*comments = append(*comments, lineSpan{
			startRow: int(start.Row), startCol: int(start.Column),
			endRow: int(end.Row), endCol: int(end.Column),
		})
	}

	metrics.ImportCount += a.spec.importDelta(n, src)
	metrics.VariableCount += a.spec.varBindings(n, src)

	// IsNamed guards against tokens that share a type name with a node kind
	// (e.g. the `function` keyword token vs a function expression node).
	if n.IsNamed() && a.spec.isFunction(t) {
		metrics.Functions = append(metrics.Functions, a.functionInfo(n, src))
	}

	for i := 0; i < int(n.ChildCount()); i++ {
		a.collect(n.Child(i), src, metrics, comments)
	}
}

func (a treeSitterAnalyzer) functionInfo(n *sitter.Node, src []byte) FunctionInfo {
	name, signature := a.spec.functionName(n, src)
	start := n.StartPoint()
	end := n.EndPoint()
	cx := a.functionComplexity(n, src)
	return FunctionInfo{
		Name:       name,
		Signature:  signature,
		Line:       int(start.Row) + 1,
		LineCount:  int(end.Row) - int(start.Row) + 1,
		Cyclomatic: cx.cyclomatic,
		Cognitive:  cx.cognitive,
		MaxNesting: cx.maxNesting,
		Params:     a.spec.paramCount(n),
	}
}

// functionComplexity walks a function subtree, skipping nested functions (which
// are enumerated and scored on their own).
func (a treeSitterAnalyzer) functionComplexity(fn *sitter.Node, src []byte) complexity {
	c := complexity{cyclomatic: 1}

	var walk func(n *sitter.Node, depth int)
	walk = func(n *sitter.Node, depth int) {
		for i := 0; i < int(n.ChildCount()); i++ {
			child := n.Child(i)
			if a.spec.isFunction(child.Type()) {
				continue // nested function scored separately
			}

			c.cyclomatic += a.spec.decision(child, src)

			nestInc := 0
			switch a.spec.cognitive(child, src) {
			case cogNesting:
				c.cognitive += 1 + depth
				nestInc = 1
			case cogFlat:
				c.cognitive++
			}

			newDepth := depth + nestInc
			if newDepth > c.maxNesting {
				c.maxNesting = newDepth
			}
			walk(child, newDepth)
		}
	}
	walk(fn, 0)
	return c
}

// --- shared helpers -------------------------------------------------------

func namedNonComment(n *sitter.Node) int {
	if n == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if n.NamedChild(i).Type() == "comment" {
			continue
		}
		count++
	}
	return count
}

func fieldContent(n *sitter.Node, field string, src []byte) string {
	if n == nil {
		return ""
	}
	if child := n.ChildByFieldName(field); child != nil {
		return child.Content(src)
	}
	return ""
}

// --- Python ---------------------------------------------------------------

func pythonSpec() tsSpec {
	nesting := map[string]struct{}{
		"if_statement": {}, "for_statement": {}, "while_statement": {},
		"except_clause": {}, "conditional_expression": {},
	}
	flat := map[string]struct{}{
		"elif_clause": {}, "else_clause": {}, "boolean_operator": {},
	}
	decisions := map[string]struct{}{
		"if_statement": {}, "elif_clause": {}, "for_statement": {},
		"while_statement": {}, "except_clause": {}, "conditional_expression": {},
		"case_clause": {}, "for_in_clause": {}, "if_clause": {}, "boolean_operator": {},
	}

	return tsSpec{
		language:    "python",
		grammar:     python.GetLanguage(),
		commentType: "comment",
		isFunction: func(t string) bool {
			return t == "function_definition"
		},
		functionName: func(n *sitter.Node, src []byte) (string, string) {
			name := fieldContent(n, "name", src)
			if name == "" {
				name = "<anonymous>"
			}
			return name, fieldContent(n, "parameters", src)
		},
		paramCount: func(n *sitter.Node) int {
			return namedNonComment(n.ChildByFieldName("parameters"))
		},
		importDelta: func(n *sitter.Node, src []byte) int {
			switch n.Type() {
			case "import_statement", "import_from_statement", "future_import_statement":
				return 1
			}
			return 0
		},
		varBindings: func(n *sitter.Node, src []byte) int {
			switch n.Type() {
			case "assignment":
				return countTargetIdentifiers(n.ChildByFieldName("left"))
			case "named_expression":
				return 1
			}
			return 0
		},
		decision: func(n *sitter.Node, src []byte) int {
			if _, ok := decisions[n.Type()]; ok {
				return 1
			}
			return 0
		},
		cognitive: func(n *sitter.Node, src []byte) cogKind {
			if _, ok := nesting[n.Type()]; ok {
				return cogNesting
			}
			if _, ok := flat[n.Type()]; ok {
				return cogFlat
			}
			return cogNone
		},
	}
}

// countTargetIdentifiers counts simple-name binding targets, ignoring attribute
// and subscript targets (which rebind existing objects rather than declare).
func countTargetIdentifiers(n *sitter.Node) int {
	if n == nil {
		return 0
	}
	switch n.Type() {
	case "identifier":
		return 1
	case "attribute", "subscript", "comment":
		return 0
	}
	count := 0
	for i := 0; i < int(n.NamedChildCount()); i++ {
		count += countTargetIdentifiers(n.NamedChild(i))
	}
	return count
}

// --- JavaScript / TypeScript ---------------------------------------------

func javascriptSpec() tsSpec { return jsLikeSpec("javascript", javascript.GetLanguage()) }
func typescriptSpec() tsSpec { return jsLikeSpec("typescript", typescript.GetLanguage()) }
func tsxSpec() tsSpec        { return jsLikeSpec("tsx", tsx.GetLanguage()) }

func jsLikeSpec(language string, grammar *sitter.Language) tsSpec {
	functionTypes := map[string]struct{}{
		"function_declaration": {}, "function_expression": {}, "function": {},
		"arrow_function": {}, "method_definition": {},
		"generator_function": {}, "generator_function_declaration": {},
	}
	nesting := map[string]struct{}{
		"if_statement": {}, "for_statement": {}, "for_in_statement": {},
		"while_statement": {}, "do_statement": {}, "switch_statement": {},
		"catch_clause": {}, "ternary_expression": {},
	}
	decisions := map[string]struct{}{
		"if_statement": {}, "for_statement": {}, "for_in_statement": {},
		"while_statement": {}, "do_statement": {}, "switch_case": {},
		"catch_clause": {}, "ternary_expression": {},
	}
	booleanOps := map[string]struct{}{"&&": {}, "||": {}, "??": {}}

	isBooleanBinary := func(n *sitter.Node, src []byte) bool {
		if n.Type() != "binary_expression" {
			return false
		}
		_, ok := booleanOps[fieldContent(n, "operator", src)]
		return ok
	}

	return tsSpec{
		language:    language,
		grammar:     grammar,
		commentType: "comment",
		isFunction: func(t string) bool {
			_, ok := functionTypes[t]
			return ok
		},
		functionName: func(n *sitter.Node, src []byte) (string, string) {
			name := fieldContent(n, "name", src)
			if name == "" {
				name = jsDeriveName(n, src)
			}
			if name == "" {
				name = "<anonymous>"
			}
			signature := fieldContent(n, "parameters", src)
			if signature == "" {
				if p := n.ChildByFieldName("parameter"); p != nil {
					signature = "(" + p.Content(src) + ")"
				}
			}
			return name, signature
		},
		paramCount: func(n *sitter.Node) int {
			if p := n.ChildByFieldName("parameters"); p != nil {
				return namedNonComment(p)
			}
			if n.ChildByFieldName("parameter") != nil {
				return 1
			}
			return 0
		},
		importDelta: func(n *sitter.Node, src []byte) int {
			switch n.Type() {
			case "import_statement":
				return 1
			case "call_expression":
				fn := n.ChildByFieldName("function")
				if fn == nil {
					return 0
				}
				if fn.Type() == "import" {
					return 1
				}
				if fn.Type() == "identifier" && fn.Content(src) == "require" {
					return 1
				}
			}
			return 0
		},
		varBindings: func(n *sitter.Node, src []byte) int {
			if n.Type() == "variable_declarator" {
				return countJSBindingNames(n.ChildByFieldName("name"))
			}
			return 0
		},
		decision: func(n *sitter.Node, src []byte) int {
			if _, ok := decisions[n.Type()]; ok {
				return 1
			}
			if isBooleanBinary(n, src) {
				return 1
			}
			return 0
		},
		cognitive: func(n *sitter.Node, src []byte) cogKind {
			if _, ok := nesting[n.Type()]; ok {
				return cogNesting
			}
			if n.Type() == "else_clause" || isBooleanBinary(n, src) {
				return cogFlat
			}
			return cogNone
		},
	}
}

func jsDeriveName(n *sitter.Node, src []byte) string {
	parent := n.Parent()
	if parent == nil {
		return ""
	}
	switch parent.Type() {
	case "variable_declarator", "public_field_definition", "field_definition", "property_signature":
		return fieldContent(parent, "name", src)
	case "pair":
		return fieldContent(parent, "key", src)
	case "assignment_expression":
		return fieldContent(parent, "left", src)
	}
	return ""
}

func countJSBindingNames(n *sitter.Node) int {
	if n == nil {
		return 0
	}
	switch n.Type() {
	case "identifier", "shorthand_property_identifier_pattern", "shorthand_property_identifier":
		return 1
	case "member_expression", "subscript_expression", "comment":
		return 0
	}
	count := 0
	for i := 0; i < int(n.NamedChildCount()); i++ {
		count += countJSBindingNames(n.NamedChild(i))
	}
	return count
}
