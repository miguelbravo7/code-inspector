package inspector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const defaultTraversalFixtureFileCount = 1600

func BenchmarkBuildTreeTraversal(b *testing.B) {
	rootPath := createTraversalBenchmarkFixture(b)
	concurrentCfg := Config{
		ExcludedDirs:  BuildExcludeSet(false, nil),
		SupportedOnly: true,
	}
	sequentialCfg := concurrentCfg
	sequentialCfg.AnalyzerWorkers = 1

	b.Run("concurrent", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tree, err := BuildTree(rootPath, concurrentCfg)
			if err != nil {
				b.Fatalf("BuildTree returned error: %v", err)
			}
			if tree == nil {
				b.Fatalf("BuildTree returned nil tree")
			}
		}
	})

	b.Run("sequential_baseline", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tree, err := BuildTree(rootPath, sequentialCfg)
			if err != nil {
				b.Fatalf("BuildTree(workers=1) returned error: %v", err)
			}
			if tree == nil {
				b.Fatalf("BuildTree(workers=1) returned nil tree")
			}
		}
	})
}

func BenchmarkBuildTreeTraversalMatrix(b *testing.B) {
	fileCounts := []int{200, 800, 1600}
	supportedModes := []bool{true, false}

	for _, fileCount := range fileCounts {
		fileCount := fileCount
		for _, supportedOnly := range supportedModes {
			supportedOnly := supportedOnly
			caseName := fmt.Sprintf("files=%d/supported_only=%t", fileCount, supportedOnly)

			b.Run(caseName, func(b *testing.B) {
				rootPath := createTraversalBenchmarkFixtureWithFileCount(b, fileCount)
				concurrentCfg := Config{
					ExcludedDirs:  BuildExcludeSet(false, nil),
					SupportedOnly: supportedOnly,
				}
				sequentialCfg := concurrentCfg
				sequentialCfg.AnalyzerWorkers = 1

				b.Run("concurrent", func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree, err := BuildTree(rootPath, concurrentCfg)
						if err != nil {
							b.Fatalf("BuildTree returned error: %v", err)
						}
						if tree == nil {
							b.Fatalf("BuildTree returned nil tree")
						}
					}
				})

				b.Run("sequential_baseline", func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree, err := BuildTree(rootPath, sequentialCfg)
						if err != nil {
							b.Fatalf("BuildTree(workers=1) returned error: %v", err)
						}
						if tree == nil {
							b.Fatalf("BuildTree(workers=1) returned nil tree")
						}
					}
				})
			})
		}
	}
}

func createTraversalBenchmarkFixture(b *testing.B) string {
	return createTraversalBenchmarkFixtureWithFileCount(b, defaultTraversalFixtureFileCount)
}

func createTraversalBenchmarkFixtureWithFileCount(b *testing.B, totalFileCount int) string {
	b.Helper()
	if totalFileCount <= 0 {
		totalFileCount = 1
	}

	const directoryCount = 10
	const bucketCount = directoryCount * 2 // per directory: root level + nested level
	if totalFileCount < bucketCount {
		totalFileCount = bucketCount
	}

	filesPerBucket := totalFileCount / bucketCount
	remainder := totalFileCount % bucketCount

	rootPath := b.TempDir()
	globalFileIdx := 0
	bucketIdx := 0

	for dirIdx := 0; dirIdx < directoryCount; dirIdx++ {
		dirPath := filepath.Join(rootPath, fmt.Sprintf("pkg_%02d", dirIdx))
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			b.Fatalf("mkdir failed for %q: %v", dirPath, err)
		}

		nestedPath := filepath.Join(dirPath, "nested")
		if err := os.MkdirAll(nestedPath, 0o755); err != nil {
			b.Fatalf("mkdir failed for %q: %v", nestedPath, err)
		}

		paths := []string{dirPath, nestedPath}
		for _, targetPath := range paths {
			fileCount := filesPerBucket
			if bucketIdx < remainder {
				fileCount++
			}
			bucketIdx++

			for i := 0; i < fileCount; i++ {
				filename, content := traversalFixtureFile(globalFileIdx, dirIdx)
				if err := os.WriteFile(filepath.Join(targetPath, filename), []byte(content), 0o644); err != nil {
					b.Fatalf("write failed for %q: %v", filename, err)
				}
				globalFileIdx++
			}
		}
	}

	return rootPath
}

func traversalFixtureFile(fileIdx int, dirIdx int) (string, string) {
	switch fileIdx % 4 {
	case 0:
		name := fmt.Sprintf("worker_%03d.go", fileIdx)
		content := fmt.Sprintf("package bench\n\nimport \"fmt\"\n\nfunc work%d(x int) int {\n\tlocal := x + %d\n\thelper := func(v int) int { return v + local }\n\tfmt.Println(helper(local))\n\treturn helper(local)\n}\n", fileIdx, dirIdx)
		return name, content
	case 1:
		name := fmt.Sprintf("worker_%03d.py", fileIdx)
		content := fmt.Sprintf("import os\n\nvalue_%d = %d\nleft, right = 1, 2\n\ndef run_%d(x):\n\ttmp = x + value_%d\n\treturn tmp\n", fileIdx, dirIdx, fileIdx, fileIdx)
		return name, content
	case 2:
		name := fmt.Sprintf("worker_%03d.ts", fileIdx)
		content := fmt.Sprintf("import fs from \"fs\";\nconst path = require(\"path\");\nconst value%d = %d;\nconst plus%d = (x: number) => x + value%d;\nexport function run%d(input: number) {\n  return plus%d(input);\n}\n", fileIdx, dirIdx, fileIdx, fileIdx, fileIdx, fileIdx)
		return name, content
	default:
		name := fmt.Sprintf("notes_%03d.txt", fileIdx)
		return name, strings.Repeat("not source code\n", 4)
	}
}
