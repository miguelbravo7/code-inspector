package inspector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

// BenchmarkAnalyzeGeneric measures the generic (introspection-derived) adapter on
// languages without a hand-tuned spec, for comparison with BenchmarkAnalyzeSources.
func BenchmarkAnalyzeGeneric(b *testing.B) {
	cases := []struct {
		name, language string
		source         []byte
	}{
		{"rust", "rust", []byte(buildRustBenchmarkSource(240))},
		{"java", "java", []byte(buildJavaBenchmarkSource(240))},
	}
	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := analyzeSource(tc.language, tc.source); err != nil {
					b.Fatalf("analyzeSource(%q): %v", tc.language, err)
				}
			}
		})
	}
}

// BenchmarkGenericSpecBuild measures the one-time per-language registration cost,
// which includes grammar introspection (deriveGrammarVocab) and classification.
func BenchmarkGenericSpecBuild(b *testing.B) {
	grammar := sitter.NewLanguage(tsrust.Language())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = genericSpec("rustbench", grammar, nil)
	}
}

func BenchmarkBuildDependencyGraph(b *testing.B) {
	dir, tree := buildRustModuleTree(b, 120)
	_ = dir
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildDependencyGraph(tree, dir, 10)
	}
}

func BenchmarkDetectDuplication(b *testing.B) {
	_, tree := buildRustModuleTree(b, 120)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DetectDuplication(tree, DefaultDuplicationMinTokens, 10)
	}
}

// BenchmarkInspect measures the full pipeline end-to-end (walk + parse + summary
// + duplication + dependency graph), excluding git churn.
func BenchmarkInspect(b *testing.B) {
	dir, _ := buildRustModuleTree(b, 120)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Inspect(dir, Options{NoGit: true}); err != nil {
			b.Fatalf("Inspect: %v", err)
		}
	}
}

func buildRustModuleTree(b *testing.B, n int) (string, *TreeNode) {
	b.Helper()
	dir := b.TempDir()
	for i := 0; i < n; i++ {
		var src strings.Builder
		if i > 0 {
			src.WriteString(fmt.Sprintf("use crate::mod%d;\n", i-1))
		}
		src.WriteString(buildRustBenchmarkSource(4))
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("mod%d.rs", i)), []byte(src.String()), 0o644); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
	tree, err := BuildTree(dir, Config{ExcludedDirs: map[string]struct{}{}})
	if err != nil {
		b.Fatalf("BuildTree: %v", err)
	}
	return dir, tree
}

func buildRustBenchmarkSource(functionCount int) string {
	var b strings.Builder
	b.WriteString("use std::collections::HashMap;\n\n")
	for i := 0; i < functionCount; i++ {
		b.WriteString(fmt.Sprintf("fn fn%d(x: i32) -> i32 {\n", i))
		b.WriteString("    if x > 0 && x < 100 {\n")
		b.WriteString("        for i in 0..x {\n")
		b.WriteString("            if i % 2 == 0 { return i; }\n")
		b.WriteString("        }\n")
		b.WriteString("    }\n")
		b.WriteString(fmt.Sprintf("    match x {\n        0 => 0,\n        _ => x + %d,\n    }\n", i))
		b.WriteString("}\n\n")
	}
	return b.String()
}

func buildJavaBenchmarkSource(methodCount int) string {
	var b strings.Builder
	b.WriteString("package bench;\n\nimport java.util.List;\n\npublic class Bench {\n")
	for i := 0; i < methodCount; i++ {
		b.WriteString(fmt.Sprintf("  int fn%d(int x) {\n", i))
		b.WriteString("    if (x > 0 && x < 100) {\n")
		b.WriteString("      for (int i = 0; i < x; i++) {\n")
		b.WriteString("        if (i % 2 == 0) { return i; }\n")
		b.WriteString("      }\n")
		b.WriteString("    }\n")
		b.WriteString(fmt.Sprintf("    switch (x) { case 0: return 0; default: return x + %d; }\n", i))
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}
