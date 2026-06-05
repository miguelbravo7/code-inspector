package render

import (
	"bytes"
	"testing"

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

	expected := "root/\n`-- app.ts [lines:12 imports:2 vars:3 funcs:1]\n    `-- fn: boot | () | line 4 | lines 3\n"
	if output.String() != expected {
		t.Fatalf("unexpected tree output\nexpected:\n%s\nactual:\n%s", expected, output.String())
	}
}
