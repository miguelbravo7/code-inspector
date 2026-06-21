package inspector

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// LanguageAnalyzer extracts metrics from the source of a single language.
type LanguageAnalyzer interface {
	Analyze(source []byte) (*FileMetrics, error)
}

// LanguageConfig registers a tree-sitter grammar for a set of file extensions.
// Construct Grammar with sitter.NewLanguage(grammarPackage.Language()).
type LanguageConfig struct {
	Name       string           // canonical language name, e.g. "rust"
	Extensions []string         // file extensions including the dot, e.g. []string{".rs"}
	Grammar    *sitter.Language // required tree-sitter grammar
	Hints      *LanguageHints   // optional; augments the heuristic defaults
}

// LanguageHints lets a caller refine the generic analyzer for a language by
// naming the relevant tree-sitter node kinds. Any unset field falls back to the
// curated cross-language defaults.
type LanguageHints struct {
	FunctionKinds []string // node kinds that define a function/method
	DecisionKinds []string // node kinds counting +1 cyclomatic
	NestingKinds  []string // node kinds that increase nesting (cognitive/depth)
	ImportKinds   []string // node kinds that are imports (counted)
	NameField     string   // field name for a definition's name (default "name")
	ParamsField   string   // field name for the parameter list (default "parameters")
}

type registeredLanguage struct {
	name     string
	analyzer LanguageAnalyzer
}

var (
	registryMu         sync.RWMutex
	extensionRegistry  = map[string]registeredLanguage{} // ".rs" -> {rust, analyzer}
	languageToAnalyzer = map[string]LanguageAnalyzer{}   // "rust" -> analyzer (for analyzeSource/tests)
)

func registerAnalyzer(name string, analyzer LanguageAnalyzer, extensions ...string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	languageToAnalyzer[name] = analyzer
	for _, ext := range extensions {
		extensionRegistry[strings.ToLower(ext)] = registeredLanguage{name: name, analyzer: analyzer}
	}
}

func init() {
	registerAnalyzer("go", goAnalyzer{}, ".go")
	registerAnalyzer("python", newTreeSitterAnalyzer(pythonSpec()), ".py")
	registerAnalyzer("javascript", newTreeSitterAnalyzer(javascriptSpec()), ".js", ".mjs", ".cjs")
	registerAnalyzer("jsx", newTreeSitterAnalyzer(javascriptSpec()), ".jsx")
	registerAnalyzer("typescript", newTreeSitterAnalyzer(typescriptSpec()), ".ts")
	registerAnalyzer("tsx", newTreeSitterAnalyzer(tsxSpec()), ".tsx")
}

// RegisterLanguage adds (or overrides) support for a tree-sitter grammar mapped
// to one or more file extensions. The grammar is analyzed with the generic
// heuristic adapter; pass Hints to improve accuracy. It is safe to call at
// program startup before analysis begins.
func RegisterLanguage(cfg LanguageConfig) {
	if cfg.Grammar == nil || len(cfg.Extensions) == 0 {
		return
	}
	name := cfg.Name
	if name == "" {
		name = strings.TrimPrefix(cfg.Extensions[0], ".")
	}
	analyzer := newTreeSitterAnalyzer(genericSpec(name, cfg.Grammar, cfg.Hints))
	registerAnalyzer(name, analyzer, cfg.Extensions...)
}

// SupportedExtensions returns the registered file extensions, sorted.
func SupportedExtensions() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	exts := make([]string, 0, len(extensionRegistry))
	for ext := range extensionRegistry {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

// SupportedLanguages returns the registered language names, sorted.
func SupportedLanguages() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(languageToAnalyzer))
	for name := range languageToAnalyzer {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func lookupByExtension(ext string) (registeredLanguage, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	entry, ok := extensionRegistry[ext]
	return entry, ok
}

func lookupByName(name string) (LanguageAnalyzer, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := languageToAnalyzer[name]
	return a, ok
}

// AnalyzeFile extracts metrics for supported source files.
// The returned bool reports whether the file extension is supported.
func AnalyzeFile(path string) (*FileMetrics, bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	entry, ok := lookupByExtension(ext)
	if !ok {
		return nil, false, nil
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return nil, true, fmt.Errorf("read file %q: %w", path, err)
	}

	metrics, err := entry.analyzer.Analyze(source)
	if metrics == nil {
		metrics = &FileMetrics{Language: entry.name}
	}
	metrics.Language = entry.name
	metrics.LineCount = countPhysicalLines(source)
	applyFunctionRollups(metrics)
	sortFunctions(metrics.Functions)

	if err != nil {
		return metrics, true, err
	}
	return metrics, true, nil
}

// analyzeSource dispatches to the analyzer registered for the language name. It
// is the single entry point used by the tests.
func analyzeSource(language string, source []byte) (*FileMetrics, error) {
	analyzer, found := lookupByName(language)
	if !found {
		return &FileMetrics{Language: language}, nil
	}
	return analyzer.Analyze(source)
}

func sortFunctions(functions []FunctionInfo) {
	if len(functions) < 2 {
		return
	}

	sort.SliceStable(functions, func(i, j int) bool {
		left := functions[i]
		right := functions[j]

		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Signature < right.Signature
	})
}

func countPhysicalLines(source []byte) int {
	if len(source) == 0 {
		return 0
	}
	return bytes.Count(source, []byte{'\n'}) + 1
}

func isSupportedExtension(path string) bool {
	_, ok := lookupByExtension(strings.ToLower(filepath.Ext(path)))
	return ok
}

func clampLineCount(startLine, endLine int) int {
	if startLine <= 0 || endLine < startLine {
		return 1
	}
	return endLine - startLine + 1
}
