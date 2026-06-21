package inspector

import "testing"

func findFunctionByName(functions []FunctionInfo, name string) *FunctionInfo {
	for idx := range functions {
		if functions[idx].Name == name {
			return &functions[idx]
		}
	}
	return nil
}

func mustAnalyze(t *testing.T, language string, source string) *FileMetrics {
	t.Helper()
	metrics, err := analyzeSource(language, []byte(source))
	if err != nil {
		t.Fatalf("analyzeSource(%q) returned error: %v", language, err)
	}
	if metrics == nil {
		t.Fatalf("analyzeSource(%q) returned nil metrics", language)
	}
	applyFunctionRollups(metrics)
	return metrics
}

func TestAnalyzeGoSourceMetrics(t *testing.T) {
	source := `package sample
import "fmt"

var global = 1

func main() {
	local := 2
	helper := func(x int) int { return x + local }
	fmt.Println(helper(global))
}

type service struct{}

func (s *service) run() {}
`

	metrics := mustAnalyze(t, "go", source)

	if metrics.ImportCount != 1 {
		t.Fatalf("expected 1 import, got %d", metrics.ImportCount)
	}
	if metrics.VariableCount != 3 {
		t.Fatalf("expected 3 variable definitions, got %d", metrics.VariableCount)
	}
	if len(metrics.Functions) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(metrics.Functions))
	}

	mainFn := findFunctionByName(metrics.Functions, "main")
	if mainFn == nil {
		t.Fatalf("expected to find main function")
	}
	if mainFn.LineCount != 5 {
		t.Fatalf("expected main function to have 5 lines, got %d", mainFn.LineCount)
	}

	runFn := findFunctionByName(metrics.Functions, "*service.run")
	if runFn == nil {
		t.Fatalf("expected to find receiver method *service.run")
	}
	if runFn.LineCount != 1 {
		t.Fatalf("expected receiver method to have 1 line, got %d", runFn.LineCount)
	}
}

func TestGoComplexityMetrics(t *testing.T) {
	source := `package sample

func cyc(a, b int) int {
	if a > 0 && b > 0 {
		for i := 0; i < a; i++ {
			if i == b {
				return i
			}
		}
	} else if a < 0 {
		return -1
	}
	switch b {
	case 1:
		return 1
	case 2:
		return 2
	}
	return 0
}
`
	metrics := mustAnalyze(t, "go", source)
	fn := findFunctionByName(metrics.Functions, "cyc")
	if fn == nil {
		t.Fatalf("expected to find cyc function")
	}
	if fn.Params != 2 {
		t.Fatalf("expected 2 params, got %d", fn.Params)
	}
	if fn.Cyclomatic != 8 {
		t.Fatalf("expected cyclomatic 8, got %d", fn.Cyclomatic)
	}
	if fn.MaxNesting != 3 {
		t.Fatalf("expected max nesting 3, got %d", fn.MaxNesting)
	}
	if metrics.MaxComplexity != 8 {
		t.Fatalf("expected file max complexity 8, got %d", metrics.MaxComplexity)
	}
}

func TestAnalyzePythonSourceMetrics(t *testing.T) {
	source := `import os
from sys import path

x = 1
a, b = 2, 3

class Service:
    def run(self):
        y = 2

def top():
    return y if False else x
`

	metrics := mustAnalyze(t, "python", source)

	if metrics.ImportCount != 2 {
		t.Fatalf("expected 2 imports, got %d", metrics.ImportCount)
	}
	if metrics.VariableCount != 4 {
		t.Fatalf("expected 4 variable definitions, got %d", metrics.VariableCount)
	}
	if len(metrics.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(metrics.Functions))
	}

	runFn := findFunctionByName(metrics.Functions, "run")
	if runFn == nil {
		t.Fatalf("expected to find run function")
	}
	if runFn.LineCount != 2 {
		t.Fatalf("expected run function to have 2 lines, got %d", runFn.LineCount)
	}
	if runFn.Params != 1 {
		t.Fatalf("expected run to have 1 param (self), got %d", runFn.Params)
	}

	topFn := findFunctionByName(metrics.Functions, "top")
	if topFn == nil {
		t.Fatalf("expected to find top function")
	}
	if topFn.Cyclomatic != 2 {
		t.Fatalf("expected top cyclomatic 2 (ternary), got %d", topFn.Cyclomatic)
	}
}

func TestAnalyzeTypeScriptSourceMetrics(t *testing.T) {
	source := `import fs from "fs";
const path = require("path");
let count = 0;
const add = (a, b) => a + b;

function boot() {
	return add(count, 1);
}

class Service {
	start() { return boot(); }
}
`

	metrics := mustAnalyze(t, "typescript", source)

	if metrics.ImportCount != 2 {
		t.Fatalf("expected 2 imports, got %d", metrics.ImportCount)
	}
	if metrics.VariableCount != 3 {
		t.Fatalf("expected 3 variable definitions, got %d", metrics.VariableCount)
	}
	if len(metrics.Functions) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(metrics.Functions))
	}

	addFn := findFunctionByName(metrics.Functions, "add")
	if addFn == nil {
		t.Fatalf("expected to find add function")
	}
	if addFn.LineCount != 1 {
		t.Fatalf("expected add function to have 1 line, got %d", addFn.LineCount)
	}
	if addFn.Params != 2 {
		t.Fatalf("expected add to have 2 params, got %d", addFn.Params)
	}

	bootFn := findFunctionByName(metrics.Functions, "boot")
	if bootFn == nil {
		t.Fatalf("expected to find boot function")
	}
	if bootFn.LineCount != 3 {
		t.Fatalf("expected boot function to have 3 lines, got %d", bootFn.LineCount)
	}

	startFn := findFunctionByName(metrics.Functions, "start")
	if startFn == nil {
		t.Fatalf("expected to find class method start")
	}
	if startFn.LineCount != 1 {
		t.Fatalf("expected class method start to have 1 line, got %d", startFn.LineCount)
	}
}

func TestTypeScriptIgnoresImportPatternsInStrings(t *testing.T) {
	source := "const text = \"import('fake')\";\n" +
		"const msg = 'require(\"ghost\")';\n" +
		"const tpl = `import(\"ghost\")`;\n" +
		"const real = require(\"path\");\n" +
		"\n" +
		"async function load() {\n" +
		"\treturn import(\"./module.js\");\n" +
		"}\n"

	metrics := mustAnalyze(t, "typescript", source)
	if metrics.ImportCount != 2 {
		t.Fatalf("expected 2 imports (require + dynamic import), got %d", metrics.ImportCount)
	}
}

func TestPythonComplexityNesting(t *testing.T) {
	source := `def handle(items):
    for item in items:
        if item > 0:
            if item % 2 == 0:
                print(item)
    return items
`
	metrics := mustAnalyze(t, "python", source)
	fn := findFunctionByName(metrics.Functions, "handle")
	if fn == nil {
		t.Fatalf("expected to find handle function")
	}
	// base 1 + for + if + if = 4
	if fn.Cyclomatic != 4 {
		t.Fatalf("expected cyclomatic 4, got %d", fn.Cyclomatic)
	}
	if fn.MaxNesting != 3 {
		t.Fatalf("expected max nesting 3, got %d", fn.MaxNesting)
	}
}

func TestLineClassificationCounts(t *testing.T) {
	source := `# leading comment
import os

def f():  # trailing comment
    """docstring"""
    return 1
`
	metrics := mustAnalyze(t, "python", source)
	if metrics.CommentLines != 1 {
		t.Fatalf("expected 1 pure comment line, got %d", metrics.CommentLines)
	}
	if metrics.BlankLines != 2 {
		t.Fatalf("expected 2 blank lines, got %d (one trailing newline)", metrics.BlankLines)
	}
	// import, def (code + trailing comment), docstring, return = 4 code lines
	if metrics.CodeLines != 4 {
		t.Fatalf("expected 4 code lines, got %d", metrics.CodeLines)
	}
}

func TestTodoMarkerCount(t *testing.T) {
	source := `// TODO: refactor this
// regular comment
function f() {
	// FIXME: broken, also HACK
	return 1;
}
`
	metrics := mustAnalyze(t, "javascript", source)
	if metrics.TodoCount != 3 {
		t.Fatalf("expected 3 todo markers, got %d", metrics.TodoCount)
	}
}

func TestHalsteadAndMaintainability(t *testing.T) {
	cases := []struct {
		name     string
		language string
		source   string
	}{
		{"go", "go", "package s\nfunc add(a, b int) int {\n\tc := a + b\n\treturn c * 2\n}\n"},
		{"python", "python", "def add(a, b):\n    c = a + b\n    return c * 2\n"},
		{"typescript", "typescript", "function add(a: number, b: number): number {\n\tconst c = a + b;\n\treturn c * 2;\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metrics := mustAnalyze(t, tc.language, tc.source)
			if metrics.Halstead.Volume <= 0 {
				t.Fatalf("expected positive Halstead volume, got %f", metrics.Halstead.Volume)
			}
			if metrics.Maintainability <= 0 || metrics.Maintainability > 100 {
				t.Fatalf("expected maintainability in (0,100], got %f", metrics.Maintainability)
			}
			fn := findFunctionByName(metrics.Functions, "add")
			if fn == nil {
				t.Fatalf("expected to find add function")
			}
			if fn.Maintainability <= 0 || fn.Maintainability > 100 {
				t.Fatalf("expected function maintainability in (0,100], got %f", fn.Maintainability)
			}
		})
	}
}

func TestMaintainabilityFallsWithComplexity(t *testing.T) {
	simple := mustAnalyze(t, "go", "package s\nfunc f(x int) int { return x }\n")
	complex := mustAnalyze(t, "go", `package s

func f(x int) int {
	total := 0
	for i := 0; i < x; i++ {
		if i%2 == 0 && i > 3 {
			total += i
		} else if i%3 == 0 {
			total -= i
		}
	}
	return total
}
`)
	if complex.Maintainability >= simple.Maintainability {
		t.Fatalf("expected complex file MI (%.1f) below simple file MI (%.1f)", complex.Maintainability, simple.Maintainability)
	}
}

func TestSortFunctionsOrdersByLineThenNameThenSignature(t *testing.T) {
	functions := []FunctionInfo{
		{Name: "zeta", Signature: "(z int)", Line: 10},
		{Name: "alpha", Signature: "(y int)", Line: 10},
		{Name: "beta", Signature: "()", Line: 2},
		{Name: "alpha", Signature: "(a int)", Line: 10},
	}

	sortFunctions(functions)

	expected := []FunctionInfo{
		{Name: "beta", Signature: "()", Line: 2},
		{Name: "alpha", Signature: "(a int)", Line: 10},
		{Name: "alpha", Signature: "(y int)", Line: 10},
		{Name: "zeta", Signature: "(z int)", Line: 10},
	}

	for idx := range expected {
		if functions[idx].Line != expected[idx].Line || functions[idx].Name != expected[idx].Name || functions[idx].Signature != expected[idx].Signature {
			t.Fatalf("unexpected function order at index %d: got %+v, expected %+v", idx, functions[idx], expected[idx])
		}
	}
}
