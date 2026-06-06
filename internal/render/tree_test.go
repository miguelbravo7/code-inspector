package render

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"code-inspector/internal/inspector"
)

func TestPrintTreeRendersAsciiTreeWithFunctions(t *testing.T) {
	root := &inspector.TreeNode{
		Name:  "root",
		Path:  "root",
		IsDir: true,
		Children: []*inspector.TreeNode{
			{
				Name:  "app.ts",
				Path:  "root/app.ts",
				IsDir: false,
				Metrics: &inspector.FileMetrics{
					LineCount:     12,
					ImportCount:   2,
					VariableCount: 3,
					Functions: []inspector.FunctionInfo{
						{Name: "boot", Signature: "()", Line: 4, LineCount: 3},
					},
				},
			},
		},
	}

	var output bytes.Buffer
	if err := PrintTree(root, &output); err != nil {
		t.Fatalf("PrintTree returned error: %v", err)
	}

	expected := "root/\n└── app.ts [lines:12 imports:2 vars:3 funcs:1]\n    └── fn: boot | () | line 4 | lines 3\n"
	if output.String() != expected {
		t.Fatalf("unexpected tree output\nexpected:\n%s\nactual:\n%s", expected, output.String())
	}
}

func TestPrintTreeAlignsFunctionMetadataColumns(t *testing.T) {
	root := &inspector.TreeNode{
		Name:  "root",
		Path:  "root",
		IsDir: true,
		Children: []*inspector.TreeNode{
			{
				Name:  "analyzer_python.go",
				Path:  "root/analyzer_python.go",
				IsDir: false,
				Metrics: &inspector.FileMetrics{
					LineCount:     100,
					ImportCount:   3,
					VariableCount: 12,
					Functions: []inspector.FunctionInfo{
						{Name: "analyzePythonSource", Signature: "(source []byte) (*FileMetrics, error)", Line: 19, LineCount: 50},
						{Name: "<anonymous>", Signature: "(fn FunctionInfo)", Line: 25, LineCount: 11},
					},
				},
			},
		},
	}

	var output bytes.Buffer
	if err := PrintTree(root, &output); err != nil {
		t.Fatalf("PrintTree returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines in output, got %d\noutput:\n%s", len(lines), output.String())
	}

	firstFnLine := lines[2]
	secondFnLine := lines[3]
	idx1 := strings.Index(firstFnLine, " | line ")
	idx2 := strings.Index(secondFnLine, " | line ")
	if idx1 < 0 || idx2 < 0 {
		t.Fatalf("could not locate line metadata separator in function lines\noutput:\n%s", output.String())
	}

	if idx1 != idx2 {
		t.Fatalf("expected aligned function metadata columns, got indexes %d and %d\noutput:\n%s", idx1, idx2, output.String())
	}
}

func TestSummarizeWarningPreservesRunes(t *testing.T) {
	message := strings.Repeat("á", 100)
	got := summarizeWarning(message)

	if !utf8.ValidString(got) {
		t.Fatalf("summarizeWarning returned invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
	if utf8.RuneCountInString(got) != 80 {
		t.Fatalf("expected 80 runes after truncation, got %d", utf8.RuneCountInString(got))
	}
}
