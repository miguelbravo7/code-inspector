# Code Inspector

A standalone root-level CLI utility to inspect source trees, surface code-quality
metrics, and rank where improvements pay off the most. It prints a Unicode file
tree with per-file and per-function metrics, followed by a ranked summary of
hotspots and the most complex functions.

## Parsing

Source is parsed into real syntax trees, not matched with regexes:

- **Go** — the standard library `go/ast`.
- **Python, JavaScript, JSX, TypeScript, TSX** — [tree-sitter](https://tree-sitter.github.io)
  grammars via [`github.com/smacker/go-tree-sitter`](https://github.com/smacker/go-tree-sitter).

Because tree-sitter grammars are C, **cgo is required** (see [Building](#building)).

## Supported Languages

- JavaScript (`.js`, `.mjs`, `.cjs`)
- JSX (`.jsx`)
- TypeScript (`.ts`)
- TSX (`.tsx`)
- Python (`.py`)
- Go (`.go`)

## Metrics

### Per file

- Physical line count, plus a **code / comment / blank** breakdown
- Import count and variable-binding count
- **Cyclomatic complexity** (sum across functions) and the highest single-function value
- **TODO/FIXME/HACK/XXX** marker count
- **Git churn** (commits touching the file) and a **hotspot score** (`complexity × churn`)

### Per function

- Name, signature hint, start line, line count
- **Cyclomatic complexity** (McCabe: decision points + 1)
- **Cognitive complexity** (SonarSource-style approximation that penalizes nesting)
- Max nesting depth and parameter count (available in JSON output)

### Ranked summary

After the tree, a summary aggregates totals and ranks:

- **Top hotspots** — files ranked by `complexity × git churn`, i.e. code that is
  both complex *and* changed often. These are the highest-value refactor targets.
  (Falls back to ranking by complexity when git history is unavailable.)
- **Most complex functions** — ranked by cyclomatic complexity.

## Building

cgo and a C compiler are required.

- **Linux/macOS:** a system `gcc`/`clang` is usually already present.
- **Windows:** install a mingw-w64 toolchain and point Go at it, e.g.

  ```bash
  go env -w CGO_ENABLED=1
  go env -w CC=C:\path\to\mingw64\bin\gcc.exe
  ```

Build:

```bash
go build ./cmd/code-inspector
```

## Usage

```bash
go run ./cmd/code-inspector -- ./path/to/directory
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

## Benchmarks

Run all inspector benchmarks:

```bash
go test ./internal/inspector -run ^$ -bench . -benchmem
```

Compare traversal concurrency against sequential baseline:

```bash
go test ./internal/inspector -run ^$ -bench BuildTreeTraversal -benchmem
```

Run the traversal benchmark matrix (supported-only true/false across multiple file counts):

```bash
go test ./internal/inspector -run ^$ -bench BuildTreeTraversalMatrix -benchmem
```

Run per-language analyzer benchmarks:

```bash
go test ./internal/inspector -run ^$ -bench AnalyzeSources -benchmem
```

## Example

```text
my-project/
├── src/
│   ├── app.ts [lines:42 code:33 cyc:9 funcs:4 churn:7 hot:63]
│   │   ├── fn: bootstrap | () | line 5 | lines 4 | cyc 1
│   │   └── fn: loadConfig | (path: string) | line 18 | lines 7 | cyc 4 | cog 6
│   └── worker.py [lines:31 code:24 cyc:6 funcs:2 todo:1 churn:3 hot:18]
│       ├── fn: run | (task) | line 4 | lines 10 | cyc 5 | cog 8
│       └── fn: helper | () | line 20 | lines 5 | cyc 1
└── main.go [lines:27 code:21 cyc:4 funcs:2 churn:2 hot:8]
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
```
