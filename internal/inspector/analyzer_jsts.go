package inspector

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	jsVarDeclRe            = regexp.MustCompile(`^(?:export\s+)?(?:declare\s+)?(?:const|let|var)\s+(.+)$`)
	jsFunctionDeclRe       = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function(?:\s*\*)?\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	jsDefaultAnonymousFnRe = regexp.MustCompile(`^(?:export\s+)?default\s+(?:async\s+)?function\s*\(`)
	jsClassDeclRe          = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\b`)
	jsMethodDeclRe         = regexp.MustCompile(`^(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|abstract\s+|async\s+|override\s+)*(?:get\s+|set\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^;{}]*\)\s*\{`)
	jsDynamicImportRe      = regexp.MustCompile(`\bimport\s*\(`)
	jsRequireRe            = regexp.MustCompile(`\brequire\s*\(`)
	jsIdentifierRe         = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`)
)

var jsControlKeywords = map[string]struct{}{
	"if": {}, "for": {}, "while": {}, "switch": {}, "catch": {},
	"do": {}, "try": {}, "else": {}, "return": {}, "throw": {},
}

var jsReservedWords = map[string]struct{}{
	"const": {}, "let": {}, "var": {}, "function": {}, "class": {},
	"import": {}, "export": {}, "default": {}, "extends": {}, "new": {},
	"async": {}, "await": {}, "type": {}, "interface": {}, "implements": {},
	"public": {}, "private": {}, "protected": {}, "readonly": {}, "static": {},
}

func analyzeJavaScriptLikeSource(source []byte, language string) (*FileMetrics, error) {
	metrics := &FileMetrics{Language: language}
	lines := strings.Split(string(source), "\n")

	inBlockComment := false
	pendingImport := false
	pendingClassHeader := false
	inClass := false
	classBraceDepth := 0

	seenFunctions := make(map[string]struct{})
	addFunction := func(fn FunctionInfo) {
		if fn.LineCount <= 0 {
			fn.LineCount = estimateJSFunctionLineCount(lines, fn.Line)
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
		line := stripJSComments(rawLine, &inBlockComment)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if pendingImport {
			if strings.Contains(trimmed, " from ") || strings.Contains(trimmed, ";") || strings.HasSuffix(trimmed, "\"") || strings.HasSuffix(trimmed, "'") {
				metrics.ImportCount++
				pendingImport = false
			}
		}

		if strings.HasPrefix(trimmed, "import ") && !strings.Contains(trimmed, "import(") {
			if strings.Contains(trimmed, " from ") || strings.Contains(trimmed, ";") || strings.HasSuffix(trimmed, "\"") || strings.HasSuffix(trimmed, "'") {
				metrics.ImportCount++
			} else {
				pendingImport = true
			}
		}
		metrics.ImportCount += countRegexMatches(jsDynamicImportRe, trimmed)
		metrics.ImportCount += countRegexMatches(jsRequireRe, trimmed)

		if match := jsVarDeclRe.FindStringSubmatch(trimmed); len(match) == 2 {
			declarators := splitTopLevelComma(stripTrailingSemicolon(match[1]))
			for _, declarator := range declarators {
				names := parseJSDeclaratorNames(declarator)
				metrics.VariableCount += len(names)
				if len(names) == 0 {
					continue
				}
				if isFunctionInitializer(declarator) {
					signature := inferJSFunctionSignature(declarator)
					for _, name := range names {
						addFunction(FunctionInfo{Name: name, Signature: signature, Line: lineNumber})
					}
				}
			}
		}

		if match := jsFunctionDeclRe.FindStringSubmatch(trimmed); len(match) == 2 {
			addFunction(FunctionInfo{
				Name:      match[1],
				Signature: inferJSDeclarationSignature(trimmed),
				Line:      lineNumber,
			})
		} else if jsDefaultAnonymousFnRe.MatchString(trimmed) {
			addFunction(FunctionInfo{Name: "default", Signature: "(anonymous)", Line: lineNumber})
		}

		if !inClass && jsClassDeclRe.MatchString(trimmed) {
			pendingClassHeader = true
		}
		if pendingClassHeader && strings.Contains(trimmed, "{") {
			inClass = true
			pendingClassHeader = false
		}

		if inClass {
			if methodMatch := jsMethodDeclRe.FindStringSubmatch(trimmed); len(methodMatch) == 2 {
				methodName := methodMatch[1]
				if !isJSControlKeyword(methodName) {
					addFunction(FunctionInfo{
						Name:      methodName,
						Signature: inferJSDeclarationSignature(trimmed),
						Line:      lineNumber,
					})
				}
			}
			classBraceDepth += countBraceDelta(trimmed)
			if classBraceDepth <= 0 {
				inClass = false
				classBraceDepth = 0
			}
		}
	}

	return metrics, nil
}

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
