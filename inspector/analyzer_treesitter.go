package inspector

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
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
// lives in these closures. Closures receive the node's kind string pre-resolved
// (interned via idToKind) so the hot walk never allocates one Kind() string per
// node call site.
type tsSpec struct {
	language     string
	grammar      *sitter.Language
	idToKind     map[uint16]string
	isFunction   func(kind string) bool
	functionName func(n *sitter.Node, src []byte) (name, signature string)
	paramCount   func(n *sitter.Node) int
	importDelta  func(kind string, n *sitter.Node, src []byte) int
	importSpecs  func(kind string, n *sitter.Node, src []byte) []string
	varBindings  func(kind string, n *sitter.Node, src []byte) int
	decision     func(kind string, n *sitter.Node, src []byte) int // cyclomatic delta
	cognitive    func(kind string, n *sitter.Node, src []byte) cogKind
}

type treeSitterAnalyzer struct {
	spec tsSpec
}

func newTreeSitterAnalyzer(spec tsSpec) treeSitterAnalyzer {
	spec.idToKind = buildIDToKind(spec.grammar)
	return treeSitterAnalyzer{spec: spec}
}

// buildIDToKind maps every symbol id of a grammar to its (interned) kind name,
// built once at registration so the per-node walk can resolve a kind without a
// cgo GoString allocation.
func buildIDToKind(grammar *sitter.Language) map[uint16]string {
	if grammar == nil {
		return map[uint16]string{}
	}
	count := grammar.NodeKindCount()
	m := make(map[uint16]string, count)
	for i := uint32(0); i < count && i <= 0xFFFF; i++ {
		id := uint16(i)
		if name := grammar.NodeKindForId(id); name != "" {
			m[id] = name
		}
	}
	return m
}

func (a treeSitterAnalyzer) Analyze(source []byte) (*FileMetrics, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(a.spec.grammar); err != nil {
		return nil, err
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		return &FileMetrics{Language: a.spec.language}, nil
	}
	defer tree.Close()

	w := &tsWalk{spec: &a.spec, src: source, fileH: newHalstead()}
	w.metrics = &FileMetrics{Language: a.spec.language}
	w.walk(tree.RootNode(), 0)

	m := w.metrics
	m.CodeLines, m.CommentLines, m.BlankLines = lineClassification(source, w.comments)
	m.Halstead = w.fileH.metrics()

	fileCyclomatic := 0
	for _, fn := range m.Functions {
		fileCyclomatic += fn.Cyclomatic
	}
	m.Maintainability = maintainabilityIndex(m.Halstead.Volume, fileCyclomatic, m.CodeLines)
	return m, nil
}

// tsWalk holds the state for a single depth-first pass that computes file-level
// metrics and per-function metrics together (no second walk).
type tsWalk struct {
	spec     *tsSpec
	src      []byte
	metrics  *FileMetrics
	comments []lineSpan
	fileH    *halsteadAccumulator
	stack    []*funcFrame // enclosing functions; innermost is last
}

// funcFrame accumulates one function's metrics while its subtree is walked.
type funcFrame struct {
	cyclomatic int
	cognitive  int
	maxNesting int
	halstead   *halsteadAccumulator
}

func (w *tsWalk) innermost() *funcFrame {
	if len(w.stack) == 0 {
		return nil
	}
	return w.stack[len(w.stack)-1]
}

// walk visits n at the given control-flow nesting depth (relative to the
// innermost enclosing function). depth is meaningful only inside a function.
func (w *tsWalk) walk(n *sitter.Node, depth int) {
	spec := w.spec
	src := w.src
	kind := spec.idToKind[n.KindId()]
	named := n.IsNamed()
	isComment := isCommentKind(kind)

	if isComment {
		w.metrics.TodoCount += countTodoMarkers(n.Utf8Text(src))
		start := n.StartPosition()
		end := n.EndPosition()
		w.comments = append(w.comments, lineSpan{
			startRow: int(start.Row), startCol: int(start.Column),
			endRow: int(end.Row), endCol: int(end.Column),
		})
	}

	// Halstead: classify leaf tokens, attributing to the file and the innermost
	// enclosing function.
	if !isComment && n.ChildCount() == 0 {
		top := w.innermost()
		if named {
			startByte, endByte := n.ByteRange()
			operand := src[startByte:endByte]
			w.fileH.addOperandBytes(operand)
			if top != nil {
				top.halstead.addOperandBytes(operand)
			}
		} else if strings.TrimSpace(kind) != "" {
			w.fileH.addOperator(kind)
			if top != nil {
				top.halstead.addOperator(kind)
			}
		}
	}

	// File-level counts.
	w.metrics.ImportCount += spec.importDelta(kind, n, src)
	w.metrics.Imports = append(w.metrics.Imports, spec.importSpecs(kind, n, src)...)
	w.metrics.VariableCount += spec.varBindings(kind, n, src)

	// A function opens a new frame; its body is scored independently and nested
	// functions are scored on their own (the parent frame gets nothing from them).
	if named && spec.isFunction(kind) {
		frame := &funcFrame{cyclomatic: 1, halstead: newHalstead()}
		w.stack = append(w.stack, frame)
		for i := uint(0); i < n.ChildCount(); i++ {
			w.walk(n.Child(i), 0)
		}
		w.stack = w.stack[:len(w.stack)-1]
		w.metrics.Functions = append(w.metrics.Functions, w.finishFunction(n, src, frame))
		return
	}

	// Attribute complexity to the innermost enclosing function.
	childDepth := depth
	if top := w.innermost(); top != nil {
		top.cyclomatic += spec.decision(kind, n, src)
		switch spec.cognitive(kind, n, src) {
		case cogNesting:
			top.cognitive += 1 + depth
			childDepth = depth + 1
			if childDepth > top.maxNesting {
				top.maxNesting = childDepth
			}
		case cogFlat:
			top.cognitive++
		}
	}

	for i := uint(0); i < n.ChildCount(); i++ {
		w.walk(n.Child(i), childDepth)
	}
}

func (w *tsWalk) finishFunction(n *sitter.Node, src []byte, f *funcFrame) FunctionInfo {
	name, signature := w.spec.functionName(n, src)
	start := n.StartPosition()
	end := n.EndPosition()
	lineCount := int(end.Row) - int(start.Row) + 1
	h := f.halstead.metrics()
	return FunctionInfo{
		Name:            name,
		Signature:       signature,
		Line:            int(start.Row) + 1,
		LineCount:       lineCount,
		Cyclomatic:      f.cyclomatic,
		Cognitive:       f.cognitive,
		MaxNesting:      f.maxNesting,
		Params:          w.spec.paramCount(n),
		Maintainability: maintainabilityIndex(h.Volume, f.cyclomatic, lineCount),
	}
}

// --- shared helpers -------------------------------------------------------

func namedNonComment(n *sitter.Node) int {
	if n == nil {
		return 0
	}
	count := 0
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if isCommentKind(n.NamedChild(uint(i)).Kind()) {
			continue
		}
		count++
	}
	return count
}

// isCommentKind reports whether a tree-sitter node kind denotes a comment.
// Grammars use "comment", "line_comment", "block_comment", etc.
func isCommentKind(kind string) bool {
	return kind == "comment" || strings.Contains(kind, "comment")
}

func fieldContent(n *sitter.Node, field string, src []byte) string {
	if n == nil {
		return ""
	}
	if child := n.ChildByFieldName(field); child != nil {
		return child.Utf8Text(src)
	}
	return ""
}

func stripQuotes(s string) string {
	return strings.Trim(s, "\"'`")
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
		language: "python",
		grammar:  sitter.NewLanguage(tspython.Language()),
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
		importDelta: func(kind string, n *sitter.Node, src []byte) int {
			switch kind {
			case "import_statement", "import_from_statement", "future_import_statement":
				return 1
			}
			return 0
		},
		importSpecs: func(kind string, n *sitter.Node, src []byte) []string {
			switch kind {
			case "import_statement":
				var specs []string
				for i := 0; i < int(n.NamedChildCount()); i++ {
					child := n.NamedChild(uint(i))
					switch child.Kind() {
					case "dotted_name", "relative_import":
						specs = append(specs, child.Utf8Text(src))
					case "aliased_import":
						if name := child.ChildByFieldName("name"); name != nil {
							specs = append(specs, name.Utf8Text(src))
						}
					}
				}
				return specs
			case "import_from_statement":
				if module := n.ChildByFieldName("module_name"); module != nil {
					return []string{module.Utf8Text(src)}
				}
			}
			return nil
		},
		varBindings: func(kind string, n *sitter.Node, src []byte) int {
			switch kind {
			case "assignment":
				return countTargetIdentifiers(n.ChildByFieldName("left"))
			case "named_expression":
				return 1
			}
			return 0
		},
		decision: func(kind string, n *sitter.Node, src []byte) int {
			if _, ok := decisions[kind]; ok {
				return 1
			}
			return 0
		},
		cognitive: func(kind string, n *sitter.Node, src []byte) cogKind {
			if _, ok := nesting[kind]; ok {
				return cogNesting
			}
			if _, ok := flat[kind]; ok {
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
	switch n.Kind() {
	case "identifier":
		return 1
	case "attribute", "subscript", "comment":
		return 0
	}
	count := 0
	for i := 0; i < int(n.NamedChildCount()); i++ {
		count += countTargetIdentifiers(n.NamedChild(uint(i)))
	}
	return count
}

// --- JavaScript / TypeScript ---------------------------------------------

func javascriptSpec() tsSpec {
	return jsLikeSpec("javascript", sitter.NewLanguage(tsjs.Language()))
}
func typescriptSpec() tsSpec {
	return jsLikeSpec("typescript", sitter.NewLanguage(tsts.LanguageTypescript()))
}
func tsxSpec() tsSpec { return jsLikeSpec("tsx", sitter.NewLanguage(tsts.LanguageTSX())) }

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

	isBooleanBinary := func(kind string, n *sitter.Node, src []byte) bool {
		if kind != "binary_expression" {
			return false
		}
		op := n.ChildByFieldName("operator")
		if op == nil {
			return false
		}
		start, end := op.ByteRange()
		_, ok := booleanOps[string(src[start:end])] // map lookup with string([]byte) does not allocate
		return ok
	}

	return tsSpec{
		language: language,
		grammar:  grammar,
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
					signature = "(" + p.Utf8Text(src) + ")"
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
		importDelta: func(kind string, n *sitter.Node, src []byte) int {
			switch kind {
			case "import_statement":
				return 1
			case "call_expression":
				fn := n.ChildByFieldName("function")
				if fn == nil {
					return 0
				}
				if fn.Kind() == "import" {
					return 1
				}
				if fn.Kind() == "identifier" && fn.Utf8Text(src) == "require" {
					return 1
				}
			}
			return 0
		},
		importSpecs: func(kind string, n *sitter.Node, src []byte) []string {
			switch kind {
			case "import_statement":
				if source := n.ChildByFieldName("source"); source != nil {
					return []string{stripQuotes(source.Utf8Text(src))}
				}
			case "call_expression":
				fn := n.ChildByFieldName("function")
				if fn == nil {
					return nil
				}
				if fn.Kind() == "import" || (fn.Kind() == "identifier" && fn.Utf8Text(src) == "require") {
					if args := n.ChildByFieldName("arguments"); args != nil {
						for i := 0; i < int(args.NamedChildCount()); i++ {
							if arg := args.NamedChild(uint(i)); arg.Kind() == "string" {
								return []string{stripQuotes(arg.Utf8Text(src))}
							}
						}
					}
				}
			}
			return nil
		},
		varBindings: func(kind string, n *sitter.Node, src []byte) int {
			if kind == "variable_declarator" {
				return countJSBindingNames(n.ChildByFieldName("name"))
			}
			return 0
		},
		decision: func(kind string, n *sitter.Node, src []byte) int {
			if _, ok := decisions[kind]; ok {
				return 1
			}
			if isBooleanBinary(kind, n, src) {
				return 1
			}
			return 0
		},
		cognitive: func(kind string, n *sitter.Node, src []byte) cogKind {
			if _, ok := nesting[kind]; ok {
				return cogNesting
			}
			if kind == "else_clause" || isBooleanBinary(kind, n, src) {
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
	switch parent.Kind() {
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
	switch n.Kind() {
	case "identifier", "shorthand_property_identifier_pattern", "shorthand_property_identifier":
		return 1
	case "member_expression", "subscript_expression", "comment":
		return 0
	}
	count := 0
	for i := 0; i < int(n.NamedChildCount()); i++ {
		count += countJSBindingNames(n.NamedChild(uint(i)))
	}
	return count
}
