package inspector

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	pyImportRe   = regexp.MustCompile(`^\s*(?:from\s+[A-Za-z0-9_\.]+\s+import\s+.+|import\s+.+)$`)
	pyFunctionRe = regexp.MustCompile(`^\s*(?:async\s+def|def)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pyAssignRe   = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*(?::[^=]+)?=`)
)

type pythonStripState struct {
	tripleQuote string
}

func analyzePythonSource(source []byte) (*FileMetrics, error) {
	metrics := &FileMetrics{Language: "python"}
	lines := strings.Split(string(source), "\n")
	state := &pythonStripState{}

	seenFunctions := make(map[string]struct{})
	addFunction := func(fn FunctionInfo) {
		if fn.LineCount <= 0 {
			fn.LineCount = estimatePythonFunctionLineCount(lines, fn.Line)
		}
		key := strconv.Itoa(fn.Line) + ":" + fn.Name + ":" + fn.Signature
		if _, exists := seenFunctions[key]; exists {
			return
		}
		seenFunctions[key] = struct{}{}
		metrics.Functions = append(metrics.Functions, fn)
	}

	for idx, rawLine := range lines {
		lineNumber := idx + 1
		cleaned := stripPythonLine(rawLine, state)
		trimmed := strings.TrimSpace(cleaned)
		if trimmed == "" {
			continue
		}

		if pyImportRe.MatchString(trimmed) {
			metrics.ImportCount++
		}

		if match := pyFunctionRe.FindStringSubmatch(trimmed); len(match) == 2 {
			addFunction(FunctionInfo{
				Name:      match[1],
				Signature: inferPythonFunctionSignature(trimmed),
				Line:      lineNumber,
			})
		}

		if match := pyAssignRe.FindStringSubmatch(trimmed); len(match) == 2 {
			for _, name := range strings.Split(match[1], ",") {
				candidate := strings.TrimSpace(name)
				if candidate != "" && candidate != "_" {
					metrics.VariableCount++
				}
			}
		}
	}

	return metrics, nil
}

func estimatePythonFunctionLineCount(lines []string, startLine int) int {
	if startLine <= 0 || startLine > len(lines) {
		return 1
	}

	startIdx := startLine - 1
	baseIndent := leadingIndentWidth(lines[startIdx])
	headerEnd := -1
	parenDepth := 0

	for i := startIdx; i < len(lines); i++ {
		colonIdx, updatedParenDepth := findPythonHeaderColon(lines[i], parenDepth)
		parenDepth = updatedParenDepth
		if colonIdx >= 0 && parenDepth == 0 {
			headerEnd = i
			if strings.TrimSpace(lines[i][colonIdx+1:]) != "" {
				return i - startIdx + 1
			}
			break
		}
	}

	if headerEnd < 0 {
		return 1
	}

	lastIncluded := headerEnd
	pendingBlankLines := 0
	bodyStarted := false
	for i := headerEnd + 1; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			if bodyStarted {
				pendingBlankLines++
			}
			continue
		}

		indent := leadingIndentWidth(raw)
		if indent <= baseIndent {
			break
		}
		if pendingBlankLines > 0 {
			lastIncluded = i
			pendingBlankLines = 0
		} else {
			lastIncluded = i
		}
		bodyStarted = true
	}

	if lastIncluded < startIdx {
		return 1
	}
	return lastIncluded - startIdx + 1
}

func findPythonHeaderColon(line string, initialParenDepth int) (int, int) {
	parenDepth := initialParenDepth
	inSingle := false
	inDouble := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		next := byte(0)
		if i+1 < len(line) {
			next = line[i+1]
		}

		if !inSingle && !inDouble {
			if ch == '#' {
				break
			}
			if ch == '/' && next == '/' {
				break
			}
		}

		if ch == '\\' {
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}

		switch ch {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ':':
			if parenDepth == 0 {
				return i, parenDepth
			}
		}
	}

	return -1, parenDepth
}

func leadingIndentWidth(line string) int {
	width := 0
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			width++
			continue
		}
		if line[i] == '\t' {
			width += 4
			continue
		}
		break
	}
	return width
}

func inferPythonFunctionSignature(line string) string {
	start := strings.Index(line, "(")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(line, ")")
	if end <= start {
		return ""
	}
	return line[start : end+1]
}

func stripPythonLine(line string, state *pythonStripState) string {
	if line == "" {
		return ""
	}

	var out strings.Builder
	inSingle := false
	inDouble := false
	i := 0

	for i < len(line) {
		if state.tripleQuote != "" {
			closing := strings.Index(line[i:], state.tripleQuote)
			if closing < 0 {
				return out.String()
			}
			i += closing + 3
			state.tripleQuote = ""
			continue
		}

		if !inSingle && !inDouble && i+2 < len(line) {
			segment := line[i : i+3]
			if segment == "'''" || segment == "\"\"\"" {
				closing := strings.Index(line[i+3:], segment)
				if closing < 0 {
					state.tripleQuote = segment
					return out.String()
				}
				i += 3 + closing + 3
				continue
			}
		}

		ch := line[i]
		if ch == '#' && !inSingle && !inDouble {
			break
		}
		if ch == '\\' {
			out.WriteByte(ch)
			if i+1 < len(line) {
				i++
				out.WriteByte(line[i])
			}
			i++
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			out.WriteByte(ch)
			i++
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			out.WriteByte(ch)
			i++
			continue
		}

		out.WriteByte(ch)
		i++
	}

	return out.String()
}
