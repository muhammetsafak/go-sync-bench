package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeRun writes a record whose content is "ok" or "bad" per attempt, or no
// record at all for "none", and returns the given exit code.
type fakeRun struct {
	results []string
	codes   []int
	calls   []string // rerunOf per call
}

func (f *fakeRun) run(target string, attempt int, rerunOf string) int {
	i := len(f.calls)
	f.calls = append(f.calls, rerunOf)
	if f.results[i] != "none" {
		os.WriteFile(target, []byte(f.results[i]), 0o644)
	}
	if i < len(f.codes) {
		return f.codes[i]
	}
	return 0
}

func fakeValid(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && string(b) == "ok"
}

func listDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, e := range es {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		m[e.Name()] = string(b)
	}
	return m
}

var placeholder = string(missingRecord("c", 1))

func TestRunRepetition(t *testing.T) {
	cases := []struct {
		name    string
		pre     map[string]string // files present before the call
		resume  bool
		results []string
		codes   []int
		want    repOutcome
		files   map[string]string
		rerun   []string
	}{
		{
			name: "valid first attempt", results: []string{"ok"}, want: repValid,
			files: map[string]string{"c.r1.json": "ok"}, rerun: []string{""},
		},
		{
			name: "invalid then valid", results: []string{"bad", "ok"}, want: repValid,
			files: map[string]string{"c.r1.invalid.json": "bad", "c.r1.json": "ok"},
			rerun: []string{"", "c.r1.invalid"},
		},
		{
			name: "invalid twice is a harness error", results: []string{"bad", "bad"}, want: repHarnessError,
			files: map[string]string{"c.r1.invalid.json": "bad", "c.r1.json": "bad"},
			rerun: []string{"", "c.r1.invalid"},
		},
		{
			name: "no record (timeout) then valid", results: []string{"none", "ok"}, codes: []int{-1, 0}, want: repValid,
			files: map[string]string{"c.r1.invalid.json": placeholder, "c.r1.json": "ok"},
			rerun: []string{"", "c.r1.invalid"},
		},
		{
			// rerun_of of the second attempt must keep pointing at the
			// placeholder after a resume moves that attempt's record away.
			name: "resume after timeout and invalid rerun keeps the chain",
			pre:  map[string]string{"c.r1.invalid.json": placeholder, "c.r1.json": "bad"}, resume: true,
			results: []string{"ok"}, want: repValid,
			files: map[string]string{
				"c.r1.invalid.json": placeholder, "c.r1.invalid-2.json": "bad", "c.r1.json": "ok",
			},
			rerun: []string{""},
		},
		{
			name: "G2 stops at once", results: []string{"bad"}, codes: []int{exitG2}, want: repG2,
			files: map[string]string{"c.r1.json.attempt": "bad"}, rerun: []string{""},
		},
		{
			name: "resume skips a valid record", pre: map[string]string{"c.r1.json": "ok"}, resume: true,
			want: repSkipped, files: map[string]string{"c.r1.json": "ok"},
		},
		{
			name: "resume re-runs an invalid record and keeps all evidence",
			pre:  map[string]string{"c.r1.invalid.json": "bad-a1", "c.r1.json": "bad-a2"}, resume: true,
			results: []string{"bad", "ok"}, want: repValid,
			files: map[string]string{
				"c.r1.invalid.json": "bad-a1", "c.r1.invalid-2.json": "bad-a2",
				"c.r1.invalid-3.json": "bad", "c.r1.json": "ok",
			},
			rerun: []string{"", "c.r1.invalid-3"},
		},
		{
			name: "without resume an existing record is replaced", pre: map[string]string{"c.r1.json": "bad"},
			results: []string{"ok"}, want: repValid,
			files: map[string]string{"c.r1.json": "ok"}, rerun: []string{""},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, v := range c.pre {
				os.WriteFile(filepath.Join(dir, n), []byte(v), 0o644)
			}
			f := &fakeRun{results: c.results, codes: c.codes}
			got, err := runRepetition(dir, "c", 1, c.resume, f.run, fakeValid)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("outcome %d, want %d", got, c.want)
			}
			files := listDir(t, dir)
			if len(files) != len(c.files) {
				t.Errorf("files %v, want %v", files, c.files)
			}
			for n, v := range c.files {
				if files[n] != v {
					t.Errorf("%s = %q, want %q (all: %v)", n, files[n], v, files)
				}
			}
			if len(f.calls) != len(c.rerun) {
				t.Fatalf("calls %q, want %q", f.calls, c.rerun)
			}
			for i := range c.rerun {
				if f.calls[i] != c.rerun[i] {
					t.Errorf("call %d rerunOf %q, want %q", i, f.calls[i], c.rerun[i])
				}
			}
		})
	}
}
