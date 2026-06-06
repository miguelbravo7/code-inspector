package inspector

import "strings"

type jsEstimateState struct {
	inBlockComment bool
	inSingle       bool
	inDouble       bool
	inBacktick     bool
}

func estimateJSFunctionLineCount(lines []string, startLine int) int {
	if startLine <= 0 || startLine > len(lines) {
		return 1
	}

	startIdx := startLine - 1
	state := &jsEstimateState{}
	bodyDepth := 0
	sawBody := false
	sawArrow := false
	parenDepth := 0
	bracketDepth := 0

	for i := startIdx; i < len(lines); i++ {
		line := lines[i]

		for j := 0; j < len(line); j++ {
			ch := line[j]
			next := byte(0)
			if j+1 < len(line) {
				next = line[j+1]
			}

			if state.inBlockComment {
				if ch == '*' && next == '/' {
					state.inBlockComment = false
					j++
				}
				continue
			}

			if !state.inSingle && !state.inDouble && !state.inBacktick {
				if ch == '/' && next == '*' {
					state.inBlockComment = true
					j++
					continue
				}
				if ch == '/' && next == '/' {
					break
				}
			}

			if ch == '\\' {
				j++
				continue
			}

			if ch == '\'' && !state.inDouble && !state.inBacktick {
				state.inSingle = !state.inSingle
				continue
			}
			if ch == '"' && !state.inSingle && !state.inBacktick {
				state.inDouble = !state.inDouble
				continue
			}
			if ch == '`' && !state.inSingle && !state.inDouble {
				state.inBacktick = !state.inBacktick
				continue
			}
			if state.inSingle || state.inDouble || state.inBacktick {
				continue
			}

			if ch == '=' && next == '>' {
				sawArrow = true
				j++
				continue
			}

			if sawBody {
				switch ch {
				case '{':
					bodyDepth++
				case '}':
					bodyDepth--
					if bodyDepth <= 0 {
						return i - startIdx + 1
					}
				}
				continue
			}

			switch ch {
			case '{':
				sawBody = true
				bodyDepth = 1
			case '(':
				parenDepth++
			case ')':
				if parenDepth > 0 {
					parenDepth--
				}
			case '[':
				bracketDepth++
			case ']':
				if bracketDepth > 0 {
					bracketDepth--
				}
			case ';':
				if parenDepth == 0 && bracketDepth == 0 {
					return i - startIdx + 1
				}
			case ',':
				if sawArrow && parenDepth == 0 && bracketDepth == 0 {
					return i - startIdx + 1
				}
			}
		}

		if sawBody && bodyDepth <= 0 {
			return i - startIdx + 1
		}

		if sawArrow && !sawBody {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" &&
				!strings.HasSuffix(trimmed, "=>") &&
				!strings.HasSuffix(trimmed, ",") &&
				!strings.HasSuffix(trimmed, "(") &&
				!strings.HasSuffix(trimmed, "[") {
				return i - startIdx + 1
			}
		}
	}

	return 1
}
