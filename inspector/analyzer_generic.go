package inspector

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// The generic adapter derives metrics from any tree-sitter grammar without a
// hand-written spec. The curated kind sets below are a superset covering the
// bundled grammars (Rust, Java, C/C++, C#, Ruby, PHP, Bash, Scala, ...) plus the
// common conventions used across the wider tree-sitter ecosystem, so newly
// registered grammars get best-effort metrics. Accuracy varies by language;
// callers can refine via LanguageHints.

var genericFunctionKinds = []string{
	"function_item", "function_definition", "function_declaration",
	"function_expression", "method_definition", "method_declaration",
	"method", "singleton_method", "constructor_declaration",
	"local_function_statement", "arrow_function", "lambda_expression",
	"closure_expression", "anonymous_function", "anonymous_function_creation_expression",
	"generator_function", "generator_function_declaration", "func_literal",
}

var genericDecisionKinds = []string{
	"if_statement", "if_expression", "if",
	"elif_clause", "elsif", "elif", "else_if_clause",
	"for_statement", "for_expression", "for_in_statement", "for_in_clause",
	"while_statement", "while_expression", "while",
	"do_statement", "loop_expression",
	"case_statement", "case_item", "switch_label", "switch_section",
	"when", "case_clause", "match_arm",
	"catch_clause", "except_clause",
	"conditional_expression", "ternary_expression", "guard",
}

var genericNestingKinds = []string{
	"if_statement", "if_expression", "if",
	"for_statement", "for_expression", "for_in_statement",
	"while_statement", "while_expression", "while",
	"do_statement", "loop_expression",
	"switch_statement", "switch_expression", "match_expression",
	"match_block", "case_block",
	"catch_clause", "except_clause",
	"conditional_expression", "ternary_expression",
}

var genericFlatKinds = []string{
	"else_clause", "else_if_clause", "elif_clause", "elsif", "elif",
}

var genericImportKinds = []string{
	"import_statement", "import_declaration", "import_from_statement",
	"future_import_statement", "use_declaration", "using_directive",
	"namespace_use_declaration", "preproc_include", "include_statement",
	"package_clause", "package_declaration", "extern_crate_declaration",
}

// genericBooleanKinds are node kinds whose operator is inspected to decide
// whether they are short-circuit boolean operators.
var genericBooleanKinds = map[string]struct{}{
	"binary_expression": {}, "binary": {}, "boolean_operator": {},
	"infix_expression": {}, "logical_expression": {},
}

var genericBooleanOperators = map[string]struct{}{
	"&&": {}, "||": {}, "??": {}, "and": {}, "or": {},
}

func kindSet(defaults, extra []string) map[string]struct{} {
	set := make(map[string]struct{}, len(defaults)+len(extra))
	for _, k := range defaults {
		set[k] = struct{}{}
	}
	for _, k := range extra {
		set[k] = struct{}{}
	}
	return set
}

func hintSlice(h *LanguageHints, pick func(*LanguageHints) []string) []string {
	if h == nil {
		return nil
	}
	return pick(h)
}

// genericSpec builds a heuristic tsSpec for an arbitrary grammar. Hints (if
// supplied) augment the curated defaults.
func genericSpec(name string, grammar *sitter.Language, hints *LanguageHints) tsSpec {
	functionSet := kindSet(genericFunctionKinds, hintSlice(hints, func(h *LanguageHints) []string { return h.FunctionKinds }))
	decisionSet := kindSet(genericDecisionKinds, hintSlice(hints, func(h *LanguageHints) []string { return h.DecisionKinds }))
	nestingSet := kindSet(genericNestingKinds, hintSlice(hints, func(h *LanguageHints) []string { return h.NestingKinds }))
	flatSet := kindSet(genericFlatKinds, nil)
	importSet := kindSet(genericImportKinds, hintSlice(hints, func(h *LanguageHints) []string { return h.ImportKinds }))

	nameField := "name"
	paramsField := "parameters"
	if hints != nil {
		if hints.NameField != "" {
			nameField = hints.NameField
		}
		if hints.ParamsField != "" {
			paramsField = hints.ParamsField
		}
	}

	booleanDecision := func(n *sitter.Node, src []byte) bool {
		if _, ok := genericBooleanKinds[n.Kind()]; !ok {
			return false
		}
		return hasBooleanOperator(n, src)
	}

	return tsSpec{
		language: name,
		grammar:  grammar,
		isFunction: func(t string) bool {
			_, ok := functionSet[t]
			return ok
		},
		functionName: func(n *sitter.Node, src []byte) (string, string) {
			label := genericFunctionName(n, src, nameField)
			if label == "" {
				label = "<anonymous>"
			}
			signature := ""
			if p := genericParamsNode(n, paramsField); p != nil {
				signature = p.Utf8Text(src)
			}
			return label, signature
		},
		paramCount: func(n *sitter.Node) int {
			return namedNonComment(genericParamsNode(n, paramsField))
		},
		importDelta: func(n *sitter.Node, src []byte) int {
			if _, ok := importSet[n.Kind()]; ok {
				return 1
			}
			return 0
		},
		// Generic languages contribute import counts but not dependency-graph
		// edges (resolution is language-specific; see dependency.go).
		importSpecs: func(n *sitter.Node, src []byte) []string { return nil },
		varBindings: func(n *sitter.Node, src []byte) int { return 0 },
		decision: func(n *sitter.Node, src []byte) int {
			if _, ok := decisionSet[n.Kind()]; ok {
				return 1
			}
			if booleanDecision(n, src) {
				return 1
			}
			return 0
		},
		cognitive: func(n *sitter.Node, src []byte) cogKind {
			kind := n.Kind()
			if _, ok := nestingSet[kind]; ok {
				return cogNesting
			}
			if _, ok := flatSet[kind]; ok {
				return cogFlat
			}
			if booleanDecision(n, src) {
				return cogFlat
			}
			return cogNone
		},
	}
}

// hasBooleanOperator reports whether a node has a direct short-circuit boolean
// operator child (e.g. &&, ||, ??, and, or).
func hasBooleanOperator(n *sitter.Node, src []byte) bool {
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if _, ok := genericBooleanOperators[child.Utf8Text(src)]; ok {
			return true
		}
	}
	return false
}

// genericFunctionName extracts a function's name across grammar conventions:
// the name field, a declarator chain (C/C++), or the first identifier descendant.
func genericFunctionName(n *sitter.Node, src []byte, nameField string) string {
	if nm := n.ChildByFieldName(nameField); nm != nil {
		return nm.Utf8Text(src)
	}
	cur := n
	for i := 0; i < 5; i++ {
		d := cur.ChildByFieldName("declarator")
		if d == nil {
			break
		}
		if strings.Contains(d.Kind(), "identifier") {
			return d.Utf8Text(src)
		}
		cur = d
	}
	if id := firstIdentifier(n, 3); id != nil {
		return id.Utf8Text(src)
	}
	return ""
}

func genericParamsNode(n *sitter.Node, paramsField string) *sitter.Node {
	if p := n.ChildByFieldName(paramsField); p != nil {
		return p
	}
	cur := n
	for i := 0; i < 5; i++ {
		d := cur.ChildByFieldName("declarator")
		if d == nil {
			break
		}
		if p := d.ChildByFieldName("parameters"); p != nil {
			return p
		}
		cur = d
	}
	return findKindWithin(n, []string{"parameters", "parameter_list", "formal_parameters"}, 3)
}

func firstIdentifier(n *sitter.Node, depth int) *sitter.Node {
	if depth < 0 {
		return nil
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if strings.Contains(child.Kind(), "identifier") {
			return child
		}
		if found := firstIdentifier(child, depth-1); found != nil {
			return found
		}
	}
	return nil
}

func findKindWithin(n *sitter.Node, kinds []string, depth int) *sitter.Node {
	if depth < 0 {
		return nil
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		for _, k := range kinds {
			if child.Kind() == k {
				return child
			}
		}
		if found := findKindWithin(child, kinds, depth-1); found != nil {
			return found
		}
	}
	return nil
}
