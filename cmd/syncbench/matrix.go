package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Matrix is bench/matrix.json: the windows, the cpusets and the cell sets.
type Matrix struct {
	Windows struct {
		WarmupS     float64 `json:"warmup_s"`
		ThroughputS float64 `json:"throughput_s"`
		LatencyS    float64 `json:"latency_s"`
	} `json:"windows"`
	Cpusets map[string]string  `json:"cpusets"`
	Sets    map[string]CellSet `json:"sets"`
}

// CellSet is a named list of cells: generative blocks plus explicit cells.
type CellSet struct {
	Rounds int     `json:"rounds"`
	Blocks []Block `json:"blocks"`
	Cells  []Cell  `json:"cells"`
}

// Block is the cartesian product of its axes.
type Block struct {
	Scenario string   `json:"scenario"`
	Impls    []string `json:"impls"`
	G        []int    `json:"G"`
	P        []int    `json:"P"`
	W        []int64  `json:"W"`
	Read     []int    `json:"read"` // S2 only
}

// Cell is one point of the matrix.
type Cell struct {
	Scenario string `json:"scenario"`
	Impl     string `json:"impl"`
	G        int    `json:"G"`
	P        int    `json:"P"`
	W        int64  `json:"W"`
	Read     int    `json:"read,omitempty"`
	Note     string `json:"note,omitempty"`
}

// ID is the stable cell identifier, also the raw file name stem.
func (c Cell) ID() string {
	id := c.Scenario + "-" + c.Impl
	if c.Scenario == "s2" {
		id += "-r" + strconv.Itoa(c.Read)
	}
	return fmt.Sprintf("%s-g%d-p%d-w%d", id, c.G, c.P, c.W)
}

func loadMatrix(path string) (*Matrix, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Matrix
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Expand returns the cells of set name, deduplicated by ID, in a stable order.
func (m *Matrix) Expand(name string) ([]Cell, error) {
	set, ok := m.Sets[name]
	if !ok {
		return nil, fmt.Errorf("matrix has no set %q", name)
	}
	var out []Cell
	seen := map[string]bool{}
	add := func(c Cell) {
		if id := c.ID(); !seen[id] {
			seen[id] = true
			out = append(out, c)
		}
	}
	for _, b := range set.Blocks {
		reads := b.Read
		if b.Scenario != "s2" || len(reads) == 0 {
			reads = []int{0}
		}
		for _, impl := range b.Impls {
			for _, r := range reads {
				for _, g := range b.G {
					for _, p := range b.P {
						for _, w := range b.W {
							add(Cell{Scenario: b.Scenario, Impl: impl, G: g, P: p, W: w, Read: r})
						}
					}
				}
			}
		}
	}
	for _, c := range set.Cells {
		add(c)
	}
	return out, nil
}

// ReadCellIDs parses a cell list: one cell id per line; blank lines and lines
// starting with '#' are ignored. A repeated id or an empty list is an error.
func ReadCellIDs(r io.Reader) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t") {
			return nil, fmt.Errorf("line %d: %q is not a single cell id", n, line)
		}
		if seen[line] {
			return nil, fmt.Errorf("line %d: cell %s listed twice", n, line)
		}
		seen[line] = true
		ids = append(ids, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("cell list is empty")
	}
	return ids, nil
}

// Subset keeps the cells whose id is in ids, in the order of cells. Every id
// must name a cell of the set: a typo must not silently shrink a rerun.
func Subset(cells []Cell, ids []string) ([]Cell, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Cell
	for _, c := range cells {
		if want[c.ID()] {
			out = append(out, c)
			delete(want, c.ID())
		}
	}
	if len(want) > 0 {
		var missing []string
		for id := range want {
			missing = append(missing, id)
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("%d cell id(s) not in the set: %s", len(missing), strings.Join(missing, " "))
	}
	return out, nil
}

// roundRNG derives the generator of one round from the run seed.
func roundRNG(seed uint64, round int, stream uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed^uint64(round)*0x9e3779b97f4a7c15, stream))
}

// POrder is level (a) of the two-level shuffle: the order of the P groups.
func POrder(ps []int, seed uint64, round int) []int {
	out := slices.Clone(ps)
	slices.Sort(out)
	r := roundRNG(seed, round, 1)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// CellOrder is level (b): the order of the cells inside one P group.
func CellOrder(cells []Cell, seed uint64, round, p int) []Cell {
	var out []Cell
	for _, c := range cells {
		if c.P == p {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Cell) int {
		switch {
		case a.ID() < b.ID():
			return -1
		case a.ID() > b.ID():
			return 1
		}
		return 0
	})
	r := roundRNG(seed, round, 100+uint64(p))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// repSeed is the PCG seed of one repetition, derived from the run seed, the
// round and the cell id (FNV-1a).
func repSeed(seed uint64, round int, id string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211
	}
	return h ^ seed ^ uint64(round)<<56
}

// Ps returns the distinct P values of cells, sorted.
func Ps(cells []Cell) []int {
	var ps []int
	for _, c := range cells {
		if !slices.Contains(ps, c.P) {
			ps = append(ps, c.P)
		}
	}
	slices.Sort(ps)
	return ps
}
