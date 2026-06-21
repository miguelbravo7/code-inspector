package inspector

import (
	"math"
	"strings"
)

// complexity bundles the per-function complexity metrics computed by analyzers.
type complexity struct {
	cyclomatic int
	cognitive  int
	maxNesting int
}

// todoMarkers are case-insensitive tokens treated as tech-debt signals.
var todoMarkers = []string{"TODO", "FIXME", "HACK", "XXX"}

// countTodoMarkers counts tech-debt markers inside a comment's text.
func countTodoMarkers(commentText string) int {
	upper := strings.ToUpper(commentText)
	count := 0
	for _, marker := range todoMarkers {
		count += strings.Count(upper, marker)
	}
	return count
}

// lineSpan is a 0-based [start,end] range over source rows/columns, used to
// describe the location of comment tokens for line classification.
type lineSpan struct {
	startRow int
	startCol int
	endRow   int
	endCol   int
}

// lineClassification splits a source file into code, comment and blank line
// counts. Comments are supplied as spans by the language analyzer so the same
// logic serves every language. A line that holds both code and a trailing
// comment is counted as code.
func lineClassification(source []byte, comments []lineSpan) (code, comment, blank int) {
	lines := strings.Split(string(source), "\n")
	n := len(lines)
	if n == 0 {
		return 0, 0, 0
	}

	commentCovered := make([]bool, n)
	codeBearing := make([]bool, n)

	for _, span := range comments {
		for row := span.startRow; row <= span.endRow && row < n; row++ {
			if row < 0 {
				continue
			}
			commentCovered[row] = true
		}
		if span.startRow >= 0 && span.startRow < n {
			if hasNonSpace(lines[span.startRow][:clampCol(lines[span.startRow], span.startCol)]) {
				codeBearing[span.startRow] = true
			}
		}
		if span.endRow >= 0 && span.endRow < n {
			tail := lines[span.endRow][clampCol(lines[span.endRow], span.endCol):]
			if hasNonSpace(tail) {
				codeBearing[span.endRow] = true
			}
		}
	}

	for row := 0; row < n; row++ {
		if strings.TrimSpace(lines[row]) == "" {
			blank++
			continue
		}
		if commentCovered[row] && !codeBearing[row] {
			comment++
			continue
		}
		code++
	}
	return code, comment, blank
}

func clampCol(line string, col int) int {
	if col < 0 {
		return 0
	}
	if col > len(line) {
		return len(line)
	}
	return col
}

func hasNonSpace(s string) bool {
	return strings.TrimSpace(s) != ""
}

// halsteadAccumulator collects operator and operand occurrences so Halstead
// measures can be derived once counting is complete. Operators are keyed by
// their (interned) token text; operands are keyed by an FNV-1a hash of their
// bytes so the hot tree-sitter walk can record them without allocating a string
// per leaf. Hash collisions only ever undercount distinct operands by a
// negligible amount, which is acceptable for a metric.
type halsteadAccumulator struct {
	operators map[string]int
	operands  map[uint64]int
}

func newHalstead() *halsteadAccumulator {
	return &halsteadAccumulator{
		operators: make(map[string]int),
		operands:  make(map[uint64]int),
	}
}

func (h *halsteadAccumulator) addOperator(token string) {
	if token == "" {
		return
	}
	h.operators[token]++
}

func (h *halsteadAccumulator) addOperand(token string) {
	if token == "" {
		return
	}
	h.operands[fnv64String(token)]++
}

func (h *halsteadAccumulator) addOperandBytes(b []byte) {
	if len(b) == 0 {
		return
	}
	h.operands[fnv64Bytes(b)]++
}

func (h *halsteadAccumulator) metrics() Halstead {
	n1 := len(h.operators)
	n2 := len(h.operands)
	N1 := sumValues(h.operators)
	N2 := sumValues(h.operands)

	vocabulary := n1 + n2
	length := N1 + N2

	out := Halstead{Vocabulary: vocabulary, Length: length}
	if vocabulary > 0 {
		out.Volume = float64(length) * math.Log2(float64(vocabulary))
	}
	if n2 > 0 {
		out.Difficulty = (float64(n1) / 2.0) * (float64(N2) / float64(n2))
	}
	out.Effort = out.Difficulty * out.Volume
	return out
}

func sumValues[K comparable](m map[K]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

func fnv64Bytes(b []byte) uint64 {
	h := uint64(fnvOffset64)
	for _, c := range b {
		h ^= uint64(c)
		h *= fnvPrime64
	}
	return h
}

func fnv64String(s string) uint64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

// maintainabilityIndex returns the normalized 0-100 Maintainability Index
// (Microsoft Visual Studio variant) from Halstead volume, cyclomatic complexity
// and lines of code. Higher is more maintainable.
func maintainabilityIndex(volume float64, cyclomatic, loc int) float64 {
	if loc <= 0 {
		return 100
	}
	v := volume
	if v < 1 {
		v = 1
	}
	raw := (171.0 - 5.2*math.Log(v) - 0.23*float64(cyclomatic) - 16.2*math.Log(float64(loc))) * 100.0 / 171.0
	if raw < 0 {
		return 0
	}
	if raw > 100 {
		return 100
	}
	return raw
}

// applyFunctionRollups fills the file-level complexity rollups from the
// per-function metrics.
func applyFunctionRollups(metrics *FileMetrics) {
	total := 0
	max := 0
	for _, fn := range metrics.Functions {
		total += fn.Cyclomatic
		if fn.Cyclomatic > max {
			max = fn.Cyclomatic
		}
	}
	metrics.Cyclomatic = total
	metrics.MaxComplexity = max
}
