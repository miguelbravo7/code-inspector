package inspector

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

func depReportFor(t *testing.T, files map[string]string) DependencyReport {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeDepFile(t, dir, name, content)
	}
	tree, err := BuildTree(dir, Config{ExcludedDirs: map[string]struct{}{}})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	return BuildDependencyGraph(tree, dir, 100)
}

func fanInOf(r DependencyReport, node string) int {
	for _, s := range r.MostDependedOn {
		if s.Node == node {
			return s.FanIn
		}
	}
	return 0
}

func fanOutOf(r DependencyReport, node string) int {
	for _, s := range r.MostDependencies {
		if s.Node == node {
			return s.FanOut
		}
	}
	return 0
}

func TestGenericDepsRust(t *testing.T) {
	r := depReportFor(t, map[string]string{
		"util.rs": "pub fn helper() -> i32 { 1 }\n",
		"lib.rs":  "use crate::util;\nuse std::collections::HashMap;\npub fn run() -> i32 { util::helper() }\n",
	})
	if got := fanInOf(r, "util.rs"); got != 1 {
		t.Fatalf("expected util.rs fan-in 1 (lib.rs -> util.rs), got %d (report %+v)", got, r.MostDependedOn)
	}
	// std::collections must be external, not a fabricated edge.
	if r.Edges != 1 {
		t.Fatalf("expected exactly 1 internal edge, got %d", r.Edges)
	}
}

func TestGenericDepsJavaPackage(t *testing.T) {
	r := depReportFor(t, map[string]string{
		"com/app/util/Helper.java": "package com.app.util;\npublic class Helper { public int v(){ return 1; } }\n",
		"com/app/Main.java":        "package com.app;\nimport com.app.util.Helper;\nimport java.util.List;\npublic class Main { }\n",
	})
	if got := fanInOf(r, "com/app/util/Helper.java"); got != 1 {
		t.Fatalf("expected Helper.java fan-in 1, got %d (report %+v)", got, r.MostDependedOn)
	}
	if r.Edges != 1 {
		t.Fatalf("expected 1 edge (java.util.List external), got %d", r.Edges)
	}
}

func TestGenericDepsCInclude(t *testing.T) {
	r := depReportFor(t, map[string]string{
		"util.h": "int helper(void);\n",
		"main.c": "#include \"util.h\"\n#include <stdio.h>\nint main(void){ return helper(); }\n",
	})
	if got := fanInOf(r, "util.h"); got != 1 {
		t.Fatalf("expected util.h fan-in 1, got %d (report %+v)", got, r.MostDependedOn)
	}
	// <stdio.h> is a system include: must stay external.
	if r.Edges != 1 {
		t.Fatalf("expected 1 edge (<stdio.h> external), got %d", r.Edges)
	}
}

func TestGenericDepsRubyRequireRelative(t *testing.T) {
	r := depReportFor(t, map[string]string{
		"lib/foo.rb": "def foo; 1; end\n",
		"app.rb":     "require_relative 'lib/foo'\nrequire 'json'\nputs foo\n",
	})
	if got := fanInOf(r, "lib/foo.rb"); got != 1 {
		t.Fatalf("expected lib/foo.rb fan-in 1, got %d (report %+v)", got, r.MostDependedOn)
	}
	if r.Edges != 1 {
		t.Fatalf("expected 1 edge (require 'json' external), got %d", r.Edges)
	}
}

func TestGenericDepsAmbiguousSuffixSkipped(t *testing.T) {
	// Two Util classes with the same basename in different packages; an import that
	// only matches by the ambiguous tail must NOT create an edge.
	r := depReportFor(t, map[string]string{
		"a/Util.java": "package a;\npublic class Util {}\n",
		"b/Util.java": "package b;\npublic class Util {}\n",
		"c/Main.java": "package c;\nimport x.y.Util;\npublic class Main {}\n",
	})
	if r.Edges != 0 {
		t.Fatalf("expected 0 edges for ambiguous/unmatched import, got %d (report %+v)", r.Edges, r.MostDependedOn)
	}
}

func TestGenericDepsNoCrossLanguageEdges(t *testing.T) {
	r := depReportFor(t, map[string]string{
		"foo.py": "def foo():\n    return 1\n",
		"foo.rs": "pub fn foo() -> i32 { 1 }\n",
		"bar.rs": "use crate::foo;\npub fn bar() -> i32 { foo::foo() }\n",
	})
	if got := fanInOf(r, "foo.rs"); got != 1 {
		t.Fatalf("expected foo.rs fan-in 1 (from bar.rs), got %d", got)
	}
	if got := fanInOf(r, "foo.py"); got != 0 {
		t.Fatalf("expected foo.py fan-in 0 (no cross-language edge), got %d", got)
	}
}

func TestAutoHintsInvariantsRust(t *testing.T) {
	vocab := deriveGrammarVocab(sitter.NewLanguage(tsrust.Language()))
	if len(vocab.kinds) == 0 {
		t.Fatalf("expected non-empty rust vocabulary")
	}
	if _, ok := vocab.fields["name"]; !ok {
		t.Fatalf("expected rust to declare a 'name' field")
	}
	d := classifyVocab(vocab)

	nesting := map[string]bool{}
	for _, k := range d.nesting {
		nesting[k] = true
	}
	for _, k := range d.flat {
		if nesting[k] {
			t.Fatalf("kind %q classified as both flat and nesting", k)
		}
	}
	for _, k := range d.functions {
		if strings.Contains(k, "type") || strings.Contains(k, "signature") || strings.Contains(k, "declarator") || strings.Contains(k, "modifier") {
			t.Fatalf("function derivation should exclude %q", k)
		}
	}
	contains := func(list []string, want string) bool {
		for _, k := range list {
			if k == want {
				return true
			}
		}
		return false
	}
	if !contains(d.functions, "function_item") {
		t.Fatalf("expected derived functions to include function_item, got %v", d.functions)
	}
	if !contains(d.decisions, "match_arm") || !contains(d.decisions, "if_expression") {
		t.Fatalf("expected derived decisions to include match_arm and if_expression, got %v", d.decisions)
	}
}

// --- regression tests for adversarial-review findings ---------------------

func TestJavaSwitchNoCyclomaticDoubleCount(t *testing.T) {
	m := mustAnalyze(t, "java", "class C{int f(int x){switch(x){case 1:return 1;case 2:return 2;default:return 0;}}}")
	fn := findFunctionByName(m.Functions, "f")
	if fn == nil {
		t.Fatalf("expected method f")
	}
	// Was 7 (switch_block_statement_group + switch_label both counted). The arm
	// label is counted once: base 1 + (case 1, case 2, default) labels = 4.
	if fn.Cyclomatic != 4 {
		t.Fatalf("expected cyclomatic 4 (no double-count), got %d", fn.Cyclomatic)
	}
}

func TestCSharpNullConditionalNotBranch(t *testing.T) {
	m := mustAnalyze(t, "csharp", "class C{int f(A a){return a?.b?.c ?? 0;}}")
	fn := findFunctionByName(m.Functions, "f")
	if fn == nil {
		t.Fatalf("expected method f")
	}
	// ?. is null-propagation, not a branch. Only ?? (boolean) adds: base 1 + 1 = 2.
	if fn.Cyclomatic != 2 {
		t.Fatalf("expected cyclomatic 2 (?. not a branch), got %d", fn.Cyclomatic)
	}
	if fn.MaxNesting != 0 {
		t.Fatalf("expected max nesting 0, got %d", fn.MaxNesting)
	}
}

func TestPhpMatchNotOverNested(t *testing.T) {
	m := mustAnalyze(t, "php", "<?php function f($x){ return match($x){1=>\"a\",2=>\"b\",default=>\"c\"}; }")
	fn := findFunctionByName(m.Functions, "f")
	if fn == nil {
		t.Fatalf("expected function f")
	}
	// Arms must not each open a nesting level; a flat match nests once.
	if fn.MaxNesting > 1 {
		t.Fatalf("expected max nesting <= 1 for a flat match, got %d", fn.MaxNesting)
	}
	if fn.Cognitive > 2 {
		t.Fatalf("expected low cognitive for a flat match, got %d", fn.Cognitive)
	}
}

func TestScalaImportRenameNotFunction(t *testing.T) {
	m := mustAnalyze(t, "scala", "import a.b.{C => D}\nobject M {}\n")
	if len(m.Functions) != 0 {
		t.Fatalf("import rename must not be counted as a function, got %+v", m.Functions)
	}
}

func TestPackageDeclarationNoFalseEdge(t *testing.T) {
	// X.java imports nothing; its `package a.b;` must not resolve to a/b.java.
	r := depReportFor(t, map[string]string{
		"a/b/X.java": "package a.b;\npublic class X {}\n",
		"a/b.java":   "package a;\npublic class b {}\n",
	})
	if r.Edges != 0 {
		t.Fatalf("package declaration must not create an edge, got %d (report %+v)", r.Edges, r.MostDependedOn)
	}
}

func TestPathLikeNoCrossLanguageEdge(t *testing.T) {
	// A C include that happens to name a file of another language family must not
	// create a cross-language edge.
	r := depReportFor(t, map[string]string{
		"data.json": "{\"a\":1}\n",
		"main.c":    "#include \"data.json\"\nint main(void){ return 0; }\n",
	})
	if r.Edges != 0 {
		t.Fatalf("cross-language path-like include must not create an edge, got %d (report %+v)", r.Edges, r.MostDependedOn)
	}
}

func TestGenericNonBooleanBinaryAddsNoComplexity(t *testing.T) {
	plain := mustAnalyze(t, "rust", "fn f(a: i32, b: i32) -> i32 { let c = a + b; c }\n")
	pf := findFunctionByName(plain.Functions, "f")
	if pf == nil || pf.Cyclomatic != 1 {
		t.Fatalf("expected non-boolean binary to keep cyclomatic 1, got %+v", pf)
	}
	boolean := mustAnalyze(t, "rust", "fn f(a: bool, b: bool) -> bool { a && b }\n")
	bf := findFunctionByName(boolean.Functions, "f")
	if bf == nil || bf.Cyclomatic != 2 {
		t.Fatalf("expected && to add 1 (cyclomatic 2), got %+v", bf)
	}
}
