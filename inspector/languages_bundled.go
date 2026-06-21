package inspector

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tsbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tscsharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tsc "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tscpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tscss "github.com/tree-sitter/tree-sitter-css/bindings/go"
	tshtml "github.com/tree-sitter/tree-sitter-html/bindings/go"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tsjson "github.com/tree-sitter/tree-sitter-json/bindings/go"
	tsphp "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tsruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tsscala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
)

// Bundled grammars, analyzed with the generic heuristic adapter. Go, Python and
// the JS/TS family are registered separately (Go via go/ast; JS/TS with
// hand-tuned specs). Pinned versions are recorded in go.mod and verified to
// build/parse against go-tree-sitter v0.25. Register more at runtime with
// RegisterLanguage.
func init() {
	register := func(name string, grammar *sitter.Language, exts ...string) {
		RegisterLanguage(LanguageConfig{Name: name, Grammar: grammar, Extensions: exts})
	}

	register("rust", sitter.NewLanguage(tsrust.Language()), ".rs")
	register("java", sitter.NewLanguage(tsjava.Language()), ".java")
	register("c", sitter.NewLanguage(tsc.Language()), ".c", ".h")
	register("cpp", sitter.NewLanguage(tscpp.Language()), ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx")
	register("csharp", sitter.NewLanguage(tscsharp.Language()), ".cs")
	register("ruby", sitter.NewLanguage(tsruby.Language()), ".rb")
	register("bash", sitter.NewLanguage(tsbash.Language()), ".sh", ".bash")
	register("css", sitter.NewLanguage(tscss.Language()), ".css")
	register("html", sitter.NewLanguage(tshtml.Language()), ".html", ".htm")
	register("json", sitter.NewLanguage(tsjson.Language()), ".json")
	register("php", sitter.NewLanguage(tsphp.LanguagePHP()), ".php")
	register("scala", sitter.NewLanguage(tsscala.Language()), ".scala", ".sc")
}
