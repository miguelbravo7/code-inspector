package inspector

import (
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

func TestGenericAdapterRust(t *testing.T) {
	source := `// classify returns the sign bucket.
fn classify(n: i32) -> i32 {
    if n > 0 && n < 10 {
        return 1;
    } else if n < 0 {
        return -1;
    }
    for i in 0..n {
        while i > 2 {
            return i;
        }
    }
    0
}
`
	metrics := mustAnalyze(t, "rust", source)
	fn := findFunctionByName(metrics.Functions, "classify")
	if fn == nil {
		t.Fatalf("expected to find rust function classify, got %+v", metrics.Functions)
	}
	if fn.Params != 1 {
		t.Fatalf("expected 1 param, got %d", fn.Params)
	}
	if fn.Cyclomatic < 5 {
		t.Fatalf("expected cyclomatic >= 5 for branchy fn, got %d", fn.Cyclomatic)
	}
	if fn.MaxNesting < 2 {
		t.Fatalf("expected max nesting >= 2, got %d", fn.MaxNesting)
	}
	if metrics.CommentLines < 1 {
		t.Fatalf("expected the line comment counted, got %d", metrics.CommentLines)
	}
	if metrics.CodeLines < 5 {
		t.Fatalf("expected code lines counted, got %d", metrics.CodeLines)
	}
}

func TestGenericAdapterJava(t *testing.T) {
	source := `class Calc {
    int classify(int n) {
        if (n > 0 && n < 10) {
            return 1;
        } else if (n < 0) {
            return -1;
        }
        for (int i = 0; i < n; i++) {
            while (i > 2) { return i; }
        }
        return 0;
    }
}
`
	metrics := mustAnalyze(t, "java", source)
	fn := findFunctionByName(metrics.Functions, "classify")
	if fn == nil {
		t.Fatalf("expected to find java method classify, got %+v", metrics.Functions)
	}
	if fn.Params != 1 {
		t.Fatalf("expected 1 param, got %d", fn.Params)
	}
	if fn.Cyclomatic < 5 {
		t.Fatalf("expected cyclomatic >= 5, got %d", fn.Cyclomatic)
	}
}

func TestRegisterLanguageCustomExtension(t *testing.T) {
	RegisterLanguage(LanguageConfig{
		Name:       "myrust",
		Grammar:    sitter.NewLanguage(tsrust.Language()),
		Extensions: []string{".myrs"},
	})

	found := false
	for _, ext := range SupportedExtensions() {
		if ext == ".myrs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected .myrs in SupportedExtensions, got %v", SupportedExtensions())
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.myrs")
	if err := os.WriteFile(path, []byte("fn f(a: i32) -> i32 { if a > 0 { return a; } -a }\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	metrics, supported, err := AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if !supported {
		t.Fatalf("expected .myrs to be supported after RegisterLanguage")
	}
	if metrics.Language != "myrust" {
		t.Fatalf("expected language myrust, got %q", metrics.Language)
	}
	if findFunctionByName(metrics.Functions, "f") == nil {
		t.Fatalf("expected function f detected, got %+v", metrics.Functions)
	}
}
