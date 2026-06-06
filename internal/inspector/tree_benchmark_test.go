package inspector

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func BenchmarkBuildTreeTraversal(b *testing.B) {
	rootPath := createTraversalBenchmarkFixture(b)
	cfg := Config{
		ExcludedDirs:  BuildExcludeSet(false, nil),
		SupportedOnly: true,
	}

	b.Run("concurrent", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tree, err := BuildTree(rootPath, cfg)
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
			tree, err := buildTreeSequentialBenchmark(rootPath, cfg)
			if err != nil {
				b.Fatalf("buildTreeSequentialBenchmark returned error: %v", err)
			}
			if tree == nil {
				b.Fatalf("buildTreeSequentialBenchmark returned nil tree")
			}
		}
	})
}

func createTraversalBenchmarkFixture(b *testing.B) string {
	b.Helper()

	rootPath := b.TempDir()
	for dirIdx := 0; dirIdx < 10; dirIdx++ {
		dirPath := filepath.Join(rootPath, fmt.Sprintf("pkg_%02d", dirIdx))
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			b.Fatalf("mkdir failed for %q: %v", dirPath, err)
		}

		nestedPath := filepath.Join(dirPath, "nested")
		if err := os.MkdirAll(nestedPath, 0o755); err != nil {
			b.Fatalf("mkdir failed for %q: %v", nestedPath, err)
		}

		for fileIdx := 0; fileIdx < 80; fileIdx++ {
			filename, content := traversalFixtureFile(fileIdx, dirIdx)
			if err := os.WriteFile(filepath.Join(dirPath, filename), []byte(content), 0o644); err != nil {
				b.Fatalf("write failed for %q: %v", filename, err)
			}
		}
		for fileIdx := 0; fileIdx < 80; fileIdx++ {
			filename, content := traversalFixtureFile(fileIdx+80, dirIdx)
			if err := os.WriteFile(filepath.Join(nestedPath, filename), []byte(content), 0o644); err != nil {
				b.Fatalf("write failed for %q: %v", filename, err)
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

func buildTreeSequentialBenchmark(rootPath string, cfg Config) (*TreeNode, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path %q: %w", rootPath, err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat path %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path %q is not a directory", absRoot)
	}

	root := &TreeNode{Name: info.Name(), Path: absRoot, IsDir: true}
	if err := walkTreeSequentialBenchmark(root, cfg); err != nil {
		return nil, err
	}
	if cfg.SupportedOnly {
		pruneUnsupportedDirectories(root)
	}
	return root, nil
}

func walkTreeSequentialBenchmark(parent *TreeNode, cfg Config) error {
	entries, err := os.ReadDir(parent.Path)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", parent.Path, err)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		iDir := entries[i].IsDir()
		jDir := entries[j].IsDir()
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	for _, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(parent.Path, name)

		if entry.IsDir() {
			if isExcludedDir(name, cfg.ExcludedDirs) {
				continue
			}
			dirNode := &TreeNode{Name: name, Path: fullPath, IsDir: true}
			if err := walkTreeSequentialBenchmark(dirNode, cfg); err != nil {
				dirNode.Warning = err.Error()
			}
			parent.Children = append(parent.Children, dirNode)
			continue
		}

		fileNode := &TreeNode{Name: name, Path: fullPath, IsDir: false}
		metrics, supported, analyzeErr := AnalyzeFile(fullPath)
		if supported {
			fileNode.Metrics = metrics
		}
		if analyzeErr != nil {
			fileNode.Warning = analyzeErr.Error()
		}

		if cfg.SupportedOnly && !supported {
			continue
		}
		parent.Children = append(parent.Children, fileNode)
	}

	return nil
}
