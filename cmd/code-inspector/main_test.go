package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTreeFormat(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{tmpDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "main.go") {
		t.Fatalf("expected tree output to include main.go, got: %s", stdout.String())
	}
}

func TestRunJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "app.py"), "x = 1\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-format=json", tmpDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", exitCode, stderr.String())
	}

	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput: %s", err, stdout.String())
	}
	if decoded["Name"] == nil {
		t.Fatalf("expected JSON output to contain Name field, got: %v", decoded)
	}
}

func TestRunWorkersOneSequentialMode(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-workers=1", tmpDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "main.go") {
		t.Fatalf("expected tree output to include main.go, got: %s", stdout.String())
	}
}

func TestRunUnsupportedFormatReturnsUsageError(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-format=xml", tmpDir}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got: %s", stderr.String())
	}
}

func TestRunNegativeWorkersReturnsUsageError(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-workers=-1", tmpDir}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "workers must be >= 0") {
		t.Fatalf("expected workers validation error, got: %s", stderr.String())
	}
}

func mustWriteCmdTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed for %q: %v", path, err)
	}
}
