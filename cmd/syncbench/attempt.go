package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// repOutcome is how one repetition of a cell ended.
type repOutcome int

const (
	repValid        repOutcome = iota // a valid raw file is at its final path
	repSkipped                        // --resume found a valid raw file
	repHarnessError                   // both attempts failed a hard gate
	repG2                             // the cpuset gate failed: stop the round
)

// repFunc runs one attempt, writing its raw record to target, and returns the
// child's exit code. rerunOf names the evidence file of a failed first
// attempt, empty on the first attempt.
type repFunc func(target string, attempt int, rerunOf string) int

// runRepetition applies the hard-gate policy of the design to one repetition:
//
//   - attempt 1 writes <id>.r<n>.json.attempt; a valid record is renamed to
//     its final path <id>.r<n>.json
//   - a record that failed G1 or G3 (or none at all) is kept as evidence under
//     <id>.r<n>.invalid.json and the repetition is run once more with a new
//     seed, rerun_of pointing at the evidence
//   - a second failure stays at the final path with gates.valid = false and is
//     reported as a harness error: the caller stops the run
//
// With resume, a valid final record is skipped. An invalid one (the harness
// error of an interrupted run) is moved to the next free evidence name and the
// repetition is run again from attempt 1, so evidence is never overwritten.
func runRepetition(rawDir, id string, round int, resume bool, run repFunc, valid func(string) bool) (repOutcome, error) {
	path := filepath.Join(rawDir, fmt.Sprintf("%s.r%d.json", id, round))
	if resume {
		if _, err := os.Stat(path); err == nil {
			if valid(path) {
				return repSkipped, nil
			}
			ev, err := evidencePath(rawDir, id, round)
			if err != nil {
				return 0, err
			}
			if err := os.Rename(path, ev); err != nil {
				return 0, err
			}
		}
	}
	attempt := path + ".attempt"
	if code := run(attempt, 1, ""); code == exitG2 {
		return repG2, nil
	}
	if valid(attempt) {
		return repValid, os.Rename(attempt, path)
	}
	ev, err := evidencePath(rawDir, id, round)
	if err != nil {
		return 0, err
	}
	// A child killed on timeout may have left no record. A placeholder then
	// holds the evidence name, so rerun_of never points at a missing file and
	// a later resume cannot give that name to another record.
	if err := os.Rename(attempt, ev); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return 0, err
		}
		if err := os.WriteFile(ev, missingRecord(id, round), 0o644); err != nil {
			return 0, err
		}
	}
	rerunOf := strings.TrimSuffix(filepath.Base(ev), ".json")
	if code := run(path, 2, rerunOf); code == exitG2 {
		return repG2, nil
	}
	if valid(path) {
		return repValid, nil
	}
	return repHarnessError, nil
}

// evidencePath returns the first free name among <id>.r<n>.invalid.json,
// <id>.r<n>.invalid-2.json, <id>.r<n>.invalid-3.json, ...
func evidencePath(rawDir, id string, round int) (string, error) {
	for k := 1; k < 1000; k++ {
		name := fmt.Sprintf("%s.r%d.invalid.json", id, round)
		if k > 1 {
			name = fmt.Sprintf("%s.r%d.invalid-%d.json", id, round, k)
		}
		p := filepath.Join(rawDir, name)
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no free evidence name for %s round %d", id, round)
}

// missingRecord is the placeholder kept as evidence when attempt 1 of a
// repetition left no record (killed on timeout or died before writing). The
// timing line of that attempt has the exit code and the wall time.
func missingRecord(id string, round int) []byte {
	b, _ := json.Marshal(struct {
		CellID  string          `json:"cell_id"`
		Round   int             `json:"round"`
		Missing string          `json:"missing"`
		Gates   map[string]bool `json:"gates"`
	}{id, round, "attempt 1 left no record; see timing", map[string]bool{"valid": false}})
	return append(b, '\n')
}
