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
	root, ok := decoded["root"].(map[string]any)
	if !ok {
		t.Fatalf("expected JSON output to contain root object, got: %v", decoded)
	}
	if root["Name"] == nil {
		t.Fatalf("expected root to contain Name field, got: %v", root)
	}
	if decoded["summary"] == nil {
		t.Fatalf("expected JSON output to contain summary, got: %v", decoded)
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

func TestRunRepeatableExcludePatterns(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "keep.go"), "package main\n")
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "worker_test.go"), "package main\n")
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "sqlc", "generated.go"), "package sqlc\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-exclude=*_test.go", "-exclude=sqlc", tmpDir}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", exitCode, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "keep.go") {
		t.Fatalf("expected output to include keep.go, got: %s", out)
	}
	if strings.Contains(out, "worker_test.go") {
		t.Fatalf("expected output to exclude *_test.go files, got: %s", out)
	}
	if strings.Contains(out, "sqlc/") {
		t.Fatalf("expected output to exclude sqlc directory, got: %s", out)
	}
}

func TestRunDefaultWorkersMatchesExplicitWorkersOne(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteCmdTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")

	var defaultStdout bytes.Buffer
	var defaultStderr bytes.Buffer
	defaultExitCode := run([]string{tmpDir}, &defaultStdout, &defaultStderr)
	if defaultExitCode != 0 {
		t.Fatalf("expected default run exit code 0, got %d (stderr: %s)", defaultExitCode, defaultStderr.String())
	}

	var explicitStdout bytes.Buffer
	var explicitStderr bytes.Buffer
	explicitExitCode := run([]string{"-workers=1", tmpDir}, &explicitStdout, &explicitStderr)
	if explicitExitCode != 0 {
		t.Fatalf("expected explicit workers=1 exit code 0, got %d (stderr: %s)", explicitExitCode, explicitStderr.String())
	}

	if defaultStdout.String() != explicitStdout.String() {
		t.Fatalf("expected default output to match explicit workers=1 output\ndefault:\n%s\nexplicit:\n%s", defaultStdout.String(), explicitStdout.String())
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
