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
	idx := buildDepIndex(files)

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
			to, resolved := resolveImport(f, spec, knownFiles, moduleRoot, modulePath, idx)
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

func resolveImport(f depFileInfo, spec string, knownFiles map[string]struct{}, moduleRoot, modulePath string, idx depIndex) (string, bool) {
	switch f.language {
	case "go":
		if modulePath == "" || !strings.HasPrefix(spec, modulePath) {
			return "", false
		}
		return spec, true
	case "python":
		return resolvePythonImport(f.rel, spec, knownFiles)
	case "javascript", "jsx", "typescript", "tsx":
		return resolveRelativeImport(f.rel, spec, knownFiles)
	default:
		return resolveGenericImport(f, spec, idx)
	}
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

// depSuffixCap bounds how many trailing path segments are indexed per file, to
// keep the suffix index memory linear while covering realistic import depths.
const depSuffixCap = 6

// depIndex supports precision-first resolution of imports to project files.
type depIndex struct {
	byExact    map[string]struct{} // slash rel path -> exists
	nameSuffix map[string][]string // trailing path suffix (no final ext) -> rel paths
	pathSuffix map[string][]string // trailing path suffix (with ext) -> rel paths
	fileLang   map[string]string   // rel path -> language
	langExts   map[string][]string // language -> its registered extensions
}

func buildDepIndex(files []depFileInfo) depIndex {
	idx := depIndex{
		byExact:    make(map[string]struct{}, len(files)),
		nameSuffix: make(map[string][]string),
		pathSuffix: make(map[string][]string),
		fileLang:   make(map[string]string, len(files)),
		langExts:   make(map[string][]string),
	}
	for _, f := range files {
		rel := f.rel
		idx.byExact[rel] = struct{}{}
		idx.fileLang[rel] = f.language

		segs := strings.Split(rel, "/")
		addSuffixKeys(idx.pathSuffix, segs, rel)

		nameSegs := append([]string(nil), segs...)
		last := nameSegs[len(nameSegs)-1]
		stem := strings.TrimSuffix(last, path.Ext(last))
		nameSegs[len(nameSegs)-1] = stem
		addSuffixKeys(idx.nameSuffix, nameSegs, rel)

		// Directory-module conventions: foo/mod.rs, pkg/__init__.py, dir/index.* are
		// referenced by their directory name, so also index the parent suffixes.
		switch stem {
		case "mod", "index", "__init__":
			if len(nameSegs) >= 2 {
				addSuffixKeys(idx.nameSuffix, nameSegs[:len(nameSegs)-1], rel)
			}
		}
	}
	for k, v := range idx.nameSuffix {
		idx.nameSuffix[k] = dedupStrings(v)
	}
	for k, v := range idx.pathSuffix {
		idx.pathSuffix[k] = dedupStrings(v)
	}

	for _, ext := range SupportedExtensions() {
		if entry, ok := lookupByExtension(ext); ok {
			idx.langExts[entry.name] = append(idx.langExts[entry.name], ext)
		}
	}
	return idx
}

func addSuffixKeys(m map[string][]string, segs []string, rel string) {
	start := 0
	if len(segs) > depSuffixCap {
		start = len(segs) - depSuffixCap
	}
	for i := len(segs) - 1; i >= start; i-- {
		key := strings.Join(segs[i:], "/")
		if key != "" {
			m[key] = append(m[key], rel)
		}
	}
}

func dedupStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// resolveGenericImport resolves an import specifier for a language without a
// dedicated resolver. It is precision-first: a path-like specifier resolves
// relative to the importer (or by unique path-suffix), a name-like (dotted/
// scoped) specifier resolves by UNIQUE path-suffix within the importer's
// language family. Anything ambiguous or unmatched is treated as external.
func resolveGenericImport(f depFileInfo, spec string, idx depIndex) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	sep, pathLike := classifySpecifier(spec)
	if pathLike {
		return resolvePathLike(f, spec, idx)
	}
	return resolveNameLike(f, spec, sep, idx)
}

func classifySpecifier(spec string) (sep string, pathLike bool) {
	switch {
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"), strings.HasPrefix(spec, "/"):
		return "/", true
	case strings.Contains(spec, "::"):
		return "::", false
	case strings.Contains(spec, "\\"):
		return "\\", false
	case strings.Contains(spec, "/"):
		return "/", true
	}
	if dot := strings.LastIndex(spec, "."); dot >= 0 {
		if hasKnownExtension(spec) {
			return "/", true // filename.ext
		}
		return ".", false // dotted name (e.g. com.example.Foo)
	}
	return "", false // single bare token
}

func resolvePathLike(f depFileInfo, spec string, idx depIndex) (string, bool) {
	base := path.Dir(f.rel)
	target := path.Clean(path.Join(base, spec))
	suffixKey := strings.TrimPrefix(spec, "./")

	exts := []string{""}
	if !hasKnownExtension(spec) {
		exts = extensionsForLanguageFamily(f.language, idx)
	}
	for _, e := range exts {
		if _, ok := idx.byExact[target+e]; ok && sameLanguageFamily(idx.fileLang[target+e], f.language) {
			return target + e, true
		}
	}
	for _, e := range exts {
		if m := uniqueFamilyMatch(idx.pathSuffix[suffixKey+e], f.language, idx); m != "" {
			return m, true
		}
	}
	return "", false
}

func resolveNameLike(f depFileInfo, spec, sep string, idx depIndex) (string, bool) {
	segs := cleanNameSegments(splitSep(spec, sep))
	if len(segs) == 0 {
		return "", false
	}
	candidate := strings.Join(segs, "/")
	if m := uniqueFamilyMatch(idx.nameSuffix[candidate], f.language, idx); m != "" {
		return m, true
	}
	return "", false
}

func splitSep(spec, sep string) []string {
	if sep == "" {
		return []string{spec}
	}
	return strings.Split(spec, sep)
}

// cleanNameSegments trims selector/wildcard tails and leading relativity prefixes
// (crate/self/super/this) that are not directory components.
func cleanNameSegments(segs []string) []string {
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.ContainsAny(s, "{}*") {
			break // grouped selectors / wildcards: stop here
		}
		out = append(out, s)
	}
	noise := map[string]struct{}{"crate": {}, "self": {}, "super": {}, "this": {}, "@": {}}
	for len(out) > 0 {
		if _, ok := noise[out[0]]; !ok {
			break
		}
		out = out[1:]
	}
	return out
}

func uniqueFamilyMatch(candidates []string, lang string, idx depIndex) string {
	match := ""
	count := 0
	for _, rel := range candidates {
		if !sameLanguageFamily(idx.fileLang[rel], lang) {
			continue
		}
		match = rel
		count++
	}
	if count == 1 {
		return match
	}
	return ""
}

func sameLanguageFamily(a, b string) bool {
	if a == b {
		return true
	}
	cFamily := func(s string) bool { return s == "c" || s == "cpp" }
	return cFamily(a) && cFamily(b)
}

func hasKnownExtension(spec string) bool {
	ext := strings.ToLower(path.Ext(spec))
	if ext == "" {
		return false
	}
	_, ok := lookupByExtension(ext)
	return ok
}

func extensionsForLanguageFamily(lang string, idx depIndex) []string {
	var exts []string
	for l, es := range idx.langExts {
		if sameLanguageFamily(l, lang) {
			exts = append(exts, es...)
		}
	}
	return exts
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
