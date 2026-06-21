package inspector

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// DependencyStat holds the fan-in/fan-out of a single graph node.
type DependencyStat struct {
	Node   string
	FanIn  int
	FanOut int
}

// DependencyReport summarizes the internal import graph. Nodes are files for
// JavaScript/TypeScript/Python and packages (directories) for Go.
type DependencyReport struct {
	Nodes            int
	Edges            int
	ExternalImports  int
	MostDependedOn   []DependencyStat // highest fan-in: change here ripples widely
	MostDependencies []DependencyStat // highest fan-out: most fragile
	Cycles           [][]string
}

type depFileInfo struct {
	absPath  string
	rel      string
	language string
	imports  []string
}

// BuildDependencyGraph resolves intra-project imports into a graph and computes
// fan-in/fan-out plus dependency cycles.
func BuildDependencyGraph(root *TreeNode, scanRoot string, topN int) DependencyReport {
	if topN <= 0 {
		topN = 10
	}

	files := collectDepFiles(root)
	report := DependencyReport{}
	if len(files) == 0 {
		return report
	}

	knownFiles := make(map[string]struct{}, len(files))
	for _, f := range files {
		knownFiles[f.rel] = struct{}{}
	}

	moduleRoot, modulePath := findGoModule(scanRoot)

	edges := make(map[string]map[string]struct{})
	nodes := make(map[string]struct{})
	addEdge := func(from, to string) {
		if from == to {
			return
		}
		nodes[from] = struct{}{}
		nodes[to] = struct{}{}
		if edges[from] == nil {
			edges[from] = make(map[string]struct{})
		}
		edges[from][to] = struct{}{}
	}

	for _, f := range files {
		from, ok := nodeID(f, moduleRoot, modulePath)
		if !ok {
			continue
		}
		nodes[from] = struct{}{}
		for _, spec := range f.imports {
			to, resolved := resolveImport(f, spec, knownFiles, moduleRoot, modulePath)
			if !resolved {
				report.ExternalImports++
				continue
			}
			addEdge(from, to)
		}
	}

	edgeCount := 0
	fanOut := make(map[string]int)
	fanIn := make(map[string]int)
	for from, tos := range edges {
		fanOut[from] = len(tos)
		edgeCount += len(tos)
		for to := range tos {
			fanIn[to]++
		}
	}

	report.Nodes = len(nodes)
	report.Edges = edgeCount
	report.MostDependedOn = topStats(nodes, fanIn, fanOut, topN, func(s DependencyStat) int { return s.FanIn })
	report.MostDependencies = topStats(nodes, fanIn, fanOut, topN, func(s DependencyStat) int { return s.FanOut })
	report.Cycles = findCycles(edges)
	return report
}

func collectDepFiles(root *TreeNode) []depFileInfo {
	var files []depFileInfo
	var walk func(n *TreeNode)
	walk = func(n *TreeNode) {
		if !n.IsDir && n.Metrics != nil {
			rel := n.RelPath
			if rel == "" {
				rel = n.Name
			}
			files = append(files, depFileInfo{
				absPath:  n.Path,
				rel:      filepath.ToSlash(rel),
				language: n.Metrics.Language,
				imports:  n.Metrics.Imports,
			})
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	return files
}

// nodeID returns the graph node for a file: its package import path for Go,
// otherwise its scan-relative file path.
func nodeID(f depFileInfo, moduleRoot, modulePath string) (string, bool) {
	if f.language != "go" {
		return f.rel, true
	}
	if modulePath == "" {
		return "", false
	}
	rel, err := filepath.Rel(moduleRoot, f.absPath)
	if err != nil {
		return "", false
	}
	dir := path.Dir(filepath.ToSlash(rel))
	if dir == "." || dir == "" {
		return modulePath, true
	}
	return modulePath + "/" + dir, true
}

func resolveImport(f depFileInfo, spec string, knownFiles map[string]struct{}, moduleRoot, modulePath string) (string, bool) {
	if f.language == "go" {
		if modulePath == "" || !strings.HasPrefix(spec, modulePath) {
			return "", false
		}
		return spec, true
	}
	if f.language == "python" {
		return resolvePythonImport(f.rel, spec, knownFiles)
	}
	return resolveRelativeImport(f.rel, spec, knownFiles)
}

// resolveRelativeImport handles JS/TS-style relative specifiers.
func resolveRelativeImport(importerRel, spec string, knownFiles map[string]struct{}) (string, bool) {
	if !strings.HasPrefix(spec, ".") {
		return "", false // bare specifier: external dependency
	}
	base := path.Dir(importerRel)
	target := path.Clean(path.Join(base, spec))

	exts := []string{"", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}
	for _, ext := range exts {
		if _, ok := knownFiles[target+ext]; ok {
			return target + ext, true
		}
	}
	for _, idx := range []string{"/index.ts", "/index.tsx", "/index.js", "/index.jsx"} {
		if _, ok := knownFiles[target+idx]; ok {
			return target + idx, true
		}
	}
	return "", false
}

func resolvePythonImport(importerRel, spec string, knownFiles map[string]struct{}) (string, bool) {
	var base string
	module := spec
	if strings.HasPrefix(spec, ".") {
		base = path.Dir(importerRel)
		dots := 0
		for dots < len(spec) && spec[dots] == '.' {
			dots++
		}
		for i := 1; i < dots; i++ {
			base = path.Dir(base)
		}
		module = spec[dots:]
	}

	rel := strings.ReplaceAll(module, ".", "/")
	target := path.Clean(path.Join(base, rel))
	if target == "." {
		return "", false
	}
	for _, candidate := range []string{target + ".py", target + "/__init__.py"} {
		if _, ok := knownFiles[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

func topStats(nodes map[string]struct{}, fanIn, fanOut map[string]int, topN int, key func(DependencyStat) int) []DependencyStat {
	stats := make([]DependencyStat, 0, len(nodes))
	for node := range nodes {
		stats = append(stats, DependencyStat{Node: node, FanIn: fanIn[node], FanOut: fanOut[node]})
	}
	sort.SliceStable(stats, func(i, j int) bool {
		if key(stats[i]) != key(stats[j]) {
			return key(stats[i]) > key(stats[j])
		}
		return stats[i].Node < stats[j].Node
	})
	out := make([]DependencyStat, 0, topN)
	for _, s := range stats {
		if key(s) <= 0 {
			break
		}
		out = append(out, s)
		if len(out) >= topN {
			break
		}
	}
	return out
}

// findCycles returns strongly connected components with more than one node, plus
// self-loops, via Tarjan's algorithm.
func findCycles(edges map[string]map[string]struct{}) [][]string {
	index := 0
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	var cycles [][]string

	// Deterministic node ordering.
	nodeList := make([]string, 0, len(edges))
	for node := range edges {
		nodeList = append(nodeList, node)
	}
	sort.Strings(nodeList)

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		successors := make([]string, 0, len(edges[v]))
		for w := range edges[v] {
			successors = append(successors, w)
		}
		sort.Strings(successors)

		for _, w := range successors {
			if _, seen := indices[w]; !seen {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			var component []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				component = append(component, w)
				if w == v {
					break
				}
			}
			if len(component) > 1 {
				sort.Strings(component)
				cycles = append(cycles, component)
			} else if len(component) == 1 {
				node := component[0]
				if _, selfLoop := edges[node][node]; selfLoop {
					cycles = append(cycles, component)
				}
			}
		}
	}

	for _, node := range nodeList {
		if _, seen := indices[node]; !seen {
			strongConnect(node)
		}
	}

	sort.SliceStable(cycles, func(i, j int) bool {
		if len(cycles[i]) != len(cycles[j]) {
			return len(cycles[i]) > len(cycles[j])
		}
		return strings.Join(cycles[i], ",") < strings.Join(cycles[j], ",")
	})
	return cycles
}

// findGoModule walks up from scanRoot looking for go.mod and returns the module
// root directory and module path.
func findGoModule(scanRoot string) (string, string) {
	dir, err := filepath.Abs(scanRoot)
	if err != nil {
		return "", ""
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			if module := parseModulePath(data); module != "" {
				return dir, module
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

func parseModulePath(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
