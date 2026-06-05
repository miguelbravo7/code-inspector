package render

import (
	"fmt"
	"io"
	"strings"

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
	connector := "|-- "
	nextPrefix := prefix + "|   "
	if isLast {
		connector = "`-- "
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

	for idx, fn := range node.Metrics.Functions {
		fnLast := idx == len(node.Metrics.Functions)-1
		fnConnector := "|-- "
		if fnLast {
			fnConnector = "`-- "
		}
		if _, err := fmt.Fprintf(writer, "%s%s%s\n", nextPrefix, fnConnector, formatFunction(fn)); err != nil {
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

func formatFunction(fn inspector.FunctionInfo) string {
	parts := []string{"fn: " + fn.Name}
	if fn.Signature != "" {
		parts = append(parts, fn.Signature)
	}
	if fn.Line > 0 {
		parts = append(parts, fmt.Sprintf("line %d", fn.Line))
	}
	if fn.LineCount > 0 {
		parts = append(parts, fmt.Sprintf("lines %d", fn.LineCount))
	}
	return strings.Join(parts, " | ")
}

func summarizeWarning(message string) string {
	const maxLen = 80
	if len(message) <= maxLen {
		return message
	}
	return message[:maxLen-3] + "..."
}
