package render

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"code-inspector/internal/inspector"
)

// PrintTree writes the analyzed directory tree to the provided writer.
func PrintTree(root *inspector.TreeNode, writer io.Writer) error {
	if root == nil {
		return fmt.Errorf("tree is nil")
	}

	if _, err := fmt.Fprintln(writer, root.Name+"/"); err != nil {
		return err
	}

	for idx, child := range root.Children {
		isLast := idx == len(root.Children)-1
		if err := renderNode(writer, child, "", isLast); err != nil {
			return err
		}
	}
	return nil
}

func renderNode(writer io.Writer, node *inspector.TreeNode, prefix string, isLast bool) error {
	connector := "├── "
	nextPrefix := prefix + "│   "
	if isLast {
		connector = "└── "
		nextPrefix = prefix + "    "
	}

	if _, err := fmt.Fprintf(writer, "%s%s%s\n", prefix, connector, formatNode(node)); err != nil {
		return err
	}

	if node.IsDir {
		for idx, child := range node.Children {
			childLast := idx == len(node.Children)-1
			if err := renderNode(writer, child, nextPrefix, childLast); err != nil {
				return err
			}
		}
		return nil
	}

	if node.Metrics == nil || len(node.Metrics.Functions) == 0 {
		return nil
	}

	layout := buildFunctionLayout(node.Metrics.Functions)

	for idx, fn := range node.Metrics.Functions {
		fnConnector := "├── "
		if idx == len(node.Metrics.Functions)-1 {
			fnConnector = "└── "
		}
		if _, err := fmt.Fprintf(writer, "%s%s%s\n", nextPrefix, fnConnector, formatFunction(fn, layout.leftWidth)); err != nil {
			return err
		}
	}
	return nil
}

func formatNode(node *inspector.TreeNode) string {
	if node == nil {
		return "<nil>"
	}
	label := node.Name
	if node.IsDir {
		label += "/"
	}

	if node.Metrics != nil {
		metrics := node.Metrics
		label += fmt.Sprintf(" [lines:%d imports:%d vars:%d funcs:%d]", metrics.LineCount, metrics.ImportCount, metrics.VariableCount, len(metrics.Functions))
	}

	if node.Warning != "" {
		label += " [warning: " + summarizeWarning(node.Warning) + "]"
	}
	return label
}

type functionFormatLayout struct {
	leftWidth int
}

func buildFunctionLayout(functions []inspector.FunctionInfo) functionFormatLayout {
	maxWidth := 0
	for _, fn := range functions {
		width := utf8.RuneCountInString(formatFunctionLeft(fn))
		if width > maxWidth {
			maxWidth = width
		}
	}
	return functionFormatLayout{leftWidth: maxWidth}
}

func formatFunction(fn inspector.FunctionInfo, leftWidth int) string {
	left := formatFunctionLeft(fn)
	rightParts := make([]string, 0, 2)
	if fn.Line > 0 {
		rightParts = append(rightParts, fmt.Sprintf("line %d", fn.Line))
	}
	if fn.LineCount > 0 {
		rightParts = append(rightParts, fmt.Sprintf("lines %d", fn.LineCount))
	}

	if len(rightParts) == 0 {
		return left
	}

	left = padRightRunes(left, leftWidth)
	return left + " | " + strings.Join(rightParts, " | ")
}

func formatFunctionLeft(fn inspector.FunctionInfo) string {
	if fn.Signature == "" {
		return "fn: " + fn.Name
	}
	return "fn: " + fn.Name + " | " + fn.Signature
}

func padRightRunes(input string, width int) string {
	current := utf8.RuneCountInString(input)
	if current >= width {
		return input
	}
	return input + strings.Repeat(" ", width-current)
}

func summarizeWarning(message string) string {
	const maxLen = 80
	if len(message) <= maxLen {
		return message
	}
	return message[:maxLen-3] + "..."
}
