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

func hintSlice(h *LanguageHints, pick func(*LanguageHints) []string) []string {
	if h == nil {
		return nil
	}
	return pick(h)
}

// genericSpec builds a heuristic tsSpec for an arbitrary grammar. The curated
// kind sets form a high-precision base; grammar introspection (deriveGrammarVocab)
// augments them with the grammar's own vocabulary so unseen grammars adapt
// automatically; user Hints add further kinds and override the name/params fields.
func genericSpec(name string, grammar *sitter.Language, hints *LanguageHints) tsSpec {
	vocab := deriveGrammarVocab(grammar)
	derived := classifyVocab(vocab)

	functionSet := mergeSets(genericFunctionKinds, derived.functions, hintSlice(hints, func(h *LanguageHints) []string { return h.FunctionKinds }))
	decisionSet := mergeSets(genericDecisionKinds, derived.decisions, hintSlice(hints, func(h *LanguageHints) []string { return h.DecisionKinds }))
	nestingSet := mergeSets(genericNestingKinds, derived.nesting, hintSlice(hints, func(h *LanguageHints) []string { return h.NestingKinds }))
	flatSet := mergeSets(genericFlatKinds, derived.flat, nil)
	importSet := mergeSets(genericImportKinds, derived.imports, hintSlice(hints, func(h *LanguageHints) []string { return h.ImportKinds }))

	// Reconciliation: a kind must never be both flat and nesting (double cognitive
	// penalty). Nesting wins (it is the block-opening construct).
	for k := range flatSet {
		if _, ok := nestingSet[k]; ok {
			delete(flatSet, k)
		}
	}

	// Resolve the name/params fields against the grammar's real field vocabulary,
	// honoring explicit hints first.
	nameField := resolveField(vocab, hintField(hints, func(h *LanguageHints) string { return h.NameField }), "name")
	paramsField := resolveField(vocab, hintField(hints, func(h *LanguageHints) string { return h.ParamsField }), "parameters", "parameter_list", "formal_parameters", "parameter")

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
		importSpecs: func(n *sitter.Node, src []byte) []string {
			return genericImportSpecs(n, src, importSet)
		},
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

// hasBooleanOperator reports whether a node is a short-circuit boolean operator
// (&&, ||, ??, and, or). It prefers the grammar's "operator" field and falls
// back to scanning direct children when no such field exists.
func hasBooleanOperator(n *sitter.Node, src []byte) bool {
	if op := n.ChildByFieldName("operator"); op != nil {
		_, ok := genericBooleanOperators[op.Utf8Text(src)]
		return ok
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child.IsNamed() {
			continue // operators are anonymous tokens; skip named operands
		}
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

// --- auto-derived hints via grammar introspection -------------------------

type grammarVocab struct {
	kinds  map[string]struct{} // named, non-supertype node kinds
	fields map[string]struct{} // declared field names
}

// deriveGrammarVocab enumerates a grammar's named (non-supertype) node kinds and
// field names via the tree-sitter Language API. It runs once per RegisterLanguage
// (never on the per-node hot path) and never panics: a nil/empty grammar yields
// empty sets, so genericSpec degrades to curated-only behavior.
func deriveGrammarVocab(grammar *sitter.Language) grammarVocab {
	v := grammarVocab{kinds: map[string]struct{}{}, fields: map[string]struct{}{}}
	if grammar == nil {
		return v
	}
	kindCount := grammar.NodeKindCount()
	for i := uint32(0); i < kindCount && i <= 0xFFFF; i++ {
		id := uint16(i)
		if !grammar.NodeKindIsNamed(id) || grammar.NodeKindIsSupertype(id) {
			continue
		}
		kind := grammar.NodeKindForId(id)
		if kind == "" || kind == "ERROR" {
			continue
		}
		v.kinds[kind] = struct{}{}
	}
	// Field ids are 1-based; 0 means "no field".
	fieldCount := grammar.FieldCount()
	for i := uint32(1); i <= fieldCount; i++ {
		if name := grammar.FieldNameForId(uint16(i)); name != "" {
			v.fields[name] = struct{}{}
		}
	}
	return v
}

type derivedKinds struct {
	functions, decisions, nesting, flat, imports []string
}

// classifyVocab buckets a grammar's real node kinds using token-aware,
// suffix-gated, exclusion-filtered rules. It only ever ADDS to the curated base,
// so misclassification can at worst affect best-effort metrics, never crash.
func classifyVocab(v grammarVocab) derivedKinds {
	var d derivedKinds
	for kind := range v.kinds {
		if deriveIsFunction(kind) {
			d.functions = append(d.functions, kind)
		}
		if deriveIsDecision(kind) {
			d.decisions = append(d.decisions, kind)
		}
		if deriveIsNesting(kind) {
			d.nesting = append(d.nesting, kind)
		}
		if deriveIsFlat(kind) {
			d.flat = append(d.flat, kind)
		}
		if deriveIsImport(kind) {
			d.imports = append(d.imports, kind)
		}
	}
	return d
}

func tokenizeKind(kind string) []string { return strings.Split(kind, "_") }

func hasKindToken(kind string, tokens ...string) bool {
	for _, t := range tokenizeKind(kind) {
		for _, want := range tokens {
			if t == want {
				return true
			}
		}
	}
	return false
}

func hasKindSuffix(kind string, suffixes ...string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(kind, s) {
			return true
		}
	}
	return false
}

func deriveIsFunction(kind string) bool {
	// Reject signatures, types, declarators, modifiers, params, calls, and any
	// name/identifier kinds (e.g. Scala's arrow_renamed_identifier import selector).
	if hasKindToken(kind, "type", "signature", "declarator", "modifier", "modifiers", "specifier", "pointer", "generic", "parameter", "parameters", "call", "invocation", "identifier", "name") {
		return false
	}
	switch kind {
	case "method", "singleton_method", "anonymous_function", "local_function_statement":
		return true
	}
	// Suffix-gated so arrow/lambda/closure tokens only match real function nodes
	// (arrow_function, lambda_expression, closure_expression), not name kinds.
	if hasKindToken(kind, "lambda", "closure", "arrow") && hasKindSuffix(kind, "_function", "_expression", "_definition", "_declaration", "_item", "_literal") {
		return true
	}
	if hasKindToken(kind, "function", "method", "constructor") && hasKindSuffix(kind, "_definition", "_declaration", "_item") {
		return true
	}
	return false
}

func deriveIsDecision(kind string) bool {
	switch kind {
	// Containers and pure-else add no cyclomatic point (the arms / the if do).
	case "switch_statement", "switch_expression", "match_expression", "match_block", "case_block", "try_statement", "else_clause", "else":
		return false
	// Arms and branch points. switch_block_statement_group is intentionally NOT
	// here: in Java it wraps a switch_label, so counting both double-counts.
	case "match_arm", "case_clause", "when", "case_item", "case_statement",
		"switch_label", "switch_section",
		"catch_clause", "except_clause", "guard",
		"elsif", "elif", "elif_clause", "else_if_clause",
		"loop_expression", "do_statement":
		return true
	}
	if kind == "if" || (hasKindToken(kind, "if") && hasKindSuffix(kind, "_statement", "_expression")) {
		return true
	}
	if hasKindToken(kind, "for", "while") && hasKindSuffix(kind, "_statement", "_expression", "_clause") {
		return true
	}
	// "conditional" excludes "access" so C# conditional_access_expression (?.) is
	// not treated as a branch.
	if hasKindToken(kind, "ternary") || (hasKindToken(kind, "conditional") && !hasKindToken(kind, "access") && hasKindSuffix(kind, "_expression")) {
		return true
	}
	return false
}

func deriveIsNesting(kind string) bool {
	switch kind {
	// Arms do not open a new nesting level; the container already did. Includes
	// PHP's match arms (match_conditional_expression / match_default_expression).
	case "match_arm", "case_clause", "when", "case_item", "case_statement",
		"switch_label", "switch_section", "switch_block_statement_group",
		"match_conditional_expression", "match_default_expression":
		return false
	// else/elif are flat continuations.
	case "else_clause", "else", "elsif", "elif", "elif_clause", "else_if_clause":
		return false
	}
	switch kind {
	// Containers nest once; their *_block bodies are not counted separately to
	// avoid double-nesting a flat switch/match.
	case "switch_statement", "switch_expression", "match_expression",
		"catch_clause", "except_clause", "loop_expression", "do_statement":
		return true
	}
	if kind == "if" || (hasKindToken(kind, "if") && hasKindSuffix(kind, "_statement", "_expression")) {
		return true
	}
	if hasKindToken(kind, "for", "while") && hasKindSuffix(kind, "_statement", "_expression") {
		return true
	}
	if hasKindToken(kind, "ternary") || (hasKindToken(kind, "conditional") && !hasKindToken(kind, "access") && hasKindSuffix(kind, "_expression")) {
		return true
	}
	return false
}

func deriveIsFlat(kind string) bool {
	switch kind {
	case "else_clause", "else", "elsif", "elif", "elif_clause", "else_if_clause":
		return true
	}
	return false
}

func deriveIsImport(kind string) bool {
	// The dotted/scoped path itself is a sub-node of an import, not the import.
	if hasKindToken(kind, "identifier", "name") && !hasKindSuffix(kind, "_declaration", "_directive", "_statement", "_clause", "_specifier") {
		return false
	}
	switch kind {
	case "preproc_include", "extern_crate_declaration", "package_clause", "package_declaration":
		return true
	}
	if hasKindToken(kind, "import", "use", "using", "include", "require") && hasKindSuffix(kind, "_statement", "_declaration", "_directive", "_clause", "_specifier") {
		return true
	}
	return false
}

func mergeSets(groups ...[]string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, group := range groups {
		for _, k := range group {
			if k != "" {
				set[k] = struct{}{}
			}
		}
	}
	return set
}

func hintField(h *LanguageHints, pick func(*LanguageHints) string) string {
	if h == nil {
		return ""
	}
	return pick(h)
}

// resolveField returns the first candidate field the grammar actually declares,
// honoring an explicit override. Falls back to the first candidate when none are
// declared (downstream helpers have further fallbacks).
func resolveField(v grammarVocab, override string, candidates ...string) string {
	if override != "" {
		return override
	}
	for _, c := range candidates {
		if _, ok := v.fields[c]; ok {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return "name"
}

// --- generic import-specifier extraction (for dependency-graph edges) -----

var requireCallNames = map[string]struct{}{
	"require": {}, "require_relative": {}, "require_once": {},
	"include": {}, "include_once": {}, "import": {}, "source": {},
	"load": {}, "autoload": {},
}

var nameLikeKinds = map[string]struct{}{
	"scoped_identifier": {}, "qualified_name": {}, "dotted_name": {},
	"namespace_name": {}, "scoped_type_identifier": {}, "namespace_use_clause": {},
	"relative_import": {}, "scoped_use_list": {},
}

// genericImportSpecs extracts the raw module specifier(s) from an import node or a
// require-style call, for the dependency graph. Returns nil when nothing concrete
// is extractable (system <...> includes, dynamic arguments, etc.).
func genericImportSpecs(n *sitter.Node, src []byte, importSet map[string]struct{}) []string {
	if _, ok := importSet[n.Kind()]; ok {
		switch n.Kind() {
		case "package_clause", "package_declaration":
			// A package statement is the file's own identity, not a dependency.
			return nil
		}
		if s := extractImportSpecifier(n, src); s != "" {
			return []string{s}
		}
		return nil
	}
	if isRequireCall(n, src) {
		if s := requireCallArg(n, src); s != "" {
			return []string{s}
		}
	}
	return nil
}

func extractImportSpecifier(n *sitter.Node, src []byte) string {
	if s := findStringLike(n, 4); s != nil {
		if s.Kind() == "system_lib_string" {
			return "" // C/C++ <system> include: external, never an edge candidate
		}
		txt := stripQuotes(s.Utf8Text(src))
		if strings.HasPrefix(txt, "<") {
			return ""
		}
		return txt
	}
	if nm := findNameLike(n, src, 4); nm != "" {
		return nm
	}
	return joinIdentifierSequence(n, src)
}

func findStringLike(n *sitter.Node, depth int) *sitter.Node {
	if depth < 0 {
		return nil
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if strings.Contains(child.Kind(), "string") {
			return child
		}
		if found := findStringLike(child, depth-1); found != nil {
			return found
		}
	}
	return nil
}

func findNameLike(n *sitter.Node, src []byte, depth int) string {
	if depth < 0 {
		return ""
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		kind := child.Kind()
		if _, ok := nameLikeKinds[kind]; ok {
			return strings.TrimSpace(child.Utf8Text(src))
		}
		if (strings.HasSuffix(kind, "identifier") || strings.HasSuffix(kind, "name")) && containsImportSeparator(child.Utf8Text(src)) {
			return strings.TrimSpace(child.Utf8Text(src))
		}
		if found := findNameLike(child, src, depth-1); found != "" {
			return found
		}
	}
	return ""
}

func containsImportSeparator(s string) bool {
	return strings.Contains(s, "::") || strings.ContainsAny(s, ".\\/")
}

// joinIdentifierSequence handles grammars (e.g. Scala) where an import path is a
// run of sibling identifier children rather than one scoped node.
func joinIdentifierSequence(n *sitter.Node, src []byte) string {
	var parts []string
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if strings.HasSuffix(child.Kind(), "identifier") {
			parts = append(parts, child.Utf8Text(src))
		}
	}
	if len(parts) >= 2 {
		return strings.Join(parts, ".")
	}
	return ""
}

func isRequireCall(n *sitter.Node, src []byte) bool {
	kind := n.Kind()
	if kind == "command" { // bash: source / .
		if name := n.ChildByFieldName("name"); name != nil {
			return isRequireName(name.Utf8Text(src))
		}
		if n.NamedChildCount() > 0 {
			return isRequireName(n.NamedChild(0).Utf8Text(src))
		}
		return false
	}
	if kind != "call" && kind != "call_expression" && kind != "function_call_expression" {
		return false
	}
	if fn := n.ChildByFieldName("function"); fn != nil {
		return isRequireName(fn.Utf8Text(src))
	}
	if m := n.ChildByFieldName("method"); m != nil {
		return isRequireName(m.Utf8Text(src))
	}
	if n.NamedChildCount() > 0 {
		return isRequireName(n.NamedChild(0).Utf8Text(src))
	}
	return false
}

func isRequireName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "." { // bash source shorthand
		return true
	}
	_, ok := requireCallNames[s]
	return ok
}

func requireCallArg(n *sitter.Node, src []byte) string {
	if s := findStringLike(n, 4); s != nil {
		if s.Kind() == "system_lib_string" {
			return ""
		}
		txt := stripQuotes(s.Utf8Text(src))
		if strings.HasPrefix(txt, "<") {
			return ""
		}
		return txt
	}
	return ""
}
