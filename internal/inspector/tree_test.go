package inspector

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestBuildTreeSupportedOnlyPrunesUnsupported(t *testing.T) {
	tmpDir := t.TempDir()

	mustWriteFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(tmpDir, "notes.txt"), "plain text\n")
	mustWriteFile(t, filepath.Join(tmpDir, "pkg", "worker.py"), "def run():\n    return 1\n")
	mustWriteFile(t, filepath.Join(tmpDir, "docs", "README.md"), "unsupported markdown\n")

	cfg := Config{
		ExcludedDirs:  BuildExcludeSet(true, nil),
		SupportedOnly: true,
	}

	tree, err := BuildTree(tmpDir, cfg)
	if err != nil {
		t.Fatalf("BuildTree returned error: %v", err)
	}

	files := collectFileNames(tree)
	sort.Strings(files)

	expected := []string{"main.go", "worker.py"}
	if len(files) != len(expected) {
		t.Fatalf("expected %d files, got %d (%v)", len(expected), len(files), files)
	}
	for idx, name := range expected {
		if files[idx] != name {
			t.Fatalf("expected file %q at position %d, got %q", name, idx, files[idx])
		}
	}
}

func TestBuildTreeDeterministicChildOrdering(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "zdir"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "adir"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	mustWriteFile(t, filepath.Join(tmpDir, "b.py"), "x = 1\n")
	mustWriteFile(t, filepath.Join(tmpDir, "A.go"), "package main\n")
	mustWriteFile(t, filepath.Join(tmpDir, "c.txt"), "note\n")

	cfg := Config{
		ExcludedDirs:  BuildExcludeSet(false, nil),
		SupportedOnly: false,
	}

	tree, err := BuildTree(tmpDir, cfg)
	if err != nil {
		t.Fatalf("BuildTree returned error: %v", err)
	}

	if len(tree.Children) != 5 {
		t.Fatalf("expected 5 direct children, got %d", len(tree.Children))
	}

	got := make([]string, 0, len(tree.Children))
	for _, child := range tree.Children {
		got = append(got, child.Name)
	}

	expected := []string{"adir", "zdir", "A.go", "b.py", "c.txt"}
	for idx := range expected {
		if got[idx] != expected[idx] {
			t.Fatalf("unexpected child order at index %d: got %q, expected %q", idx, got[idx], expected[idx])
		}
	}
}

func collectFileNames(node *TreeNode) []string {
	if node == nil {
		return nil
	}
	if !node.IsDir {
		return []string{node.Name}
	}
	result := make([]string, 0)
	for _, child := range node.Children {
		result = append(result, collectFileNames(child)...)
	}
	return result
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed for %q: %v", path, err)
	}
}
