package inspector

import (
	"fmt"
	"sync"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

// TestRegistryConcurrent exercises the registry under -race: RegisterLanguage
// (which now introspects the grammar) running concurrently with reads.
func TestRegistryConcurrent(t *testing.T) {
	const workers = 24
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				RegisterLanguage(LanguageConfig{
					Name:       fmt.Sprintf("rusty%d", i),
					Grammar:    sitter.NewLanguage(tsrust.Language()),
					Extensions: []string{fmt.Sprintf(".rusty%d", i)},
				})
			case 1:
				_ = SupportedExtensions()
			case 2:
				_ = SupportedLanguages()
			default:
				_, _ = analyzeSource("rust", []byte("fn f(a: i32) -> i32 { if a > 0 { a } else { -a } }\n"))
			}
		}(i)
	}
	wg.Wait()

	// Registry must stay self-consistent.
	for _, ext := range SupportedExtensions() {
		if _, ok := lookupByExtension(ext); !ok {
			t.Fatalf("extension %q has no registered language", ext)
		}
	}
}
