package inspector

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkAnalyzeSources(b *testing.B) {
	cases := []struct {
		name     string
		language string
		source   []byte
	}{
		{"go", "go", []byte(buildGoBenchmarkSource(240))},
		{"python", "python", []byte(buildPythonBenchmarkSource(320))},
		{"typescript", "typescript", []byte(buildTypeScriptBenchmarkSource(280))},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tc.source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				metrics, err := analyzeSource(tc.language, tc.source)
				if err != nil {
					b.Fatalf("analyzeSource(%q) returned error: %v", tc.language, err)
				}
				if metrics == nil {
					b.Fatalf("analyzeSource(%q) returned nil metrics", tc.language)
				}
			}
		})
	}
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
