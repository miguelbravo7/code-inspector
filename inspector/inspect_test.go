package inspector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectAggregatesReport(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.go":   "package main\n\nfunc main() {\n\tfor i := 0; i < 10; i++ {\n\t\tif i%2 == 0 {\n\t\t\tprintln(i)\n\t\t}\n\t}\n}\n",
		"util.py":   "def add(a, b):\n    return a + b\n",
		"app.ts":    "export function add(a: number, b: number): number {\n\treturn a + b;\n}\n",
		"README.md": "# not analyzed\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}

	report, err := Inspect(dir, Options{NoGit: true})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if report.Root == nil {
		t.Fatalf("expected a tree root")
	}
	if report.Summary.SupportedFiles != 3 {
		t.Fatalf("expected 3 analyzed files, got %d", report.Summary.SupportedFiles)
	}
	if report.Summary.TotalFunctions < 3 {
		t.Fatalf("expected at least 3 functions, got %d", report.Summary.TotalFunctions)
	}
	if report.Duplication == nil {
		t.Fatalf("expected a duplication report")
	}
	if report.Dependencies == nil {
		t.Fatalf("expected a dependency report")
	}
}

func TestInspectRespectsDisableFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nvar X = 1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := Inspect(dir, Options{NoGit: true, NoDup: true, NoDeps: true})
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if report.Duplication != nil {
		t.Fatalf("expected no duplication report when NoDup set")
	}
	if report.Dependencies != nil {
		t.Fatalf("expected no dependency report when NoDeps set")
	}
}
