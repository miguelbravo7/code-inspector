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

	// Phase 1: build the tree structure sequentially (I/O only) and collect the
	// supported files. Phase 2: analyze them with a single pool spanning the whole
	// tree (so parallelism is not limited to one directory at a time).
	var pending []*TreeNode
	if err := buildSubtree(root, cfg, absRoot, &pending); err != nil {
		return nil, err
	}
	analyzeFiles(pending, cfg.AnalyzerWorkers)

	if cfg.SupportedOnly {
		pruneUnsupportedDirectories(root)
	}
	return root, nil
}

// buildSubtree builds the tree structure under parent (sequential, I/O only),
// appending every supported file node to pending for later parallel analysis.
func buildSubtree(parent *TreeNode, cfg Config, rootPath string, pending *[]*TreeNode) error {
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
			if isExcludedDir(name, cfg.ExcludedDirs) || isExcludedPath(name, fullPath, cfg.ExcludePatterns) {
				continue
			}
			dirNode := &TreeNode{Name: name, Path: fullPath, RelPath: relPath(rootPath, fullPath), IsDir: true}
			if err := buildSubtree(dirNode, cfg, rootPath, pending); err != nil {
				dirNode.Warning = err.Error()
			}
			parent.Children = append(parent.Children, dirNode)
			continue
		}

		if isExcludedPath(name, fullPath, cfg.ExcludePatterns) {
			continue
		}

		supported := isSupportedExtension(name)
		if cfg.SupportedOnly && !supported {
			continue
		}
		fileNode := &TreeNode{Name: name, Path: fullPath, RelPath: relPath(rootPath, fullPath), IsDir: false}
		parent.Children = append(parent.Children, fileNode)
		if supported {
			*pending = append(*pending, fileNode)
		}
	}

	return nil
}

// analyzeFiles fills metrics for every collected file node using a single bounded
// worker pool spanning the whole tree. workers: 1 = sequential, 0 = auto. Each
// worker writes only to its own node, so no synchronization is needed.
func analyzeFiles(nodes []*TreeNode, configuredWorkerCount int) {
	if len(nodes) == 0 {
		return
	}

	workerCount := normalizeWorkerCount(len(nodes), configuredWorkerCount)
	if workerCount <= 1 {
		for _, node := range nodes {
			analyzeInto(node)
		}
		return
	}

	tasks := make(chan *TreeNode)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range tasks {
				analyzeInto(node)
			}
		}()
	}
	for _, node := range nodes {
		tasks <- node
	}
	close(tasks)
	wg.Wait()
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

func analyzeInto(node *TreeNode) {
	metrics, supported, err := AnalyzeFile(node.Path)
	if supported {
		node.Metrics = metrics
	}
	if err != nil {
		node.Warning = err.Error()
	}
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
