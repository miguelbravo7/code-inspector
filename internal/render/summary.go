package render

import (
	"fmt"
	"io"
	"strings"

	"code-inspector/internal/inspector"
)

// PrintSummary writes a ranked, aggregate view designed to surface the highest
// value places to improve.
func PrintSummary(summary inspector.Summary, writer io.Writer) error {
	w := func(format string, args ...interface{}) error {
		_, err := fmt.Fprintf(writer, format, args...)
		return err
	}

	if err := w("\nSummary\n"); err != nil {
		return err
	}
	if err := w("  files: %d (%d analyzed)  lines: %d (code %d / comment %d / blank %d)\n",
		summary.Files, summary.SupportedFiles, summary.TotalLines,
		summary.TotalCode, summary.TotalComment, summary.TotalBlank); err != nil {
		return err
	}
	if err := w("  functions: %d  todo markers: %d\n", summary.TotalFunctions, summary.TotalTodos); err != nil {
		return err
	}

	heading := "Top hotspots (complexity x git churn)"
	if !summary.GitChurn {
		heading = "Top files by complexity (git churn unavailable)"
	}
	if len(summary.TopHotspots) > 0 {
		if err := w("\n  %s:\n", heading); err != nil {
			return err
		}
		for _, f := range summary.TopHotspots {
			if summary.GitChurn {
				if err := w("    %-40s hot %-7.0f cyc %-5d churn %-4d\n", truncatePath(f.Path, 40), f.Hotspot, f.Cyclomatic, f.Churn); err != nil {
					return err
				}
			} else {
				if err := w("    %-40s cyc %-5d lines %d\n", truncatePath(f.Path, 40), f.Cyclomatic, f.LineCount); err != nil {
					return err
				}
			}
		}
	}

	if len(summary.MostComplex) > 0 {
		if err := w("\n  Most complex functions:\n"); err != nil {
			return err
		}
		for _, fn := range summary.MostComplex {
			if err := w("    %-28s cyc %-4d cog %-4d  %s:%d\n",
				truncatePath(fn.Name, 28), fn.Cyclomatic, fn.Cognitive, fn.Path, fn.Line); err != nil {
				return err
			}
		}
	}

	if len(summary.LowestMaintainable) > 0 {
		if err := w("\n  Lowest maintainability (0-100, higher is better):\n"); err != nil {
			return err
		}
		for _, f := range summary.LowestMaintainable {
			if err := w("    %-40s mi %-6.1f cyc %d\n", truncatePath(f.Path, 40), f.Maintainability, f.Cyclomatic); err != nil {
				return err
			}
		}
	}

	return nil
}

// PrintDuplication writes the duplicate-code report.
func PrintDuplication(report inspector.DuplicationReport, writer io.Writer) error {
	w := func(format string, args ...interface{}) error {
		_, err := fmt.Fprintf(writer, format, args...)
		return err
	}

	if report.TotalBlocks == 0 {
		return w("\n  Duplication: none found (>= %d tokens)\n", report.MinTokens)
	}

	if err := w("\n  Duplication: %d clone blocks, ~%d duplicated lines (>= %d tokens):\n",
		report.TotalBlocks, report.DuplicatedLines, report.MinTokens); err != nil {
		return err
	}
	for _, b := range report.Blocks {
		if err := w("    %d tokens / %d lines:\n", b.Tokens, b.Lines); err != nil {
			return err
		}
		if err := w("      %s:%d-%d\n", b.FirstPath, b.FirstStart, b.FirstEnd); err != nil {
			return err
		}
		if err := w("      %s:%d-%d\n", b.OtherPath, b.OtherStart, b.OtherEnd); err != nil {
			return err
		}
	}
	return nil
}

// PrintDependency writes the import dependency-graph report.
func PrintDependency(report inspector.DependencyReport, writer io.Writer) error {
	w := func(format string, args ...interface{}) error {
		_, err := fmt.Fprintf(writer, format, args...)
		return err
	}

	if report.Nodes == 0 {
		return nil
	}

	if err := w("\n  Dependency graph: %d modules, %d internal edges, %d external imports\n",
		report.Nodes, report.Edges, report.ExternalImports); err != nil {
		return err
	}

	if len(report.MostDependedOn) > 0 {
		if err := w("\n  Most depended-on (high fan-in = wide blast radius):\n"); err != nil {
			return err
		}
		for _, s := range report.MostDependedOn {
			if err := w("    %-44s fan-in %-4d fan-out %d\n", truncatePath(s.Node, 44), s.FanIn, s.FanOut); err != nil {
				return err
			}
		}
	}

	if len(report.MostDependencies) > 0 {
		if err := w("\n  Most dependencies (high fan-out = fragile):\n"); err != nil {
			return err
		}
		for _, s := range report.MostDependencies {
			if err := w("    %-44s fan-out %-4d fan-in %d\n", truncatePath(s.Node, 44), s.FanOut, s.FanIn); err != nil {
				return err
			}
		}
	}

	if len(report.Cycles) > 0 {
		if err := w("\n  Dependency cycles (%d):\n", len(report.Cycles)); err != nil {
			return err
		}
		for _, cycle := range report.Cycles {
			if err := w("    %s\n", strings.Join(cycle, " -> ")); err != nil {
				return err
			}
		}
	}
	return nil
}

func truncatePath(path string, width int) string {
	runes := []rune(path)
	if len(runes) <= width {
		return path
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return "..." + string(runes[len(runes)-(width-3):])
}
