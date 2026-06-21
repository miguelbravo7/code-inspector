package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/miguelbravo7/code-inspector/inspector"
	"github.com/miguelbravo7/code-inspector/internal/render"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	var excludes excludePatternFlag
	var noDefaultExcludes bool
	var supportedOnly bool
	var outputFormat string
	var analyzerWorkers int
	var noSummary bool
	var noGit bool
	var topN int
	var noDup bool
	var dupMinTokens int
	var noDeps bool

	flags := flag.NewFlagSet("code-inspector", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Var(&excludes, "exclude", "Exclude file or directory names/patterns (repeatable, e.g. -exclude=*_test.go -exclude=sqlc)")
	flags.BoolVar(&noDefaultExcludes, "no-default-excludes", false, "Disable built-in excluded directory names")
	flags.BoolVar(&supportedOnly, "supported-only", false, "Show only supported source files")
	flags.StringVar(&outputFormat, "format", "tree", "Output format: tree or json")
	flags.IntVar(&analyzerWorkers, "workers", 1, "File analysis workers per directory (default 1 = sequential, 0 = auto)")
	flags.BoolVar(&noSummary, "no-summary", false, "Skip the ranked summary of hotspots and complex functions")
	flags.BoolVar(&noGit, "no-git", false, "Disable git churn and hotspot scoring")
	flags.IntVar(&topN, "top", 10, "Number of entries per ranked summary list")
	flags.BoolVar(&noDup, "no-dup", false, "Disable duplicate-code detection")
	flags.IntVar(&dupMinTokens, "dup-min-tokens", inspector.DefaultDuplicationMinTokens, "Minimum token run length for duplicate-code detection")
	flags.BoolVar(&noDeps, "no-deps", false, "Disable the import dependency graph (fan-in/out + cycles)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: code-inspector [flags] <directory>")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Recursively prints a file tree with per-file code metrics.")
		fmt.Fprintln(stderr, "Supported file types: .js, .jsx, .ts, .tsx, .py, .go")
		fmt.Fprintln(stderr)
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}

	targetPath := flags.Arg(0)
	if analyzerWorkers < 0 {
		fmt.Fprintf(stderr, "error: workers must be >= 0, got %d\n", analyzerWorkers)
		return 2
	}

	excludeValues := []string(excludes)

	cfg := inspector.Config{
		ExcludedDirs:    inspector.BuildExcludeSet(!noDefaultExcludes, excludeValues),
		ExcludePatterns: append([]string(nil), excludeValues...),
		SupportedOnly:   supportedOnly,
		AnalyzerWorkers: analyzerWorkers,
		GitChurn:        !noGit,
	}

	tree, err := inspector.BuildTree(targetPath, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	gitChurn := false
	if !noGit {
		gitChurn = inspector.ComputeChurn(tree, targetPath)
	}

	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "tree":
		if err := render.PrintTree(tree, stdout); err != nil {
			fmt.Fprintf(stderr, "error rendering tree: %v\n", err)
			return 1
		}
		if !noSummary {
			summary := inspector.BuildSummary(tree, topN, gitChurn)
			if err := render.PrintSummary(summary, stdout); err != nil {
				fmt.Fprintf(stderr, "error rendering summary: %v\n", err)
				return 1
			}
			if !noDup {
				dup := inspector.DetectDuplication(tree, dupMinTokens, topN)
				if err := render.PrintDuplication(dup, stdout); err != nil {
					fmt.Fprintf(stderr, "error rendering duplication: %v\n", err)
					return 1
				}
			}
			if !noDeps {
				deps := inspector.BuildDependencyGraph(tree, targetPath, topN)
				if err := render.PrintDependency(deps, stdout); err != nil {
					fmt.Fprintf(stderr, "error rendering dependencies: %v\n", err)
					return 1
				}
			}
		}
	case "json":
		report := struct {
			Root         *inspector.TreeNode          `json:"root"`
			Summary      inspector.Summary            `json:"summary"`
			Duplication  *inspector.DuplicationReport `json:"duplication,omitempty"`
			Dependencies *inspector.DependencyReport  `json:"dependencies,omitempty"`
		}{
			Root:    tree,
			Summary: inspector.BuildSummary(tree, topN, gitChurn),
		}
		if !noDup {
			dup := inspector.DetectDuplication(tree, dupMinTokens, topN)
			report.Duplication = &dup
		}
		if !noDeps {
			deps := inspector.BuildDependencyGraph(tree, targetPath, topN)
			report.Dependencies = &deps
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "error rendering json: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "error: unsupported format %q (expected tree or json)\n", outputFormat)
		return 2
	}

	return 0
}

type excludePatternFlag []string

func (f *excludePatternFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *excludePatternFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		*f = append(*f, trimmed)
	}
	return nil
}
