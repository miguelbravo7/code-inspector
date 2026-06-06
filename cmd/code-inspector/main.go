package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"code-inspector/internal/inspector"
	"code-inspector/internal/render"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	var excludeCSV string
	var noDefaultExcludes bool
	var supportedOnly bool
	var outputFormat string

	flags := flag.NewFlagSet("code-inspector", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&excludeCSV, "exclude", "", "Comma-separated directory names to exclude")
	flags.BoolVar(&noDefaultExcludes, "no-default-excludes", false, "Disable built-in excluded directory names")
	flags.BoolVar(&supportedOnly, "supported-only", false, "Show only supported source files")
	flags.StringVar(&outputFormat, "format", "tree", "Output format: tree or json")
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

	var extras []string
	if trimmed := strings.TrimSpace(excludeCSV); trimmed != "" {
		extras = strings.Split(trimmed, ",")
	}

	cfg := inspector.Config{
		ExcludedDirs:  inspector.BuildExcludeSet(!noDefaultExcludes, extras),
		SupportedOnly: supportedOnly,
	}

	tree, err := inspector.BuildTree(targetPath, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "", "tree":
		if err := render.PrintTree(tree, stdout); err != nil {
			fmt.Fprintf(stderr, "error rendering tree: %v\n", err)
			return 1
		}
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(tree); err != nil {
			fmt.Fprintf(stderr, "error rendering json: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "error: unsupported format %q (expected tree or json)\n", outputFormat)
		return 2
	}

	return 0
}
