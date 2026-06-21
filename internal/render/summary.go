package render

import (
	"fmt"
	"io"

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
