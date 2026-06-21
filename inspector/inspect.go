package inspector

// Options configures a full inspection run. The zero value is valid and
// produces sensible defaults: built-in directory excludes applied, automatic
// worker sizing, top-10 ranked lists, and git churn, duplicate detection and
// the dependency graph all enabled.
type Options struct {
	// Excludes are file or directory names/glob patterns to skip.
	Excludes []string
	// NoDefaultExcludes disables the built-in excludes (.git, node_modules, ...).
	NoDefaultExcludes bool
	// SupportedOnly prunes unsupported files from the tree.
	SupportedOnly bool
	// Workers controls per-directory analysis workers: 0 = auto, 1 = sequential.
	Workers int
	// TopN caps each ranked list (0 = 10).
	TopN int
	// DupMinTokens is the clone window size (0 = DefaultDuplicationMinTokens).
	DupMinTokens int
	// NoGit disables git churn and hotspot scoring.
	NoGit bool
	// NoDup disables duplicate-code detection.
	NoDup bool
	// NoDeps disables the import dependency graph.
	NoDeps bool
}

// Report is the aggregate result of a full inspection.
type Report struct {
	Root         *TreeNode
	Summary      Summary
	Duplication  *DuplicationReport `json:",omitempty"`
	Dependencies *DependencyReport  `json:",omitempty"`
}

// Inspect analyzes the tree rooted at path and returns the metrics tree plus the
// ranked summary, duplicate-code report and dependency graph. It is the primary
// entry point for embedding the inspector in another program.
//
// Building any program that calls Inspect requires cgo (CGO_ENABLED=1 and a C
// compiler) because the tree-sitter grammars are C.
func Inspect(path string, opts Options) (*Report, error) {
	cfg := Config{
		ExcludedDirs:    BuildExcludeSet(!opts.NoDefaultExcludes, opts.Excludes),
		ExcludePatterns: append([]string(nil), opts.Excludes...),
		SupportedOnly:   opts.SupportedOnly,
		AnalyzerWorkers: opts.Workers,
		GitChurn:        !opts.NoGit,
	}

	tree, err := BuildTree(path, cfg)
	if err != nil {
		return nil, err
	}

	gitChurn := false
	if !opts.NoGit {
		gitChurn = ComputeChurn(tree, path)
	}

	report := &Report{
		Root:    tree,
		Summary: BuildSummary(tree, opts.TopN, gitChurn),
	}

	if !opts.NoDup {
		dup := DetectDuplication(tree, opts.DupMinTokens, opts.TopN)
		report.Duplication = &dup
	}
	if !opts.NoDeps {
		deps := BuildDependencyGraph(tree, path, opts.TopN)
		report.Dependencies = &deps
	}

	return report, nil
}
