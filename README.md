# Code Inspector

**A Go CLI and library that finds your highest-value refactoring targets — by ranking code on complexity × git churn, cognitive complexity, duplication, the Maintainability Index, and the import dependency graph, across any tree-sitter language.**

[![License: MIT](https://img.shields.io/github/license/miguelbravo7/code-inspector)](LICENSE)
[![Latest release](https://img.shields.io/github/v/tag/miguelbravo7/code-inspector?label=release)](https://github.com/miguelbravo7/code-inspector/tags)
[![Go Reference](https://pkg.go.dev/badge/github.com/miguelbravo7/code-inspector.svg)](https://pkg.go.dev/github.com/miguelbravo7/code-inspector)
![Go version](https://img.shields.io/github/go-mod/go-version/miguelbravo7/code-inspector)

`code-inspector` measures **code complexity**, finds **technical-debt hotspots**, detects **duplicate code**, maps the **dependency graph**, and ranks **where refactoring pays off most** — in one tool. It builds a metrics tree with per-file and per-function data, then a ranked summary of hotspots, complex functions, low-maintainability files, duplication, and import dependencies.

Think of it as an `scc`/`gocyclo`/`jscpd`-style metrics tool that *also* ranks **complexity × git-churn hotspots** — the "Your Code as a Crime Scene" prioritization (Adam Tornhill / CodeScene) — but free, offline, multi-language, and scriptable. It is both a **command-line tool** and an importable **Go package**, and it ships a ready-to-use **[Claude Code skill](#claude-code-skill)** for AI-assisted code review.

Module path: `github.com/miguelbravo7/code-inspector`

## Why code-inspector

Most metrics tools answer one question. This one answers "**what should I fix first?**" by combining the signals that matter:

- **Hotspots = complexity × git churn.** Code that is both complex *and* changed often is where bugs and effort concentrate — the highest-value refactor targets. This is the headline feature, and few free CLIs combine it with the rest of the metrics below.
- **All-in-one breadth.** Cyclomatic + cognitive complexity, nesting depth, Halstead, Maintainability Index, **token-normalized duplicate detection**, and an **import dependency graph** (fan-in / fan-out + cycle detection) — instead of stitching together `gocyclo` + `jscpd` + `madge`.
- **Truly multi-language via tree-sitter.** Real syntax trees, not regexes. 18 languages bundled, and `RegisterLanguage` adds *any* tree-sitter grammar — with metric hints auto-derived from the grammar's own vocabulary.
- **CLI *and* Go library.** Drive it from the shell, or embed the `inspector` package to build a quality gate or CI check.

Honest scope: it is a metrics CLI/library plus a Claude skill — not a server, dashboard, or LLM analyzer. Go and Python/JS/TS are high-fidelity; other languages use a generic adapter (treat their absolute numbers as directional). It is cgo, so building it needs a C compiler — see [Requirements](#requirements).

## Quickstart

Install the CLI (needs the cgo toolchain — see [Requirements](#requirements)):

```bash
go install github.com/miguelbravo7/code-inspector/cmd/code-inspector@latest
```

Run it on a directory:

```bash
code-inspector ./path/to/directory
```

You get a Unicode file tree annotated with per-file/per-function metrics, followed by a ranked **Summary** of hotspots, complex functions, low-maintainability files, duplication, and the dependency graph. See the full [example output](#example) below.

## Requirements

This package parses non-Go languages with [tree-sitter](https://tree-sitter.github.io),
whose grammars are C — so **cgo is required**. Any build that imports this
package (or the CLI) needs `CGO_ENABLED=1` and a C compiler on `PATH`:

- **Linux/macOS:** a system `gcc`/`clang` is usually already present.
- **Windows:** install a mingw-w64 toolchain and point Go at it:

  ```bash
  go env -w CGO_ENABLED=1
  go env -w CC=C:\path\to\mingw64\bin\gcc.exe
  ```

`CGO_ENABLED=0` and pure cross-compilation are not supported.

## Install

CLI:

```bash
go install github.com/miguelbravo7/code-inspector/cmd/code-inspector@latest
```

Library:

```bash
go get github.com/miguelbravo7/code-inspector/inspector
```

## As a Go library

Beyond the CLI, `code-inspector` is an importable Go package for computing code
metrics — useful for building a custom quality gate, a CI check, or your own
dashboard:

```go
package main

import (
	"fmt"
	"log"

	"github.com/miguelbravo7/code-inspector/inspector"
)

func main() {
	report, err := inspector.Inspect("./path/to/project", inspector.Options{})
	if err != nil {
		log.Fatal(err)
	}
	for _, h := range report.Summary.TopHotspots {
		fmt.Printf("%s\thot=%.0f cyc=%d churn=%d\n", h.Path, h.Hotspot, h.Cyclomatic, h.Churn)
	}
}
```

`Inspect` runs the full pipeline and returns a `*Report` (metrics tree, summary,
duplication, dependency graph). The individual stages — `BuildTree`,
`ComputeChurn`, `BuildSummary`, `DetectDuplication`, `BuildDependencyGraph` — are
also exported for finer control. Full API on
[pkg.go.dev](https://pkg.go.dev/github.com/miguelbravo7/code-inspector/inspector).

## Supported languages

Source is parsed into real syntax trees, not matched with regexes — Go via the
standard library `go/ast`, everything else via
[tree-sitter](https://github.com/tree-sitter/go-tree-sitter).

Bundled by default (18 languages): **Go, Python, JavaScript, JSX, TypeScript,
TSX, Rust, Java, C, C++, C#, Ruby, PHP, Bash, Scala, CSS, HTML, JSON**
(`.go .py .js .mjs .cjs .jsx .ts .tsx .rs .java .c .h .cc .cpp .cxx .hpp .cs .rb
.sh .bash .scala .sc .css .html .htm .json`).

Go uses `go/ast`; Python and the JS/TS family have hand-tuned tree-sitter specs;
the rest use a **generic adapter** that **auto-derives its hints from the grammar
itself** — at registration it introspects the grammar's node-kind and field
vocabulary (`NodeKindForId`, `FieldNameForId`, …) and classifies kinds into
functions / decisions / nesting / imports, on top of a curated cross-language
base. So a newly registered grammar adapts to its own vocabulary rather than
relying on a fixed list. **Any other tree-sitter grammar can be added** with
[`RegisterLanguage`](#adding-languages) and gets functions, complexity, line
breakdown, comments, and — where imports resolve to project files — dependency
edges.

## Adding languages

Bring any tree-sitter grammar (a `bindings/go` module) and register it at startup:

```go
import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tszig "github.com/tree-sitter-grammars/tree-sitter-zig/bindings/go"
	"github.com/miguelbravo7/code-inspector/inspector"
)

func init() {
	inspector.RegisterLanguage(inspector.LanguageConfig{
		Name:       "zig",
		Grammar:    sitter.NewLanguage(tszig.Language()),
		Extensions: []string{".zig"},
		// Optional: refine accuracy with Hints{FunctionKinds, DecisionKinds, ...}.
	})
}
```

The CLI's bundled set is fixed at build time; `RegisterLanguage` is for programs
that embed the `inspector` package (or a custom build of the CLI).

## Claude Code skill

This repo ships a ready-to-use [Claude Code](https://claude.com/claude-code)
**AI skill** at
[`.claude/skills/code-hotspots`](.claude/skills/code-hotspots/SKILL.md). It drives
the tool for a first-pass **code-health review**: runs the inspector, detects and
excludes generated code, finds the metrics that stand out for that codebase, and
proposes **prioritized, evidence-backed refactoring actions**. It is available
automatically when working in this repo; to use it anywhere, copy the
`code-hotspots` directory into `~/.claude/skills/`.

## Metrics

### Per file

- Physical line count, plus a **code / comment / blank** breakdown
- Import count and variable-binding count
- **Cyclomatic complexity** (sum across functions) and the highest single-function value
- **Halstead** volume/difficulty/effort and a 0–100 **Maintainability Index**
- **TODO/FIXME/HACK/XXX** marker count
- **Git churn** (commits touching the file) and a **hotspot score** (`complexity × churn`)

### Per function

- Name, signature hint, start line, line count
- **Cyclomatic complexity** (McCabe: decision points + 1)
- **Cognitive complexity** (SonarSource-style approximation that penalizes nesting)
- **Maintainability Index** (0–100)
- Max nesting depth and parameter count (available in JSON output)

### Ranked summary

After the tree, a summary aggregates totals and ranks:

- **Top hotspots** — files ranked by `complexity × git churn`, i.e. code that is
  both complex *and* changed often. These are the highest-value refactor targets.
  (Falls back to ranking by complexity when git history is unavailable.)
- **Most complex functions** — ranked by cyclomatic complexity.
- **Lowest maintainability** — files ranked by Maintainability Index (ascending).
- **Duplication** — token-level clone blocks across files. Each file is reduced to
  a structure-preserving token stream (identifiers and literals normalized) so
  renamed or retyped copies still match; clones of at least `-dup-min-tokens`
  tokens are reported with their locations.
- **Dependency graph** — intra-project import graph across languages (packages
  for Go; files for everything else). Go/Python/JS use dedicated resolvers;
  other languages use a generic, precision-first resolver (relative paths and
  unique path-suffix matches, qualified to the importer's language — ambiguous or
  external imports are never turned into edges). Reports **fan-in** (most
  depended-on = wide blast radius), **fan-out** (most dependencies = fragile),
  and **dependency cycles**.

## Usage

Run from a clone (see [Requirements](#requirements) for the cgo toolchain):

```bash
go run ./cmd/code-inspector -- ./path/to/directory
```

Or, once installed:

```bash
code-inspector ./path/to/directory
```

### Flags

- `-exclude=PATTERN`: file or directory names/glob patterns to skip (repeatable, e.g. `-exclude=*_test.go -exclude=sqlc`)
- `-no-default-excludes`: disable defaults (`.git`, `node_modules`, `dist`, `build`, `out`, `vendor`)
- `-supported-only`: include only supported file types in the output tree
- `-format=tree|json`: choose human-readable tree output or JSON output (JSON includes the ranked summary)
- `-workers=N`: file-analysis workers (default `1` sequential, `0` auto)
- `-no-summary`: skip the ranked summary
- `-no-git`: disable git churn and hotspot scoring
- `-top=N`: entries per ranked summary list (default `10`)
- `-no-dup`: disable duplicate-code detection
- `-dup-min-tokens=N`: minimum token run length for a clone (default `50`)
- `-no-deps`: disable the import dependency graph

## Comparison / alternatives

`code-inspector` overlaps with several focused tools but combines their jobs and
adds churn-based prioritization:

| Tools | Their focus | What `code-inspector` adds |
|---|---|---|
| [`scc`](https://github.com/boyter/scc), [`tokei`](https://github.com/XAMPPRocky/tokei) | fast line counting | per-function complexity, hotspots, duplication, dependency graph |
| [`gocyclo`](https://github.com/fzipp/gocyclo), [`gocognit`](https://github.com/uudashr/gocognit), [`lizard`](https://github.com/terryyin/lizard), [`radon`](https://github.com/rubik/radon) | complexity only | + cognitive/Halstead/Maintainability Index, **churn hotspots**, duplication, dependency graph, more languages |
| [`jscpd`](https://github.com/kucherenko/jscpd), PMD CPD | duplication only | + complexity, hotspots, dependency graph |
| [`madge`](https://github.com/pahen/madge), [`dependency-cruiser`](https://github.com/sverweij/dependency-cruiser) | dependency graph only | + complexity, hotspots, duplication |
| SonarQube, Code Climate, [CodeScene](https://codescene.com) | hosted platforms / dashboards | a **free, offline CLI + Go library** with the complexity × churn "crime scene" prioritization CodeScene popularized — no server required |

If you want a command-line, open-source take on hotspot analysis, or one tool
that covers what `gocyclo`, `jscpd`, and `madge` each do separately, that's the
niche this fills.

## Benchmarks

Run all inspector benchmarks:

```bash
go test ./inspector -run ^$ -bench . -benchmem
```

Per-language analyzer benchmarks (hand-tuned go/python/typescript, plus the
generic adapter on rust/java):

```bash
go test ./inspector -run ^$ -bench 'AnalyzeSources|AnalyzeGeneric' -benchmem
```

Registration introspection, dependency graph, duplication, and the full pipeline:

```bash
go test ./inspector -run ^$ -bench 'GenericSpecBuild|BuildDependencyGraph|DetectDuplication|Inspect' -benchmem
```

> Notes (measured locally, GOMAXPROCS=16): the tree-sitter walk interns node
> kinds (via a per-grammar id→kind map) and computes file- and function-level
> metrics in a single pass, which cut allocations by ~80% and analysis time by
> 20–45% versus a naive `Kind()`-per-node, two-pass walk. The standard-library
> `go/ast` analyzer is still several times faster per byte than the tree-sitter
> ones; per-language registration introspection is a one-time ~0.3 ms.
>
> Traversal uses a single global worker pool over the whole file set. Even so,
> concurrency (`-workers=0`) is *slower* than the sequential default
> (`-workers=1`) for a tree-sitter workload: every node access is a cgo call, and
> that per-node cgo overhead under many goroutines outweighs the multi-core gain.
> Sequential is therefore the default; the pool helps only when parsing is cheap
> relative to I/O.

## Example

```text
my-project/
├── src/
│   ├── app.ts [lines:42 code:33 cyc:9 mi:64 funcs:4 churn:7 hot:63]
│   │   ├── fn: bootstrap | () | line 5 | lines 4 | cyc 1
│   │   └── fn: loadConfig | (path: string) | line 18 | lines 7 | cyc 4 | cog 6
│   └── worker.py [lines:31 code:24 cyc:6 mi:58 funcs:2 todo:1 churn:3 hot:18]
│       ├── fn: run | (task) | line 4 | lines 10 | cyc 5 | cog 8
│       └── fn: helper | () | line 20 | lines 5 | cyc 1
└── main.go [lines:27 code:21 cyc:4 mi:71 funcs:2 churn:2 hot:8]
    ├── fn: main | () | line 10 | lines 5 | cyc 1
    └── fn: service.start | (ctx context.Context) error | line 16 | lines 3 | cyc 2

Summary
  files: 3 (3 analyzed)  lines: 100 (code 78 / comment 9 / blank 13)
  functions: 8  todo markers: 1

  Top hotspots (complexity x git churn):
    src/app.ts                               hot 63      cyc 9     churn 7
    worker.py                                hot 18      cyc 6     churn 3
    main.go                                  hot 8       cyc 4     churn 2

  Most complex functions:
    run                          cyc 5    cog 8     worker.py:4
    loadConfig                   cyc 4    cog 6     src/app.ts:18

  Lowest maintainability (0-100, higher is better):
    worker.py                                mi 58.0  cyc 6
    src/app.ts                               mi 64.0  cyc 9

  Duplication: 1 clone blocks, ~6 duplicated lines (>= 50 tokens):
    52 tokens / 6 lines:
      src/app.ts:18-23
      src/legacy.ts:4-9

  Dependency graph: 3 modules, 2 internal edges, 5 external imports

  Most depended-on (high fan-in = wide blast radius):
    src/app.ts                                   fan-in 2    fan-out 1
```

## License

[MIT](LICENSE)
