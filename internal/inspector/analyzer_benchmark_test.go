package inspector

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkAnalyzeHotspots(b *testing.B) {
	goSource := []byte(buildGoBenchmarkSource(240))
	pythonSource := []byte(buildPythonBenchmarkSource(320))
	typescriptSource := []byte(buildTypeScriptBenchmarkSource(280))

	b.Run("go", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(goSource)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics, err := analyzeGoSource(goSource)
			if err != nil {
				b.Fatalf("analyzeGoSource returned error: %v", err)
			}
			if metrics == nil {
				b.Fatalf("analyzeGoSource returned nil metrics")
			}
		}
	})

	b.Run("python", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(pythonSource)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics, err := analyzePythonSource(pythonSource)
			if err != nil {
				b.Fatalf("analyzePythonSource returned error: %v", err)
			}
			if metrics == nil {
				b.Fatalf("analyzePythonSource returned nil metrics")
			}
		}
	})

	b.Run("typescript", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(typescriptSource)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics, err := analyzeJavaScriptLikeSource(typescriptSource, "typescript")
			if err != nil {
				b.Fatalf("analyzeJavaScriptLikeSource returned error: %v", err)
			}
			if metrics == nil {
				b.Fatalf("analyzeJavaScriptLikeSource returned nil metrics")
			}
		}
	})
}

func BenchmarkEstimateJSFunctionLineCount(b *testing.B) {
	source := buildTypeScriptBenchmarkSource(400)
	lines := strings.Split(source, "\n")
	startLines := jsFunctionStartLines(lines)
	if len(startLines) == 0 {
		b.Fatalf("expected at least one function start line")
	}

	b.ReportAllocs()
	b.ResetTimer()

	idx := 0
	for i := 0; i < b.N; i++ {
		_ = estimateJSFunctionLineCount(lines, startLines[idx])
		idx++
		if idx >= len(startLines) {
			idx = 0
		}
	}
}

func jsFunctionStartLines(lines []string) []int {
	starts := make([]int, 0, len(lines)/4)
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "function ") || strings.HasPrefix(trimmed, "export function ") {
			starts = append(starts, idx+1)
		}
	}
	return starts
}

func buildGoBenchmarkSource(functionCount int) string {
	var builder strings.Builder
	builder.WriteString("package bench\n\nimport \"fmt\"\n\n")
	for i := 0; i < functionCount; i++ {
		builder.WriteString(fmt.Sprintf("func fn%d(x int) int {\n", i))
		builder.WriteString(fmt.Sprintf("\tlocal := x + %d\n", i))
		builder.WriteString("\thelper := func(v int) int { return v + local }\n")
		builder.WriteString("\tfmt.Println(helper(local))\n")
		builder.WriteString("\treturn helper(local)\n")
		builder.WriteString("}\n\n")
	}
	return builder.String()
}

func buildPythonBenchmarkSource(functionCount int) string {
	var builder strings.Builder
	builder.WriteString("import os\nfrom sys import path\n\n")
	for i := 0; i < functionCount; i++ {
		builder.WriteString(fmt.Sprintf("value_%d: int = %d\n", i, i))
		builder.WriteString(fmt.Sprintf("left_%d, right_%d = %d, %d\n", i, i, i+1, i+2))
		builder.WriteString(fmt.Sprintf("first_%d = second_%d = %d\n", i, i, i+3))
		builder.WriteString(fmt.Sprintf("def run_%d(x):\n", i))
		builder.WriteString(fmt.Sprintf("\ttmp = x + value_%d\n", i))
		builder.WriteString("\treturn tmp\n\n")
	}
	return builder.String()
}

func buildTypeScriptBenchmarkSource(functionCount int) string {
	var builder strings.Builder
	builder.WriteString("import fs from \"fs\";\n")
	builder.WriteString("const path = require(\"path\");\n\n")
	for i := 0; i < functionCount; i++ {
		builder.WriteString(fmt.Sprintf("const value%d = %d;\n", i, i))
		builder.WriteString(fmt.Sprintf("const add%d = (x: number) => x + value%d;\n", i, i))
		builder.WriteString(fmt.Sprintf("export function run%d(input: number) {\n", i))
		builder.WriteString(fmt.Sprintf("  return add%d(input);\n", i))
		builder.WriteString("}\n")
		builder.WriteString(fmt.Sprintf("class Service%d {\n", i))
		builder.WriteString(fmt.Sprintf("  start() { return run%d(value%d); }\n", i, i))
		builder.WriteString("}\n\n")
	}
	return builder.String()
}
