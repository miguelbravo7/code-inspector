package inspector

// FunctionInfo stores function metadata discovered in a source file.
type FunctionInfo struct {
	Name      string
	Signature string
	Line      int
	LineCount int

	// Cyclomatic is the McCabe cyclomatic complexity: decision points + 1.
	Cyclomatic int
	// Cognitive is an approximation of SonarSource cognitive complexity; it
	// penalizes nesting and is a better "how hard to understand" proxy.
	Cognitive int
	// MaxNesting is the deepest control-flow nesting level inside the function.
	MaxNesting int
	// Params is the number of declared parameters.
	Params int
	// Maintainability is a 0-100 Maintainability Index for the function
	// (higher is better).
	Maintainability float64
}

// Halstead holds the Halstead complexity measures derived from the operators
// and operands in a unit of code.
type Halstead struct {
	Vocabulary int     // distinct operators + distinct operands
	Length     int     // total operators + total operands
	Volume     float64 // Length * log2(Vocabulary)
	Difficulty float64 // (distinctOperators/2) * (totalOperands/distinctOperands)
	Effort     float64 // Difficulty * Volume
}

// FileMetrics stores per-file metrics extracted by analyzers.
type FileMetrics struct {
	Language      string
	LineCount     int // physical line count
	CodeLines     int
	CommentLines  int
	BlankLines    int
	ImportCount   int
	VariableCount int
	// TodoCount counts TODO/FIXME/HACK/XXX markers found in comments.
	TodoCount int
	// Cyclomatic is the sum of every function's cyclomatic complexity.
	Cyclomatic int
	// MaxComplexity is the highest single-function cyclomatic complexity.
	MaxComplexity int
	// Halstead holds the file-level Halstead measures.
	Halstead Halstead
	// Maintainability is a 0-100 Maintainability Index for the file.
	Maintainability float64
	// Imports holds the raw import specifiers found in the file, used to build
	// the dependency graph.
	Imports   []string `json:",omitempty"`
	Functions []FunctionInfo
}

// FunctionCount returns the number of discovered functions in this file.
func (m *FileMetrics) FunctionCount() int {
	if m == nil {
		return 0
	}
	return len(m.Functions)
}

// CommentRatio returns comment lines as a fraction of code+comment lines.
func (m *FileMetrics) CommentRatio() float64 {
	if m == nil {
		return 0
	}
	denom := m.CodeLines + m.CommentLines
	if denom == 0 {
		return 0
	}
	return float64(m.CommentLines) / float64(denom)
}

// TreeNode is a directory or file entry in the output tree.
type TreeNode struct {
	Name     string
	Path     string
	RelPath  string `json:",omitempty"` // path relative to the scan root
	IsDir    bool
	Metrics  *FileMetrics `json:",omitempty"`
	Children []*TreeNode  `json:",omitempty"`
	Warning  string       `json:",omitempty"`

	// Churn is the number of git commits that touched this file.
	Churn int `json:",omitempty"`
	// Hotspot is the refactoring-priority score: complexity * churn.
	Hotspot float64 `json:",omitempty"`
}

// Config controls traversal and filtering behavior.
type Config struct {
	ExcludedDirs    map[string]struct{}
	ExcludePatterns []string
	SupportedOnly   bool
	// AnalyzerWorkers controls per-directory file analysis workers.
	// 0 uses automatic sizing, 1 forces sequential file analysis.
	AnalyzerWorkers int
	// GitChurn enables per-file commit-frequency and hotspot scoring.
	GitChurn bool
}
