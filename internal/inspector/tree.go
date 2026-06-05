package inspector

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	for _, entry := range entries {
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
