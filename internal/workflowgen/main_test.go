package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const workflowsDir = "../../.github/workflows"

// TestWorkflowsAreGenerated fails when a committed bantool workflow is not
// what the table generates, so a hand edit or a stale file fails CI.
func TestWorkflowsAreGenerated(t *testing.T) {
	dir := t.TempDir()
	err := write(dir)
	if err != nil {
		t.Fatal(err)
	}

	generated, err := filepath.Glob(filepath.Join(dir, "bantool-*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range generated {
		name := filepath.Base(path)
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(workflowsDir, name))
		if err != nil {
			t.Errorf("%s is in the table but not committed; run go generate ./internal/workflowgen", name)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from what the table generates; edit internal/workflowgen/targets.go and run go generate ./internal/workflowgen", name)
		}
	}

	committed, err := filepath.Glob(filepath.Join(workflowsDir, "bantool-*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(committed) != len(generated) {
		t.Errorf("%d bantool workflows are committed, the table generates %d; run go generate ./internal/workflowgen", len(committed), len(generated))
	}
}

// TestTargetsAreInFileNameOrder keeps the table in the order the files list
// in, so a new row has one place to go.
func TestTargetsAreInFileNameOrder(t *testing.T) {
	sorted := slices.IsSortedFunc(targets, func(a, b workflow) int {
		return strings.Compare(a.FileName(), b.FileName())
	})
	if !sorted {
		t.Error("targets is not sorted by file name")
	}
}
