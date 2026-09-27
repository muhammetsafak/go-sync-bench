// Command syncbench is the harness of the go-sync-bench experiment: sync.Mutex,
// sync.RWMutex, atomics and channels under goroutine contention.
//
// Subcommands:
//
//	run        one repetition of one cell, JSON to --out
//	round      one round of one P group, every repetition in its own process
//	selftest   the starving-bit self-test (alias: --selftest)
//	calibrate  distributions that check the 10 µs park threshold
//	order      the shuffled P order and cell order of a round
//	cells      the number (or list) of cells of a matrix set
package main

import (
	"fmt"
	"os"
	"runtime"
)

const usage = `usage: syncbench <run|round|selftest|calibrate|order|cells|version> [flags]
run "syncbench <subcommand> -h" for the flags of a subcommand`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	args := os.Args[2:]
	var code int
	switch os.Args[1] {
	case "run":
		code = cmdRun(args)
	case "round":
		code = cmdRound(args)
	case "selftest", "--selftest":
		code = cmdSelfTest(args)
	case "calibrate":
		code = cmdCalibrate(args)
	case "order":
		code = cmdOrder(args)
	case "cells":
		code = cmdCells(args)
	case "version":
		fmt.Println(runtime.Version())
	default:
		fmt.Fprintln(os.Stderr, usage)
		code = 2
	}
	os.Exit(code)
}
