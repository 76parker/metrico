package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGenerate_WritesResetMethodsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOCACHE", filepath.Join(root, "gocache"))
	t.Setenv("GOWORK", "off")

	writeFixtureFile(t, root, "go.mod", "module example.com/resetfixture\n\ngo 1.26.2\n")
	writeFixtureFile(t, root, "fixture/model.go", `package fixture

var resetCalls int

type nested struct {
	value int
}

func (n *nested) Reset() {
	resetCalls++
	n.value = 0
}

type nestedWithoutReset struct {
	value int
}

// generate:reset
type state[T any] struct {
	count           int
	text            string
	ready           bool
	countPointer    *int
	countPointerPtr **int
	items           []string
	itemsPointer    *[]string
	labels          map[string]int
	labelsPointer   *map[string]int
	child           nested
	childPointer    *nested
	childWithoutReset nestedWithoutReset
	dynamicChild    any
	genericValue    T
}
`)
	writeFixtureFile(t, root, "fixture/model_test.go", `package fixture

import "testing"

func TestGeneratedReset(t *testing.T) {
	count := 42
	items := make([]string, 2, 8)
	labels := map[string]int{"key": 1}
	pointedItems := make([]string, 1, 5)
	pointedLabels := map[string]int{"pointed": 2}
	stateValue := state[string]{
		count:            9,
		text:             "value",
		ready:            true,
		countPointer:     &count,
		countPointerPtr:  &[]*int{&count}[0],
		items:            items,
		itemsPointer:     &pointedItems,
		labels:           labels,
		labelsPointer:    &pointedLabels,
		child:            nested{value: 3},
		childPointer:     &nested{value: 4},
		childWithoutReset: nestedWithoutReset{value: 5},
		dynamicChild:     &nested{value: 6},
		genericValue:     "generic",
	}
	itemsStorage := &stateValue.items[0]
	pointedItemsStorage := &pointedItems[0]
	resetCalls = 0

	stateValue.Reset()

	if stateValue.count != 0 || stateValue.text != "" || stateValue.ready {
		t.Fatalf("primitive fields were not reset: %+v", stateValue)
	}
	if *stateValue.countPointer != 0 || **stateValue.countPointerPtr != 0 {
		t.Fatal("pointer fields were not reset")
	}
	if stateValue.items == nil || len(stateValue.items) != 0 || cap(stateValue.items) != 8 {
		t.Fatalf("slice was not truncated in place: len=%d cap=%d", len(stateValue.items), cap(stateValue.items))
	}
	if &stateValue.items[:1][0] != itemsStorage {
		t.Fatal("slice backing array was not retained")
	}
	if len(stateValue.labels) != 0 || stateValue.labels == nil {
		t.Fatal("map was not cleared")
	}
	if len(*stateValue.itemsPointer) != 0 || cap(*stateValue.itemsPointer) != 5 {
		t.Fatal("pointed-to slice was not truncated in place")
	}
	if &(*stateValue.itemsPointer)[:1][0] != pointedItemsStorage {
		t.Fatal("pointed-to slice backing array was not retained")
	}
	if len(*stateValue.labelsPointer) != 0 {
		t.Fatal("pointed-to map was not cleared")
	}
	if stateValue.child.value != 0 || stateValue.childPointer.value != 0 {
		t.Fatal("nested Reset method was not called")
	}
	if stateValue.childWithoutReset.value != 0 || stateValue.genericValue != "" {
		t.Fatal("nested or generic fields were not reset to zero")
	}
	if resetCalls != 3 {
		t.Fatalf("Reset was called %d times, want 3", resetCalls)
	}

	resetCalls = 0
	stateWithNilPointers := state[string]{}
	stateWithNilPointers.Reset()
	if resetCalls != 1 {
		t.Fatalf("Reset was called %d times with nil pointer fields, want 1", resetCalls)
	}

	var nilState *state[string]
	nilState.Reset()
}
`)
	writeFixtureFile(t, root, "fixture/nested/other.go", `package nested

// generate:reset
type Counter struct {
	count int
}
`)
	writeFixtureFile(t, root, "fixture/nested/other_test.go", `package nested

import "testing"

func TestGeneratedReset(t *testing.T) {
	counter := Counter{count: 5}
	counter.Reset()
	if counter.count != 0 {
		t.Fatalf("count = %d, want 0", counter.count)
	}
}
`)

	if err := generate(root); err != nil {
		t.Fatalf("generate reset methods: %v", err)
	}

	outputPath := filepath.Join(root, "fixture", generatedFileName)
	firstGeneration, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if !bytes.Contains(firstGeneration, []byte("func (r *state[T]) Reset()")) {
		t.Fatal("generated file does not contain the generic Reset method")
	}
	if err := generate(root); err != nil {
		t.Fatalf("regenerate reset methods: %v", err)
	}
	secondGeneration, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated file after second generation: %v", err)
	}
	if !bytes.Equal(firstGeneration, secondGeneration) {
		t.Fatal("generated file changed on the second run")
	}

	command := exec.Command("go", "test", "./...")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("test generated packages: %v\n%s", err, output)
	}
}

func writeFixtureFile(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}
