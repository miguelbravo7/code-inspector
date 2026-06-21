package inspector

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FileHotspot is a ranked file entry in a Summary.
type FileHotspot struct {
	Path       string
	Language   string
	Cyclomatic int
	Churn      int
	Hotspot    float64
	LineCount  int
}

// FunctionHotspot is a ranked function entry in a Summary.
type FunctionHotspot struct {
	Path       string
	Name       string
	Line       int
	Cyclomatic int
	Cognitive  int
	LineCount  int
}

// Summary is an aggregate, ranked view of a scan, built to surface the highest
// value places to improve.
type Summary struct {
	Files          int
	SupportedFiles int
	TotalLines     int
	TotalCode      int
	TotalComment   int
	TotalBlank     int
	TotalFunctions int
	TotalTodos     int
	GitChurn       bool
	TopHotspots    []FileHotspot
	MostComplex    []FunctionHotspot
	Longest        []FunctionHotspot
}

// ComputeChurn annotates each file node with its git commit frequency and a
// hotspot score (complexity * churn). It returns false when the scan root is
// not inside a git work tree or git is unavailable, leaving the tree untouched.
func ComputeChurn(root *TreeNode, scanRoot string) bool {
	absScan, err := filepath.Abs(scanRoot)
	if err != nil {
		return false
	}

	repoRoot, err := runGit(absScan, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return false
	}

	logOut, err := runGit(absScan, "log", "--no-merges", "--name-only", "--format=")
	if err != nil {
		return false
	}

	churn := make(map[string]int)
	for _, line := range strings.Split(logOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		churn[line]++
	}

	var walk func(n *TreeNode)
	walk = func(n *TreeNode) {
		if !n.IsDir && n.Metrics != nil {
			if rel, err := filepath.Rel(repoRoot, n.Path); err == nil {
				if c, found := churn[filepath.ToSlash(rel)]; found {
					n.Churn = c
					n.Hotspot = float64(c) * float64(n.Metrics.Cyclomatic)
				}
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	return true
}

// BuildSummary aggregates the tree into ranked lists. topN caps each list.
func BuildSummary(root *TreeNode, topN int, gitChurn bool) Summary {
	if topN <= 0 {
		topN = 10
	}
	summary := Summary{GitChurn: gitChurn}

	files := make([]FileHotspot, 0)
	funcs := make([]FunctionHotspot, 0)

	var walk func(n *TreeNode)
	walk = func(n *TreeNode) {
		if !n.IsDir {
			summary.Files++
			if n.Metrics != nil {
				m := n.Metrics
				summary.SupportedFiles++
				summary.TotalLines += m.LineCount
				summary.TotalCode += m.CodeLines
				summary.TotalComment += m.CommentLines
				summary.TotalBlank += m.BlankLines
				summary.TotalFunctions += len(m.Functions)
				summary.TotalTodos += m.TodoCount

				path := n.RelPath
				if path == "" {
					path = n.Name
				}
				files = append(files, FileHotspot{
					Path:       path,
					Language:   m.Language,
					Cyclomatic: m.Cyclomatic,
					Churn:      n.Churn,
					Hotspot:    n.Hotspot,
					LineCount:  m.LineCount,
				})
				for _, fn := range m.Functions {
					funcs = append(funcs, FunctionHotspot{
						Path:       path,
						Name:       fn.Name,
						Line:       fn.Line,
						Cyclomatic: fn.Cyclomatic,
						Cognitive:  fn.Cognitive,
						LineCount:  fn.LineCount,
					})
				}
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)

	if gitChurn {
		sort.SliceStable(files, func(i, j int) bool {
			if files[i].Hotspot != files[j].Hotspot {
				return files[i].Hotspot > files[j].Hotspot
			}
			return files[i].Cyclomatic > files[j].Cyclomatic
		})
		for _, f := range files {
			if f.Hotspot <= 0 {
				break
			}
			summary.TopHotspots = append(summary.TopHotspots, f)
			if len(summary.TopHotspots) >= topN {
				break
			}
		}
	} else {
		sort.SliceStable(files, func(i, j int) bool {
			return files[i].Cyclomatic > files[j].Cyclomatic
		})
		for _, f := range files {
			if f.Cyclomatic <= 0 {
				break
			}
			summary.TopHotspots = append(summary.TopHotspots, f)
			if len(summary.TopHotspots) >= topN {
				break
			}
		}
	}

	complexFuncs := append([]FunctionHotspot(nil), funcs...)
	sort.SliceStable(complexFuncs, func(i, j int) bool {
		if complexFuncs[i].Cyclomatic != complexFuncs[j].Cyclomatic {
			return complexFuncs[i].Cyclomatic > complexFuncs[j].Cyclomatic
		}
		return complexFuncs[i].Cognitive > complexFuncs[j].Cognitive
	})
	for _, fn := range complexFuncs {
		if fn.Cyclomatic <= 1 {
			break
		}
		summary.MostComplex = append(summary.MostComplex, fn)
		if len(summary.MostComplex) >= topN {
			break
		}
	}

	longest := append([]FunctionHotspot(nil), funcs...)
	sort.SliceStable(longest, func(i, j int) bool {
		return longest[i].LineCount > longest[j].LineCount
	})
	if len(longest) > topN {
		longest = longest[:topN]
	}
	summary.Longest = longest

	return summary
}

func runGit(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
