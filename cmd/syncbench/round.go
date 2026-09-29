package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/muhammetsafak/go-sync-bench/internal/measure"
)

// Exit codes of the selftest subcommand.
const (
	exitSelfTestFail     = 5
	exitSelfTestUnstable = 6
)

// exitHarnessError is the exit code of a round that stopped because one
// repetition failed a hard gate (G1 or G3) on both attempts.
const exitHarnessError = 9

// repBudget is the wall time a repetition is expected to stay under; longer
// ones are reported (Faz 01 open item 3).
const repBudget = 5 * time.Second

// repTimeout kills a repetition subprocess that never returns: the three
// windows, two drain timeouts and generous slack.
func repTimeout(m *Matrix) time.Duration {
	w := m.Windows.WarmupS + m.Windows.ThroughputS + m.Windows.LatencyS
	return time.Duration(w*float64(time.Second)) + 3*10*time.Second + 30*time.Second
}

func cmdSelfTest(args []string) int {
	fs := flag.NewFlagSet("selftest", flag.ContinueOnError)
	out := fs.String("out", "", "output file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r := measure.SelfTest()
	if err := writeJSON(*out, r); err != nil {
		fmt.Fprintln(os.Stderr, "syncbench selftest:", err)
		return 1
	}
	switch r.Result {
	case "pass":
		return 0
	case "setup_unstable":
		return exitSelfTestUnstable
	}
	return exitSelfTestFail
}

type setFlags struct {
	matrix, set string
	cells       string // optional cell list restricting the set (a rerun)
	seed        uint64
	round       int
}

func (s *setFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&s.matrix, "matrix", "/app/matrix.json", "matrix file")
	fs.StringVar(&s.set, "set", "main", "matrix set")
	fs.Uint64Var(&s.seed, "seed", 1, "run seed")
	fs.IntVar(&s.round, "round", 1, "round number")
	fs.StringVar(&s.cells, "cells", "", "file of cell ids (one per line, '#' comments) restricting the set")
}

func (s *setFlags) load() (*Matrix, []Cell, error) {
	m, err := loadMatrix(s.matrix)
	if err != nil {
		return nil, nil, err
	}
	cells, err := m.Expand(s.set)
	if err != nil || s.cells == "" {
		return m, cells, err
	}
	f, err := os.Open(s.cells)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	ids, err := ReadCellIDs(f)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", s.cells, err)
	}
	cells, err = Subset(cells, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", s.cells, err)
	}
	return m, cells, nil
}

func cmdCells(args []string) int {
	var s setFlags
	fs := flag.NewFlagSet("cells", flag.ContinueOnError)
	s.register(fs)
	list := fs.Bool("list", false, "print cell ids instead of the count")
	ps := fs.Bool("ps", false, "print the distinct P values")
	rounds := fs.Bool("rounds", false, "print the round count of the set")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	m, cells, err := s.load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncbench cells:", err)
		return 1
	}
	switch {
	case *rounds:
		fmt.Println(m.Sets[s.set].Rounds)
	case *ps:
		fmt.Println(joinInts(Ps(cells)))
	case *list:
		for _, c := range cells {
			fmt.Println(c.ID())
		}
	default:
		fmt.Println(len(cells))
	}
	return 0
}

// cmdOrder prints the two-level shuffle of one round: first line the P order,
// then one line per P group "P=<p>: id id ...".
func cmdOrder(args []string) int {
	var s setFlags
	fs := flag.NewFlagSet("order", flag.ContinueOnError)
	s.register(fs)
	pOnly := fs.Bool("p-only", false, "print only the P order")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_, cells, err := s.load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncbench order:", err)
		return 1
	}
	po := POrder(Ps(cells), s.seed, s.round)
	fmt.Println(joinInts(po))
	if *pOnly {
		return 0
	}
	for _, p := range po {
		var ids []string
		for _, c := range CellOrder(cells, s.seed, s.round, p) {
			ids = append(ids, c.ID())
		}
		fmt.Printf("P=%d: %s\n", p, strings.Join(ids, " "))
	}
	return 0
}

func joinInts(xs []int) string {
	var b strings.Builder
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Itoa(x))
	}
	return b.String()
}

// timing is one line of timing/r<round>-P<p>.jsonl.
type timing struct {
	CellID   string  `json:"cell_id"`
	Round    int     `json:"round"`
	Attempt  int     `json:"attempt"`
	WallS    float64 `json:"wall_s"`
	Exit     int     `json:"exit"`
	Valid    bool    `json:"valid"`
	OverBudg bool    `json:"over_budget"`
	Skipped  bool    `json:"skipped,omitempty"`
}

// cmdRound runs one round of one P group inside its container: the self-test
// once, then every cell of the group in the shuffled order, each repetition in
// its own process.
func cmdRound(args []string) int {
	var s setFlags
	fs := flag.NewFlagSet("round", flag.ContinueOnError)
	s.register(fs)
	p := fs.Int("P", 0, "P group to run (required)")
	outDir := fs.String("out", "/results", "result directory")
	cpuset := fs.String("cpuset", "", "cpuset string (recorded only)")
	resume := fs.Bool("resume", false, "skip repetitions whose raw file exists")
	rerunOfDir := fs.String("rerun-of-dir", "", "in a cell rerun, the first-pass raw directory relative to -out/raw; "+
		"attempt 1 of every repetition records rerun_of = <dir>/<id>.r<round>")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *p <= 0 {
		fmt.Fprintln(os.Stderr, "syncbench round: -P is required")
		return 2
	}
	m, cells, err := s.load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncbench round:", err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncbench round:", err)
		return 1
	}
	tag := fmt.Sprintf("r%d-P%d", s.round, *p)
	for _, d := range []string{"raw", "selftest", "order", "timing"} {
		if err := os.MkdirAll(filepath.Join(*outDir, d), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "syncbench round:", err)
			return 1
		}
	}
	logf := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "[%s %s] %s\n", time.Now().UTC().Format("15:04:05"), tag, fmt.Sprintf(format, a...))
	}

	// Self-test once per container, GOMAXPROCS = 2 as the design requires.
	stPath := filepath.Join(*outDir, "selftest", tag+".json")
	stCode := runChild(self, 2, time.Minute, "selftest", "-out", stPath)
	stResult := map[int]string{0: "pass", exitSelfTestFail: "fail", exitSelfTestUnstable: "setup_unstable"}[stCode]
	if stResult == "" {
		stResult = "error"
	}
	logf("selftest: %s", stResult)

	order := CellOrder(cells, s.seed, s.round, *p)
	var ids []string
	for _, c := range order {
		ids = append(ids, c.ID())
	}
	if err := os.WriteFile(filepath.Join(*outDir, "order", tag+".txt"),
		[]byte(fmt.Sprintf("seed=%d round=%d P=%d\n%s\n", s.seed, s.round, *p, strings.Join(ids, "\n"))), 0o644); err != nil {
		logf("order file: %v", err)
	}

	tf, err := os.OpenFile(filepath.Join(*outDir, "timing", tag+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "syncbench round:", err)
		return 1
	}
	defer tf.Close()
	tw := bufio.NewWriter(tf)
	defer tw.Flush()
	writeTiming := func(t timing) {
		b, _ := json.Marshal(t)
		tw.Write(append(b, '\n'))
		tw.Flush()
	}

	timeout := repTimeout(m)
	win := m.Windows
	rawDir := filepath.Join(*outDir, "raw")
	for i, c := range order {
		id := c.ID()
		run := func(target string, attempt int, rerunOf string) int {
			seed := repSeed(s.seed, s.round, id)
			if attempt == 2 {
				seed ^= 0xa5a5a5a5a5a5a5a5
			}
			argv := []string{"run",
				"-scenario", c.Scenario, "-impl", c.Impl,
				"-G", strconv.Itoa(c.G), "-P", strconv.Itoa(c.P),
				"-W", strconv.FormatInt(c.W, 10), "-read", strconv.Itoa(c.Read),
				"-round", strconv.Itoa(s.round), "-cell-id", id,
				"-rep-seed", strconv.FormatUint(seed, 10),
				"-order-seed", strconv.FormatUint(s.seed, 10),
				"-cpuset", *cpuset, "-set", s.set,
				"-selftest-result", stResult,
				"-starving=" + strconv.FormatBool(stResult == "pass"),
				"-warmup", secs(win.WarmupS), "-window", secs(win.ThroughputS),
				"-latency-window", secs(win.LatencyS),
				"-out", target,
			}
			if rerunOf != "" {
				argv = append(argv, "-rerun-of", rerunOf)
			}
			t0 := time.Now()
			code := runChild(self, c.P, timeout, argv...)
			wall := time.Since(t0)
			valid := readValid(target)
			writeTiming(timing{CellID: id, Round: s.round, Attempt: attempt, WallS: wall.Seconds(),
				Exit: code, Valid: valid, OverBudg: wall > repBudget})
			logf("%d/%d %s attempt=%d exit=%d valid=%t wall=%.2fs", i+1, len(order), id, attempt, code, valid, wall.Seconds())
			return code
		}
		supersedes := ""
		if *rerunOfDir != "" {
			supersedes = path.Join(*rerunOfDir, fmt.Sprintf("%s.r%d", id, s.round))
		}
		out, err := runRepetition(rawDir, id, s.round, *resume, supersedes, run, readValid)
		switch {
		case err != nil:
			logf("%s: %v", id, err)
			return 1
		case out == repSkipped:
			writeTiming(timing{CellID: id, Round: s.round, Skipped: true, Valid: true})
		case out == repG2:
			logf("G2 cpuset gate failed; stopping the round")
			return exitG2
		case out == repHarnessError:
			// The design's gate policy: a repetition that fails a hard gate
			// twice is a harness error and the run stops for the user.
			logf("%s failed a hard gate twice: harness error, stopping the round", id)
			return exitHarnessError
		}
	}
	return 0
}

func secs(s float64) string {
	return time.Duration(s * float64(time.Second)).String()
}

// runChild re-executes this binary with GOMAXPROCS=procs and returns the exit
// code; -1 means it could not start or was killed on timeout.
func runChild(self string, procs int, timeout time.Duration, args ...string) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, args...)
	cmd.Env = append(filterEnv(os.Environ(), "GOMAXPROCS="), "GOMAXPROCS="+strconv.Itoa(procs))
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee) && ee.ExitCode() >= 0:
		return ee.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "syncbench: child %v: %v\n", args[:1], err)
	return -1
}

func filterEnv(env []string, prefix string) []string {
	out := env[:0:0]
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// readValid reports whether the raw file exists and passed G1–G3.
func readValid(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var r struct {
		Gates HardGates `json:"gates"`
	}
	if json.Unmarshal(b, &r) != nil {
		return false
	}
	return r.Gates.Valid
}
