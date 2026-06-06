package inspector

// FunctionInfo stores function metadata discovered in a source file.
type FunctionInfo struct {
	Name      string
	Signature string
	Line      int
	LineCount int
}

// FileMetrics stores per-file metrics extracted by analyzers.
type FileMetrics struct {
	Language      string
	LineCount     int
	ImportCount   int
	VariableCount int
	Functions     []FunctionInfo
}

// FunctionCount returns the number of discovered functions in this file.
func (m *FileMetrics) FunctionCount() int {
	if m == nil {
		return 0
	}
	return len(m.Functions)
}

// TreeNode is a directory or file entry in the output tree.
type TreeNode struct {
	Name     string
	Path     string
	IsDir    bool
	Metrics  *FileMetrics
	Children []*TreeNode
	Warning  string
}

// Config controls traversal and filtering behavior.
type Config struct {
	ExcludedDirs  map[string]struct{}
	SupportedOnly bool
	// AnalyzerWorkers controls per-directory file analysis workers.
	// 0 uses automatic sizing, 1 forces sequential file analysis.
	AnalyzerWorkers int
}
