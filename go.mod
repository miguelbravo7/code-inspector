module code-inspector

go 1.22

require github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82

// The javascript grammar is published both inside the main module and as a
// stale standalone module; exclude the stray one so it resolves from the parent.
exclude github.com/smacker/go-tree-sitter/javascript v0.0.1
