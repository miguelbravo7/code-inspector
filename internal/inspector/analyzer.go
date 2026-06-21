package inspector

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LanguageAnalyzer extracts metrics from the source of a single language.
type LanguageAnalyzer interface {
	Analyze(source []byte) (*FileMetrics, error)
}

var supportedExtensions = map[string]string{
	".js":  "javascript",
	".mjs": "javascript",
	".cjs": "javascript",
	".jsx": "jsx",
	".ts":  "typescript",
	".tsx": "tsx",
	".py":  "python",
	".go":  "go",
}

// languageAnalyzers maps a language to its analyzer. Go uses the standard
// library AST; every other language is parsed with tree-sitter.
var languageAnalyzers = map[string]LanguageAnalyzer{
	"go":         goAnalyzer{},
	"python":     newTreeSitterAnalyzer(pythonSpec()),
	"javascript": newTreeSitterAnalyzer(javascriptSpec()),
	"jsx":        newTreeSitterAnalyzer(javascriptSpec()),
	"typescript": newTreeSitterAnalyzer(typescriptSpec()),
	"tsx":        newTreeSitterAnalyzer(tsxSpec()),
}

// AnalyzeFile extracts metrics for supported source files.
// The returned bool reports whether the file extension is supported.
func AnalyzeFile(path string) (*FileMetrics, bool, error) {
	ext := strings.ToLower(filepath.Ext(path))
	language, ok := supportedExtensions[ext]
	if !ok {
		return nil, false, nil
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return nil, true, fmt.Errorf("read file %q: %w", path, err)
	}

	metrics, err := analyzeSource(language, source)
	if metrics == nil {
		metrics = &FileMetrics{Language: language}
	}
	metrics.Language = language
	metrics.LineCount = countPhysicalLines(source)
	applyFunctionRollups(metrics)
	sortFunctions(metrics.Functions)

	if err != nil {
		return metrics, true, err
	}
	return metrics, true, nil
}

// analyzeSource dispatches to the analyzer registered for the language. It is
// the single entry point used by AnalyzeFile and the tests.
func analyzeSource(language string, source []byte) (*FileMetrics, error) {
	analyzer, found := languageAnalyzers[language]
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
	_, ok := supportedExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func clampLineCount(startLine, endLine int) int {
	if startLine <= 0 || endLine < startLine {
		return 1
	}
	return endLine - startLine + 1
}
