# Code Inspector

A Go library and CLI to inspect source trees, surface code-quality metrics, and
rank where improvements pay off the most. It builds a metrics tree with per-file
and per-function data, then a ranked summary of hotspots, complex functions,
low-maintainability files, duplication, and the import dependency graph.

Module path: `github.com/miguelbravo7/code-inspector`

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

## Library usage

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
also exported for finer control.

## Parsing

Source is parsed into real syntax trees, not matched with regexes:

- **Go** — the standard library `go/ast`.
- **Everything else** — tree-sitter grammars via the official
  [`github.com/tree-sitter/go-tree-sitter`](https://github.com/tree-sitter/go-tree-sitter)
  bindings.

## Supported Languages

Bundled by default: **Go, Python, JavaScript, JSX, TypeScript, TSX, Rust, Java,
C, C++, C#, Ruby, PHP, Bash, Scala, CSS, HTML, JSON**
(`.go .py .js .mjs .cjs .jsx .ts .tsx .rs .java .c .h .cc .cpp .cxx .hpp .cs .rb
.sh .bash .scala .sc .css .html .htm .json`).

Go uses `go/ast`; Python and the JS/TS family have hand-tuned tree-sitter specs;
the rest use a **generic adapter** that **auto-derives its hints from the
grammar itself** — at registration it introspects the grammar's node-kind and
field vocabulary (`NodeKindForId`, `FieldNameForId`, …) and classifies kinds into
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

This repo bundles a [Claude Code](https://claude.com/claude-code) skill at
[`.claude/skills/code-hotspots`](.claude/skills/code-hotspots/SKILL.md) that
drives the tool for a first-pass code-health review: it runs the inspector,
detects and excludes generated code, finds the metrics that stand out for that
codebase, and proposes prioritized, evidence-backed refactoring actions. It is
available automatically when working in this repo; to use it anywhere, copy the
`code-hotspots` directory into `~/.claude/skills/`.

## Metrics

### Per file

- Physical line count, plus a **code / comment / blank** breakdown
- Import count and variable-binding count
- **Cyclomatic complexity** (sum across functions) and the highest single-function value
- **Halstead** volume/difficulty/effort and a 0-100 **Maintainability Index**
- **TODO/FIXME/HACK/XXX** marker count
- **Git churn** (commits touching the file) and a **hotspot score** (`complexity × churn`)

### Per function

- Name, signature hint, start line, line count
- **Cyclomatic complexity** (McCabe: decision points + 1)
- **Cognitive complexity** (SonarSource-style approximation that penalizes nesting)
- **Maintainability Index** (0-100)
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
  depended-on = wide blast radius),
  **fan-out** (most dependencies = fragile), and **dependency cycles**.

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
- `-workers=N`: file-analysis workers per directory (default `1` sequential, `0` auto)
- `-no-summary`: skip the ranked summary
- `-no-git`: disable git churn and hotspot scoring
- `-top=N`: entries per ranked summary list (default `10`)
- `-no-dup`: disable duplicate-code detection
- `-dup-min-tokens=N`: minimum token run length for a clone (default `50`)
- `-no-deps`: disable the import dependency graph

## Benchmarks

Run all inspector benchmarks:

```bash
go test ./inspector -run ^$ -bench . -benchmem
```

Compare traversal concurrency against sequential baseline:

```bash
go test ./inspector -run ^$ -bench BuildTreeTraversal -benchmem
```

Run the traversal benchmark matrix (supported-only true/false across multiple file counts):

```bash
go test ./inspector -run ^$ -bench BuildTreeTraversalMatrix -benchmem
```

Run per-language analyzer benchmarks (hand-tuned go/python/typescript, and the
generic adapter on rust/java):

```bash
go test ./inspector -run ^$ -bench 'AnalyzeSources|AnalyzeGeneric' -benchmem
```

Benchmark the registration introspection, dependency graph, duplication, and the
full pipeline:

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
> Traversal uses a single global worker pool over the whole file set (not a pool
> per directory). Even so, concurrency (`-workers=0`) is *slower* than the
> sequential default (`-workers=1`) for a tree-sitter workload: every node access
> is a cgo call, and that per-node cgo overhead under many goroutines outweighs
> the multi-core gain. Sequential is therefore the default; the pool helps only
> when parsing is cheap relative to I/O.

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
