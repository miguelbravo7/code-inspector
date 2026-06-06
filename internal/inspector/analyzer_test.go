package inspector

import "testing"

func findFunctionByName(functions []FunctionInfo, name string) *FunctionInfo {
	for idx := range functions {
		if functions[idx].Name == name {
			return &functions[idx]
		}
	}
	return nil
}

func TestAnalyzeGoSourceMetrics(t *testing.T) {
	source := []byte(`package sample
import "fmt"

var global = 1

func main() {
	local := 2
	helper := func(x int) int { return x + local }
	fmt.Println(helper(global))
}

type service struct{}

func (s *service) run() {}
`)

	metrics, err := analyzeGoSource(source)
	if err != nil {
		t.Fatalf("analyzeGoSource returned error: %v", err)
	}

	if metrics.ImportCount != 1 {
		t.Fatalf("expected 1 import, got %d", metrics.ImportCount)
	}
	if metrics.VariableCount != 3 {
		t.Fatalf("expected 3 variable definitions, got %d", metrics.VariableCount)
	}
	if len(metrics.Functions) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(metrics.Functions))
	}

	mainFn := findFunctionByName(metrics.Functions, "main")
	if mainFn == nil {
		t.Fatalf("expected to find main function")
	}
	if mainFn.LineCount != 5 {
		t.Fatalf("expected main function to have 5 lines, got %d", mainFn.LineCount)
	}

	runFn := findFunctionByName(metrics.Functions, "*service.run")
	if runFn == nil {
		t.Fatalf("expected to find receiver method *service.run")
	}
	if runFn.LineCount != 1 {
		t.Fatalf("expected receiver method to have 1 line, got %d", runFn.LineCount)
	}
}

func TestAnalyzePythonSourceMetrics(t *testing.T) {
	source := []byte(`import os
from sys import path

x = 1
a, b = 2, 3

class Service:
	def run(self):
		y = 2

def top():
	return y if False else x
`)

	metrics, err := analyzePythonSource(source)
	if err != nil {
		t.Fatalf("analyzePythonSource returned error: %v", err)
	}

	if metrics.ImportCount != 2 {
		t.Fatalf("expected 2 imports, got %d", metrics.ImportCount)
	}
	if metrics.VariableCount != 4 {
		t.Fatalf("expected 4 variable definitions, got %d", metrics.VariableCount)
	}
	if len(metrics.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(metrics.Functions))
	}

	runFn := findFunctionByName(metrics.Functions, "run")
	if runFn == nil {
		t.Fatalf("expected to find run function")
	}
	if runFn.LineCount != 2 {
		t.Fatalf("expected run function to have 2 lines, got %d", runFn.LineCount)
	}

	topFn := findFunctionByName(metrics.Functions, "top")
	if topFn == nil {
		t.Fatalf("expected to find top function")
	}
	if topFn.LineCount != 2 {
		t.Fatalf("expected top function to have 2 lines, got %d", topFn.LineCount)
	}
}

func TestAnalyzeJavaScriptLikeSourceMetrics(t *testing.T) {
	source := []byte(`import fs from "fs";
const path = require("path");
let count = 0;
const add = (a, b) => a + b;

function boot() {
	return add(count, 1);
}

class Service {
	start() { return boot(); }
}
`)

	metrics, err := analyzeJavaScriptLikeSource(source, "typescript")
	if err != nil {
		t.Fatalf("analyzeJavaScriptLikeSource returned error: %v", err)
	}

	if metrics.ImportCount != 2 {
		t.Fatalf("expected 2 imports, got %d", metrics.ImportCount)
	}
	if metrics.VariableCount != 3 {
		t.Fatalf("expected 3 variable definitions, got %d", metrics.VariableCount)
	}
	if len(metrics.Functions) != 3 {
		t.Fatalf("expected 3 functions, got %d", len(metrics.Functions))
	}

	addFn := findFunctionByName(metrics.Functions, "add")
	if addFn == nil {
		t.Fatalf("expected to find add function")
	}
	if addFn.LineCount != 1 {
		t.Fatalf("expected add function to have 1 line, got %d", addFn.LineCount)
	}

	bootFn := findFunctionByName(metrics.Functions, "boot")
	if bootFn == nil {
		t.Fatalf("expected to find boot function")
	}
	if bootFn.LineCount != 3 {
		t.Fatalf("expected boot function to have 3 lines, got %d", bootFn.LineCount)
	}

	startFn := findFunctionByName(metrics.Functions, "start")
	if startFn == nil {
		t.Fatalf("expected to find class method start")
	}
	if startFn.LineCount != 1 {
		t.Fatalf("expected class method start to have 1 line, got %d", startFn.LineCount)
	}
}

func TestSortFunctionsOrdersByLineThenNameThenSignature(t *testing.T) {
	functions := []FunctionInfo{
		{Name: "zeta", Signature: "(z int)", Line: 10},
		{Name: "alpha", Signature: "(y int)", Line: 10},
		{Name: "beta", Signature: "()", Line: 2},
		{Name: "alpha", Signature: "(a int)", Line: 10},
	}

	sortFunctions(functions)

	expected := []FunctionInfo{
		{Name: "beta", Signature: "()", Line: 2},
		{Name: "alpha", Signature: "(a int)", Line: 10},
		{Name: "alpha", Signature: "(y int)", Line: 10},
		{Name: "zeta", Signature: "(z int)", Line: 10},
	}

	for idx := range expected {
		if functions[idx].Line != expected[idx].Line || functions[idx].Name != expected[idx].Name || functions[idx].Signature != expected[idx].Signature {
			t.Fatalf("unexpected function order at index %d: got %+v, expected %+v", idx, functions[idx], expected[idx])
		}
	}
}
