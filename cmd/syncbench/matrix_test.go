package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadCellIDs(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
		err      string
	}{
		{name: "ids, comments and blanks", in: "# rerun\n\na-x-g1-p1-w0\n  b-y-g2-p2-w0  \n# end\n",
			want: []string{"a-x-g1-p1-w0", "b-y-g2-p2-w0"}},
		{name: "no trailing newline", in: "a-x-g1-p1-w0", want: []string{"a-x-g1-p1-w0"}},
		{name: "empty list", in: "# nothing\n\n", err: "empty"},
		{name: "repeated id", in: "a\nb\na\n", err: "listed twice"},
		{name: "two ids on one line", in: "a b\n", err: "not a single cell id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ReadCellIDs(strings.NewReader(c.in))
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("err = %v, want one containing %q", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSubset(t *testing.T) {
	cells := []Cell{
		{Scenario: "s1", Impl: "mutex", G: 1, P: 1},
		{Scenario: "s1", Impl: "chan", G: 4, P: 2},
		{Scenario: "s2", Impl: "rwmutex", Read: 90, G: 8, P: 4},
	}
	got, err := Subset(cells, []string{"s2-rwmutex-r90-g8-p4-w0", "s1-mutex-g1-p1-w0"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range got {
		ids = append(ids, c.ID())
	}
	// The set's order is kept, not the list's.
	if want := []string{"s1-mutex-g1-p1-w0", "s2-rwmutex-r90-g8-p4-w0"}; !slices.Equal(ids, want) {
		t.Fatalf("got %q, want %q", ids, want)
	}

	if _, err := Subset(cells, []string{"s1-mutex-g1-p1-w0", "s1-mutex-g9-p1-w0"}); err == nil ||
		!strings.Contains(err.Error(), "s1-mutex-g9-p1-w0") {
		t.Fatalf("unknown id: err = %v, want it named", err)
	}
}

// TestSetFlagsCells checks that -cells restricts every command that loads the set.
func TestSetFlagsCells(t *testing.T) {
	dir := t.TempDir()
	matrix := filepath.Join(dir, "matrix.json")
	if err := os.WriteFile(matrix, []byte(`{"sets":{"main":{"cells":[
		{"scenario":"s1","impl":"mutex","G":1,"P":1,"W":0},
		{"scenario":"s1","impl":"chan","G":1,"P":1,"W":0},
		{"scenario":"s1","impl":"chan","G":4,"P":2,"W":0}]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "cells.txt")
	if err := os.WriteFile(list, []byte("s1-chan-g4-p2-w0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := setFlags{matrix: matrix, set: "main"}
	if _, all, err := s.load(); err != nil || len(all) != 3 {
		t.Fatalf("without -cells: %d cells, err %v; want 3", len(all), err)
	}
	s.cells = list
	_, sub, err := s.load()
	if err != nil || len(sub) != 1 || sub[0].ID() != "s1-chan-g4-p2-w0" {
		t.Fatalf("with -cells: %v, err %v", sub, err)
	}

	if err := os.WriteFile(list, []byte("s1-chan-g4-p8-w0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.load(); err == nil {
		t.Fatal("an id outside the set must fail the load")
	}
}
