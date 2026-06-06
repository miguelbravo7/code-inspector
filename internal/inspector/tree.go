package inspector

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var defaultExcludedDirs = []string{
	".git",
	"node_modules",
	"dist",
	"build",
	"out",
	"vendor",
}

// BuildExcludeSet constructs a normalized set of excluded directory names.
func BuildExcludeSet(includeDefaults bool, extras []string) map[string]struct{} {
	excluded := make(map[string]struct{})
	if includeDefaults {
		for _, name := range defaultExcludedDirs {
			excluded[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
		}
	}
	for _, extra := range extras {
		trimmed := strings.ToLower(strings.TrimSpace(extra))
		if trimmed == "" {
			continue
		}
		excluded[trimmed] = struct{}{}
	}
	return excluded
}

// BuildTree traverses the target directory and builds a deterministic tree.
func BuildTree(rootPath string, cfg Config) (*TreeNode, error) {
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

	root := &TreeNode{
		Name:  info.Name(),
		Path:  absRoot,
		IsDir: true,
	}
	if err := walkTree(root, cfg); err != nil {
		return nil, err
	}
	if cfg.SupportedOnly {
		pruneUnsupportedDirectories(root)
	}
	return root, nil
}

func walkTree(parent *TreeNode, cfg Config) error {
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

	analyzedFiles := analyzeFilesInDirectory(parent.Path, entries)

	for idx, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(parent.Path, name)

		if entry.IsDir() {
			if isExcludedDir(name, cfg.ExcludedDirs) {
				continue
			}
			dirNode := &TreeNode{Name: name, Path: fullPath, IsDir: true}
			if err := walkTree(dirNode, cfg); err != nil {
				dirNode.Warning = err.Error()
			}
			parent.Children = append(parent.Children, dirNode)
			continue
		}

		analyzed, ok := analyzedFiles[idx]
		if !ok {
			continue
		}

		if cfg.SupportedOnly && !analyzed.supported {
			continue
		}
		parent.Children = append(parent.Children, analyzed.node)
	}

	return nil
}

type fileAnalysisResult struct {
	node      *TreeNode
	supported bool
}

type indexedFileAnalysisResult struct {
	index  int
	result fileAnalysisResult
}

func analyzeFilesInDirectory(parentPath string, entries []os.DirEntry) map[int]fileAnalysisResult {
	fileIndexes := make([]int, 0, len(entries))
	for idx, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileIndexes = append(fileIndexes, idx)
	}

	if len(fileIndexes) == 0 {
		return nil
	}

	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(fileIndexes) {
		workerCount = len(fileIndexes)
	}

	tasks := make(chan int, len(fileIndexes))
	results := make(chan indexedFileAnalysisResult, len(fileIndexes))

	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range tasks {
				entry := entries[idx]
				name := entry.Name()
				fullPath := filepath.Join(parentPath, name)

				fileNode := &TreeNode{Name: name, Path: fullPath, IsDir: false}
				metrics, supported, analyzeErr := AnalyzeFile(fullPath)
				if supported {
					fileNode.Metrics = metrics
				}
				if analyzeErr != nil {
					fileNode.Warning = analyzeErr.Error()
				}

				results <- indexedFileAnalysisResult{
					index: idx,
					result: fileAnalysisResult{
						node:      fileNode,
						supported: supported,
					},
				}
			}
		}()
	}

	for _, idx := range fileIndexes {
		tasks <- idx
	}
	close(tasks)

	wg.Wait()
	close(results)

	analyzed := make(map[int]fileAnalysisResult, len(fileIndexes))
	for item := range results {
		analyzed[item.index] = item.result
	}

	return analyzed
}

func pruneUnsupportedDirectories(node *TreeNode) bool {
	if !node.IsDir {
		return node.Metrics != nil
	}
	kept := node.Children[:0]
	for _, child := range node.Children {
		if child.IsDir {
			if pruneUnsupportedDirectories(child) {
				kept = append(kept, child)
			}
			continue
		}
		if child.Metrics != nil {
			kept = append(kept, child)
		}
	}
	node.Children = kept
	return len(node.Children) > 0
}

func isExcludedDir(name string, excluded map[string]struct{}) bool {
	if len(excluded) == 0 {
		return false
	}
	_, ok := excluded[strings.ToLower(strings.TrimSpace(name))]
	return ok
}
