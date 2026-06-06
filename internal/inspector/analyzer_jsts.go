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
		importScanLine := stripJSStringLiterals(trimmed)
		metrics.ImportCount += countRegexMatches(jsDynamicImportRe, importScanLine)
		metrics.ImportCount += countRegexMatches(jsRequireRe, importScanLine)

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
