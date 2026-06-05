package inspector

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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

	var metrics *FileMetrics
	switch language {
	case "go":
		metrics, err = analyzeGoSource(source)
	case "python":
		metrics, err = analyzePythonSource(source)
	case "javascript", "jsx", "typescript":
		metrics, err = analyzeJavaScriptLikeSource(source, language)
	default:
		return nil, false, nil
	}
	if metrics == nil {
		metrics = &FileMetrics{Language: language}
	}
	metrics.Language = language
	metrics.LineCount = countPhysicalLines(source)

	if len(metrics.Functions) > 1 {
		sort.SliceStable(metrics.Functions, func(i, j int) bool {
			if metrics.Functions[i].Line == metrics.Functions[j].Line {
				return metrics.Functions[i].Name < metrics.Functions[j].Name
			}
			return metrics.Functions[i].Line < metrics.Functions[j].Line
		})
	}

	if err != nil {
		return metrics, true, err
	}
	return metrics, true, nil
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
