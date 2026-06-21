// Package inspector analyzes source trees and extracts code-quality metrics
// aimed at finding where to improve a codebase.
//
// For each supported file it reports a code/comment/blank line breakdown, import
// and variable counts, and per-function cyclomatic complexity, cognitive
// complexity, nesting depth, parameter count and a Maintainability Index. It also
// computes file-level Halstead measures, git churn and a hotspot score
// (complexity x churn), detects duplicate code across files, and builds an import
// dependency graph with fan-in/fan-out and cycle detection.
//
// Go is parsed with the standard library go/ast. Python, JavaScript, JSX,
// TypeScript and TSX are parsed with tree-sitter. Because the tree-sitter
// grammars are C, building any program that imports this package requires cgo:
// set CGO_ENABLED=1 and have a C compiler (gcc or clang) on PATH.
//
// The simplest entry point is Inspect, which runs everything and returns a
// single Report:
//
//	report, err := inspector.Inspect("./path/to/project", inspector.Options{})
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, h := range report.Summary.TopHotspots {
//		fmt.Printf("%s\thot=%.0f cyc=%d churn=%d\n", h.Path, h.Hotspot, h.Cyclomatic, h.Churn)
//	}
//
// The individual stages (BuildTree, ComputeChurn, BuildSummary,
// DetectDuplication, BuildDependencyGraph) are also exported for callers that
// need finer control.
package inspector
