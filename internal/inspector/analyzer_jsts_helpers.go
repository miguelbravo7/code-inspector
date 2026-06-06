package inspector

import (
	"regexp"
	"strings"
)

func countRegexMatches(re *regexp.Regexp, line string) int {
	return len(re.FindAllStringIndex(line, -1))
}

func stripTrailingSemicolon(input string) string {
	return strings.TrimSuffix(strings.TrimSpace(input), ";")
}

func parseJSDeclaratorNames(declarator string) []string {
	left := strings.TrimSpace(declarator)
	if left == "" {
		return nil
	}

	if eq := strings.Index(left, "="); eq >= 0 {
		left = strings.TrimSpace(left[:eq])
	}
	left = strings.TrimPrefix(left, "...")

	if strings.HasPrefix(left, "{") || strings.HasPrefix(left, "[") {
		matches := jsIdentifierRe.FindAllString(left, -1)
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			if isJSReservedWord(match) {
				continue
			}
			names = append(names, match)
		}
		return uniqueStrings(names)
	}

	if colon := strings.Index(left, ":"); colon >= 0 {
		left = strings.TrimSpace(left[:colon])
	}

	name := jsIdentifierRe.FindString(left)
	if name == "" || isJSReservedWord(name) {
		return nil
	}
	return []string{name}
}

func isFunctionInitializer(declarator string) bool {
	eq := strings.Index(declarator, "=")
	if eq < 0 {
		return false
	}
	right := strings.TrimSpace(declarator[eq+1:])
	return strings.Contains(right, "=>") || strings.HasPrefix(right, "function") || strings.HasPrefix(right, "async function")
}

func inferJSFunctionSignature(declarator string) string {
	eq := strings.Index(declarator, "=")
	if eq < 0 {
		return ""
	}
	right := strings.TrimSpace(declarator[eq+1:])
	if strings.Contains(right, "=>") {
		return "(arrow)"
	}
	if strings.HasPrefix(right, "async function") {
		return "(async function expr)"
	}
	if strings.HasPrefix(right, "function") {
		return "(function expr)"
	}
	return ""
}

func inferJSDeclarationSignature(line string) string {
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

func splitTopLevelComma(input string) []string {
	parts := make([]string, 0)
	if strings.TrimSpace(input) == "" {
		return parts
	}

	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if ch == '\\' {
			i++
			continue
		}

		if !inDouble && !inBacktick && ch == '\'' {
			inSingle = !inSingle
			continue
		}
		if !inSingle && !inBacktick && ch == '"' {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble && ch == '`' {
			inBacktick = !inBacktick
			continue
		}
		if inSingle || inDouble || inBacktick {
			continue
		}

		switch ch {
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
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:i]))
				start = i + 1
			}
		}
	}

	if start < len(input) {
		parts = append(parts, strings.TrimSpace(input[start:]))
	}
	return parts
}

func stripJSComments(line string, inBlockComment *bool) string {
	if line == "" {
		return ""
	}

	var out strings.Builder
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		next := byte(0)
		if i+1 < len(line) {
			next = line[i+1]
		}

		if *inBlockComment {
			if ch == '*' && next == '/' {
				*inBlockComment = false
				i++
			}
			continue
		}

		if !inSingle && !inDouble && !inBacktick {
			if ch == '/' && next == '*' {
				*inBlockComment = true
				i++
				continue
			}
			if ch == '/' && next == '/' {
				break
			}
		}

		if ch == '\\' {
			out.WriteByte(ch)
			if i+1 < len(line) {
				i++
				out.WriteByte(line[i])
			}
			continue
		}

		if ch == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
		} else if ch == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
		}

		out.WriteByte(ch)
	}

	return out.String()
}

func stripJSStringLiterals(line string) string {
	if line == "" {
		return ""
	}

	var out strings.Builder
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if ch == '\\' {
			if inSingle || inDouble || inBacktick {
				out.WriteByte(' ')
				if i+1 < len(line) {
					i++
					out.WriteByte(' ')
				}
				continue
			}
			out.WriteByte(ch)
			if i+1 < len(line) {
				i++
				out.WriteByte(line[i])
			}
			continue
		}

		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			out.WriteByte(' ')
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
			}
			out.WriteByte(' ')
			continue
		}
		if inBacktick {
			if ch == '`' {
				inBacktick = false
			}
			out.WriteByte(' ')
			continue
		}

		if ch == '\'' {
			inSingle = true
			out.WriteByte(' ')
			continue
		}
		if ch == '"' {
			inDouble = true
			out.WriteByte(' ')
			continue
		}
		if ch == '`' {
			inBacktick = true
			out.WriteByte(' ')
			continue
		}

		out.WriteByte(ch)
	}

	return out.String()
}

func countBraceDelta(line string) int {
	delta := 0
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if ch == '\\' {
			i++
			continue
		}
		if ch == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
			continue
		}
		if ch == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
			continue
		}
		if inSingle || inDouble || inBacktick {
			continue
		}

		if ch == '{' {
			delta++
		} else if ch == '}' {
			delta--
		}
	}
	return delta
}

func isJSControlKeyword(name string) bool {
	_, found := jsControlKeywords[name]
	return found
}

func isJSReservedWord(word string) bool {
	_, found := jsReservedWords[word]
	return found
}

func uniqueStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
