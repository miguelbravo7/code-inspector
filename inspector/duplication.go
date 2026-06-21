package inspector

import (
	"go/scanner"
	"go/token"
	"hash/fnv"
	"os"
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// DefaultDuplicationMinTokens is the default clone window size (in normalized
// tokens) for duplicate-code detection.
const DefaultDuplicationMinTokens = 50

// DuplicateBlock is a pair of code regions that share an identical normalized
// token sequence (variable names and literal values are normalized, so renamed
// copies still match).
type DuplicateBlock struct {
	Tokens     int
	Lines      int
	FirstPath  string
	FirstStart int
	FirstEnd   int
	OtherPath  string
	OtherStart int
	OtherEnd   int
}

// DuplicationReport summarizes clone detection across a scan.
type DuplicationReport struct {
	MinTokens       int
	Blocks          []DuplicateBlock
	TotalBlocks     int
	DuplicatedLines int
}

type dupToken struct {
	norm string
	line int
}

type dupFile struct {
	path   string
	tokens []dupToken
}

// DetectDuplication finds duplicated token sequences of at least minTokens
// across all analyzed files in the tree. topN caps the reported block list.
func DetectDuplication(root *TreeNode, minTokens, topN int) DuplicationReport {
	if minTokens <= 0 {
		minTokens = DefaultDuplicationMinTokens
	}
	if topN <= 0 {
		topN = 10
	}

	files := collectDupFiles(root, minTokens)
	report := DuplicationReport{MinTokens: minTokens}
	if len(files) == 0 {
		return report
	}

	// Index every window of minTokens tokens by its hash.
	type occurrence struct{ file, pos int }
	windows := make(map[uint64][]occurrence)
	for fi := range files {
		tokens := files[fi].tokens
		for i := 0; i+minTokens <= len(tokens); i++ {
			h := hashWindow(tokens, i, minTokens)
			windows[h] = append(windows[h], occurrence{fi, i})
		}
	}

	covered := make([][]bool, len(files))
	for fi := range files {
		covered[fi] = make([]bool, len(files[fi].tokens))
	}

	duplicatedLines := make(map[lineKey]struct{})
	var blocks []DuplicateBlock

	for fi := range files {
		tokens := files[fi].tokens
		for i := 0; i+minTokens <= len(tokens); i++ {
			if covered[fi][i] {
				continue
			}
			h := hashWindow(tokens, i, minTokens)
			bestFile, bestPos, bestLen := -1, -1, 0
			for _, occ := range windows[h] {
				if occ.file == fi && occ.pos == i {
					continue
				}
				if covered[occ.file][occ.pos] {
					continue
				}
				if !equalTokens(files[fi].tokens, i, files[occ.file].tokens, occ.pos, minTokens) {
					continue // hash collision
				}
				length := extendMatch(files[fi].tokens, i, files[occ.file].tokens, occ.pos)
				if length > bestLen {
					bestFile, bestPos, bestLen = occ.file, occ.pos, length
				}
			}

			if bestLen < minTokens {
				continue
			}

			block := DuplicateBlock{
				Tokens:     bestLen,
				FirstPath:  files[fi].path,
				FirstStart: tokens[i].line,
				FirstEnd:   tokens[i+bestLen-1].line,
				OtherPath:  files[bestFile].path,
				OtherStart: files[bestFile].tokens[bestPos].line,
				OtherEnd:   files[bestFile].tokens[bestPos+bestLen-1].line,
			}
			block.Lines = block.FirstEnd - block.FirstStart + 1
			blocks = append(blocks, block)

			markCovered(covered[fi], i, bestLen, files[fi].tokens, files[fi].path, duplicatedLines)
			markCovered(covered[bestFile], bestPos, bestLen, files[bestFile].tokens, files[bestFile].path, duplicatedLines)
			i += bestLen - 1
		}
	}

	report.TotalBlocks = len(blocks)
	report.DuplicatedLines = len(duplicatedLines)

	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Tokens != blocks[j].Tokens {
			return blocks[i].Tokens > blocks[j].Tokens
		}
		if blocks[i].FirstPath != blocks[j].FirstPath {
			return blocks[i].FirstPath < blocks[j].FirstPath
		}
		return blocks[i].FirstStart < blocks[j].FirstStart
	})
	if len(blocks) > topN {
		blocks = blocks[:topN]
	}
	report.Blocks = blocks
	return report
}

type lineKey struct {
	path string
	line int
}

func markCovered(covered []bool, start, length int, tokens []dupToken, path string, lines map[lineKey]struct{}) {
	for k := 0; k < length && start+k < len(covered); k++ {
		covered[start+k] = true
		lines[lineKey{path, tokens[start+k].line}] = struct{}{}
	}
}

func collectDupFiles(root *TreeNode, minTokens int) []dupFile {
	var files []dupFile
	var walk func(n *TreeNode)
	walk = func(n *TreeNode) {
		if !n.IsDir && n.Metrics != nil {
			source, err := os.ReadFile(n.Path)
			if err == nil {
				tokens := normalizedTokens(n.Metrics.Language, source)
				if len(tokens) >= minTokens {
					path := n.RelPath
					if path == "" {
						path = n.Name
					}
					files = append(files, dupFile{path: path, tokens: tokens})
				}
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	return files
}

func hashWindow(tokens []dupToken, start, width int) uint64 {
	h := fnv.New64a()
	for k := 0; k < width; k++ {
		h.Write([]byte(tokens[start+k].norm))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func equalTokens(a []dupToken, ia int, b []dupToken, ib, width int) bool {
	for k := 0; k < width; k++ {
		if a[ia+k].norm != b[ib+k].norm {
			return false
		}
	}
	return true
}

func extendMatch(a []dupToken, ia int, b []dupToken, ib int) int {
	k := 0
	for ia+k < len(a) && ib+k < len(b) && a[ia+k].norm == b[ib+k].norm {
		k++
	}
	return k
}

// normalizedTokens produces a structure-preserving token stream: identifiers
// become "ID" and literals become "LIT" so renamed/retyped copies still match,
// while keywords, operators and punctuation are kept verbatim.
func normalizedTokens(language string, source []byte) []dupToken {
	if language == "go" {
		return goNormalizedTokens(source)
	}
	grammar, ok := dupGrammars[language]
	if !ok {
		return nil
	}
	return tsNormalizedTokens(grammar, source)
}

var dupGrammars = map[string]*sitter.Language{
	"python":     sitter.NewLanguage(tspython.Language()),
	"javascript": sitter.NewLanguage(tsjs.Language()),
	"jsx":        sitter.NewLanguage(tsjs.Language()),
	"typescript": sitter.NewLanguage(tsts.LanguageTypescript()),
	"tsx":        sitter.NewLanguage(tsts.LanguageTSX()),
}

func goNormalizedTokens(source []byte) []dupToken {
	var tokens []dupToken
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(source))
	s.Init(file, source, nil, 0)
	for {
		pos, tok, _ := s.Scan()
		if tok == token.EOF {
			break
		}
		line := fset.Position(pos).Line
		switch {
		case tok == token.IDENT:
			tokens = append(tokens, dupToken{"ID", line})
		case tok.IsLiteral():
			tokens = append(tokens, dupToken{"LIT", line})
		case tok == token.SEMICOLON, tok == token.COMMENT:
			// skip separators and comments
		default:
			tokens = append(tokens, dupToken{tok.String(), line})
		}
	}
	return tokens
}

func tsNormalizedTokens(grammar *sitter.Language, source []byte) []dupToken {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(grammar); err != nil {
		return nil
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	var tokens []dupToken
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		count := int(n.ChildCount())
		if count == 0 {
			if norm, ok := normalizeTSLeaf(n, source); ok {
				tokens = append(tokens, dupToken{norm, int(n.StartPosition().Row) + 1})
			}
			return
		}
		for i := 0; i < count; i++ {
			walk(n.Child(uint(i)))
		}
	}
	walk(tree.RootNode())
	return tokens
}

func normalizeTSLeaf(n *sitter.Node, src []byte) (string, bool) {
	t := n.Kind()
	if isCommentKind(t) {
		return "", false
	}
	if n.IsNamed() {
		if strings.HasSuffix(t, "identifier") {
			return "ID", true
		}
		return "LIT", true
	}
	if strings.TrimSpace(t) == "" {
		return "", false
	}
	return t, true
}
