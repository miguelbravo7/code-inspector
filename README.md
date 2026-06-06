# Code Inspector

A standalone root-level CLI utility to inspect source trees and print code metrics as a Unicode file tree.

## Supported Languages

- JavaScript (`.js`, `.mjs`, `.cjs`)
- JSX (`.jsx`)
- TypeScript (`.ts`, `.tsx`)
- Python (`.py`)
- Go (`.go`)

## Metrics Per Supported File

- Physical line count
- Number of imports
- Number of variable definitions
- Number of functions declared
- Enumerated function entries (name, signature hint, line)
- Function line count per enumerated function

## Usage

```bash
go run ./cmd/code-inspector -- ./path/to/directory
```

### Flags

- `-exclude="dir1,dir2"`: additional directory names to skip
- `-no-default-excludes`: disable defaults (`.git`, `node_modules`, `dist`, `build`, `out`, `vendor`)
- `-supported-only`: include only supported file types in the output tree
- `-format=tree|json`: choose human-readable tree output or JSON output
- `-workers=N`: file-analysis workers per directory (default `1` sequential, `0` auto)

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

Run analyzer hotspot benchmarks:

```bash
go test ./internal/inspector -run ^$ -bench AnalyzeHotspots -benchmem
```

## Example

```text
my-project/
├── src/
│   ├── app.ts [lines:42 imports:3 vars:7 funcs:4]
│   │   ├── fn: bootstrap | () | line 5 | lines 4
│   │   └── fn: loadConfig | (path: string) | line 18 | lines 7
│   └── worker.py [lines:31 imports:2 vars:5 funcs:2]
│       ├── fn: run | (task) | line 4 | lines 10
│       └── fn: helper | () | line 20 | lines 5
└── main.go [lines:27 imports:4 vars:3 funcs:2]
	├── fn: main | () | line 10 | lines 5
	└── fn: service.start | (ctx context.Context) error | line 16 | lines 3
```
