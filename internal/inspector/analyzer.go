package inspector

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type analyzerFunc func(source []byte) (*FileMetrics, error)

var supportedExtensions = map[string]string{
	".js":  "javascript",
	".mjs": "javascript",
	".cjs": "javascript",
	".jsx": "jsx",
	".ts":  "typescript",
	".tsx": "typescript",
	".py":  "python",
	".go":  "go",
}

var languageAnalyzers = map[string]analyzerFunc{
	"go":         analyzeGoSource,
	"python":     analyzePythonSource,
	"javascript": func(source []byte) (*FileMetrics, error) { return analyzeJavaScriptLikeSource(source, "javascript") },
	"jsx":        func(source []byte) (*FileMetrics, error) { return analyzeJavaScriptLikeSource(source, "jsx") },
	"typescript": func(source []byte) (*FileMetrics, error) { return analyzeJavaScriptLikeSource(source, "typescript") },
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

	analyzer, found := languageAnalyzers[language]
	if !found {
		return nil, false, nil
	}

	metrics, err := analyzer(source)
	if metrics == nil {
		metrics = &FileMetrics{Language: language}
	}
	metrics.Language = language
	metrics.LineCount = countPhysicalLines(source)
	sortFunctions(metrics.Functions)

	if err != nil {
		return metrics, true, err
	}
	return metrics, true, nil
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
