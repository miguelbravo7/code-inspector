package inspector

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDepFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", name, err)
	}
}

func TestDependencyGraphFanInAndCycle(t *testing.T) {
	dir := t.TempDir()
	// utils is imported by both a and b -> fan-in 2.
	writeDepFile(t, dir, "utils.ts", "export const u = 1;\n")
	writeDepFile(t, dir, "a.ts", "import { u } from \"./utils\";\nimport { b } from \"./b\";\nexport const a = u;\n")
	writeDepFile(t, dir, "b.ts", "import { u } from \"./utils\";\nimport { a } from \"./a\";\nexport const b = u;\n")

	tree, err := BuildTree(dir, Config{ExcludedDirs: map[string]struct{}{}})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	report := BuildDependencyGraph(tree, dir, 10)
	if report.Nodes < 3 {
		t.Fatalf("expected at least 3 nodes, got %d", report.Nodes)
	}

	var utilsFanIn int
	for _, s := range report.MostDependedOn {
		if s.Node == "utils.ts" {
			utilsFanIn = s.FanIn
		}
	}
	if utilsFanIn != 2 {
		t.Fatalf("expected utils.ts fan-in 2, got %d", utilsFanIn)
	}

	// a.ts <-> b.ts form a cycle.
	if len(report.Cycles) == 0 {
		t.Fatalf("expected a dependency cycle between a.ts and b.ts")
	}
	found := false
	for _, cycle := range report.Cycles {
		if len(cycle) == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a 2-node cycle, got %v", report.Cycles)
	}
}

func TestDependencyGraphGoPackages(t *testing.T) {
	dir := t.TempDir()
	writeDepFile(t, dir, "go.mod", "module example.com/proj\n\ngo 1.22\n")
	writeDepFile(t, dir, "main.go", "package main\n\nimport \"example.com/proj/util\"\n\nfunc main() { _ = util.X }\n")
	writeDepFile(t, dir, "util/util.go", "package util\n\nvar X = 1\n")

	tree, err := BuildTree(dir, Config{ExcludedDirs: map[string]struct{}{}})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	report := BuildDependencyGraph(tree, dir, 10)
	var utilFanIn int
	for _, s := range report.MostDependedOn {
		if s.Node == "example.com/proj/util" {
			utilFanIn = s.FanIn
		}
	}
	if utilFanIn != 1 {
		t.Fatalf("expected util package fan-in 1, got %d (report: %+v)", utilFanIn, report.MostDependedOn)
	}
}
