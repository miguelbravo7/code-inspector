package inspector

import (
	"fmt"
	"os"
	"path"
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
		Name:    info.Name(),
		Path:    absRoot,
		RelPath: ".",
		IsDir:   true,
	}
	if err := walkTree(root, cfg, absRoot); err != nil {
		return nil, err
	}
	if cfg.SupportedOnly {
		pruneUnsupportedDirectories(root)
	}
	return root, nil
}

func walkTree(parent *TreeNode, cfg Config, rootPath string) error {
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

	analyzedFiles := analyzeFilesInDirectory(parent.Path, entries, cfg.AnalyzerWorkers)

	for idx, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(parent.Path, name)

		if entry.IsDir() {
			if isExcludedDir(name, cfg.ExcludedDirs) || isExcludedPath(name, fullPath, cfg.ExcludePatterns) {
				continue
			}
			dirNode := &TreeNode{Name: name, Path: fullPath, RelPath: relPath(rootPath, fullPath), IsDir: true}
			if err := walkTree(dirNode, cfg, rootPath); err != nil {
				dirNode.Warning = err.Error()
			}
			parent.Children = append(parent.Children, dirNode)
			continue
		}

		if isExcludedPath(name, fullPath, cfg.ExcludePatterns) {
			continue
		}

		analyzed, ok := analyzedFiles[idx]
		if !ok {
			continue
		}

		if cfg.SupportedOnly && !analyzed.supported {
			continue
		}
		analyzed.node.RelPath = relPath(rootPath, analyzed.node.Path)
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

func analyzeFilesInDirectory(parentPath string, entries []os.DirEntry, configuredWorkerCount int) map[int]fileAnalysisResult {
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

	workerCount := normalizeWorkerCount(len(fileIndexes), configuredWorkerCount)
	if workerCount == 1 {
		analyzed := make(map[int]fileAnalysisResult, len(fileIndexes))
		for _, idx := range fileIndexes {
			analyzed[idx] = analyzeFileEntry(parentPath, entries[idx])
		}
		return analyzed
	}

	tasks := make(chan int, len(fileIndexes))
	results := make(chan indexedFileAnalysisResult, len(fileIndexes))

	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range tasks {
				results <- indexedFileAnalysisResult{
					index:  idx,
					result: analyzeFileEntry(parentPath, entries[idx]),
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

func normalizeWorkerCount(fileCount int, configuredWorkerCount int) int {
	if fileCount <= 0 {
		return 0
	}

	workerCount := configuredWorkerCount
	if workerCount <= 0 {
		workerCount = runtime.GOMAXPROCS(0)
	}
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > fileCount {
		workerCount = fileCount
	}
	return workerCount
}

func analyzeFileEntry(parentPath string, entry os.DirEntry) fileAnalysisResult {
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

	return fileAnalysisResult{node: fileNode, supported: supported}
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

func relPath(rootPath, fullPath string) string {
	rel, err := filepath.Rel(rootPath, fullPath)
	if err != nil {
		return filepath.ToSlash(fullPath)
	}
	return filepath.ToSlash(rel)
}

func isExcludedDir(name string, excluded map[string]struct{}) bool {
	if len(excluded) == 0 {
		return false
	}
	_, ok := excluded[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func isExcludedPath(name string, fullPath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	normalizedName := strings.ToLower(strings.TrimSpace(name))
	normalizedPath := strings.ToLower(filepath.ToSlash(strings.TrimSpace(fullPath)))

	for _, rawPattern := range patterns {
		pattern := strings.ToLower(strings.TrimSpace(rawPattern))
		if pattern == "" {
			continue
		}

		if pattern == normalizedName {
			return true
		}
		if !strings.ContainsAny(pattern, "*?[") && strings.HasSuffix(normalizedPath, "/"+pattern) {
			return true
		}
		if matchesExcludePattern(pattern, normalizedName) || matchesExcludePattern(pattern, normalizedPath) {
			return true
		}

		relativeCandidate := normalizedPath
		for {
			nextSlash := strings.Index(relativeCandidate, "/")
			if nextSlash < 0 {
				break
			}
			relativeCandidate = relativeCandidate[nextSlash+1:]
			if matchesExcludePattern(pattern, relativeCandidate) {
				return true
			}
		}
	}

	return false
}

func matchesExcludePattern(pattern string, value string) bool {
	matched, err := path.Match(pattern, value)
	return err == nil && matched
}
