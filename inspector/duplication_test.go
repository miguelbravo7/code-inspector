package inspector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDuplicationFindsRenamedClone(t *testing.T) {
	// Two functions with identical structure but renamed identifiers and changed
	// literals should be detected as a clone.
	block := func(fn, a, b, lit string) string {
		return "func " + fn + "(" + a + " int) int {\n" +
			"\t" + b + " := " + a + " + " + lit + "\n" +
			"\tif " + b + " > " + lit + " {\n" +
			"\t\t" + b + " = " + b + " * " + a + "\n" +
			"\t}\n" +
			"\tfor i := 0; i < " + b + "; i++ {\n" +
			"\t\t" + b + " = " + b + " - i\n" +
			"\t}\n" +
			"\treturn " + b + "\n" +
			"}\n"
	}
	source := "package sample\n\n" +
		block("first", "x", "y", "10") + "\n" +
		block("second", "p", "q", "42") + "\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	tree, err := BuildTree(dir, Config{ExcludedDirs: map[string]struct{}{}})
	if err != nil {
		t.Fatalf("BuildTree failed: %v", err)
	}

	report := DetectDuplication(tree, 20, 10)
	if report.TotalBlocks == 0 {
		t.Fatalf("expected at least one duplicate block, got none")
	}
	if len(report.Blocks) == 0 {
		t.Fatalf("expected reported blocks")
	}
	if report.Blocks[0].Tokens < 20 {
		t.Fatalf("expected a clone of at least 20 tokens, got %d", report.Blocks[0].Tokens)
	}
}

func TestDetectDuplicationIgnoresDistinctCode(t *testing.T) {
	source := `package sample

func alpha() int {
	return 1
}

func beta(name string) string {
	return "hello " + name
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	tree, err := BuildTree(dir, Config{ExcludedDirs: map[string]struct{}{}})
	if err != nil {
		t.Fatalf("BuildTree failed: %v", err)
	}

	report := DetectDuplication(tree, 50, 10)
	if report.TotalBlocks != 0 {
		t.Fatalf("expected no duplicate blocks, got %d", report.TotalBlocks)
	}
}
