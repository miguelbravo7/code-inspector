package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"code-inspector/internal/inspector"
	"code-inspector/internal/render"
)

func main() {
	var excludeCSV string
	var noDefaultExcludes bool
	var supportedOnly bool

	flag.StringVar(&excludeCSV, "exclude", "", "Comma-separated directory names to exclude")
	flag.BoolVar(&noDefaultExcludes, "no-default-excludes", false, "Disable built-in excluded directory names")
	flag.BoolVar(&supportedOnly, "supported-only", false, "Show only supported source files")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <directory>\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Recursively prints a file tree with per-file code metrics.")
		fmt.Fprintln(os.Stderr, "Supported file types: .js, .jsx, .ts, .tsx, .py, .go")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	targetPath := flag.Arg(0)

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
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := render.PrintTree(tree, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error rendering tree: %v\n", err)
		os.Exit(1)
	}
}
