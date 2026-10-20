# Run 2026-10-19 — cell rerun

- rerun of: `results/2026-10-19` (first pass, read only); list: `cells.txt`; links: `rerun_of.json`
- set: `main` (73 cells x 5 rounds)
- seed: `3859364050`
- image: `go-sync-bench` (sha256:d324581ae7e75db64f1a175f8942432b0f62d64aa6fe632dde4532ccf3ab0617)
- go: go1.27.1
- profile: `linux-metal` (isolated 8-15, system 0-7)
- host: AMD Ryzen 9 5950X 16-Core Processor, 16 CPUs online; kernel 7.0.0-38-generic; docker 29.9.1
- P groups: 1 2 4 8; cpusets: {"1":"8","2":"8-9","4":"8-11","8":"8-15"}
- cool-down: 60s after every container; E1 enforced: 1 (limit 100% for up to 600s)
- E2: host-check start: passed; temperature Tctl 40.375, Tccd1 40, Tccd2 32.25; enforced: 1; records `host-check/`, `environment.json`
- started: 2026-10-19T22:33:32Z

## Round 1

```
host-check round-1: passed; temperature Tctl 42.125, Tccd1 41.75, Tccd2 32.25
sleep: see host-check
P order: 8 4 2 1
P=8: s1-atomic-g4-p8-w0 s2-mutex-r90-g10000-p8-w0 s3-cond1024-g10000-p8-w1000 s3-chan0-g10000-p8-w1000 s1-mutex-g64-p8-w0 s2-mutex-r50-g1024-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s3-cond64-g1024-p8-w0 s3-cond64-g10000-p8-w0 s1-mutex-g256-p8-w0 s3-chan1024-g10000-p8-w1000 s2-mutex-r50-g10000-p8-w0 s2-mutex-r50-g64-p8-w0 s1-chan_oneway-g1-p8-w100 s3-chan64-g10000-p8-w1000 s1-rwmutex-g10000-p8-w100 s1-mutex-g10000-p8-w0 s1-mutex-g1024-p8-w0 s3-chan0-g1024-p8-w1000 s3-chan1024-g64-p8-w1000 s3-chan1-g10000-p8-w1000
P=4: s3-chan64-g10000-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s3-cond1024-g64-p4-w0 s2-mutex-r90-g64-p4-w0 s1-mutex-g1024-p4-w0 s1-mutex-g16-p4-w0 s2-mutex-r50-g1024-p4-w0 s3-chan0-g1024-p4-w0 s3-cond64-g64-p4-w0 s1-chan_oneway-g1-p4-w100
P=2: s1-rwmutex-g4-p2-w100 s1-mutex-g256-p2-w0 s3-cond1024-g1024-p2-w0 s1-mutex-g1024-p2-w100 s3-cond64-g10000-p2-w0 s3-chan1-g4-p2-w1000 s3-chan1-g10000-p2-w1000 s3-cond1024-g64-p2-w1000 s1-rwmutex-g16-p2-w0 s1-mutex-g10000-p2-w0 s1-mutex-g16-p2-w0 s3-chan0-g10000-p2-w0 s1-chan_oneway-g16-p2-w0 s1-chan_oneway-g1024-p2-w100 s2-rwmutex-r50-g10000-p2-w0 s1-rwmutex-g16-p2-w1000 s1-rwmutex-g1024-p2-w100 s3-chan1024-g1024-p2-w1000 s3-cond64-g64-p2-w0 s1-mutex-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w0 s3-cond1024-g10000-p2-w1000 s2-rwmutex-r50-g4-p2-w1000 s1-chan_reqresp-g16-p2-w100 s1-mutex-g4-p2-w100 s3-cond1024-g64-p2-w0 s1-mutex-g16-p2-w1000 s1-rwmutex-g10000-p2-w100 s1-chan_reqresp-g1024-p2-w100 s3-chan0-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s1-mutex-g1024-p2-w0 s2-rwmutex-r90-g10000-p2-w1000
P=1: s1-mutex-g1024-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s1-mutex-g16-p1-w0 s2-mutex-r90-g1024-p1-w0 s1-chan_reqresp-g1024-p1-w1000 s1-chan_reqresp-g1024-p1-w100 s1-rwmutex-g1024-p1-w0 s3-cond64-g10000-p1-w0 s2-rwmutex-r99-g1024-p1-w1000
```

- round 1 P=8: exit 0, 93 s
  - E1: host load 0.3% (loadavg 2.05 2.16 2.30), extra wait 0s
- round 1 P=4: exit 0, 44 s
  - E1: host load 0.3% (loadavg 0.80 1.79 2.16), extra wait 0s
- round 1 P=2: exit 0, 147 s
  - E1: host load 0.0% (loadavg 0.62 1.49 1.98), extra wait 0s
- round 1 P=1: exit 0, 40 s
  - E1: host load 0.0% (loadavg 0.27 1.15 1.80), extra wait 0s
## Round 2

```
host-check round-2: passed; temperature Tctl 38.75, Tccd1 39, Tccd2 31.75
sleep: see host-check
P order: 1 8 4 2
P=1: s1-mutex-g16-p1-w0 s3-cond64-g10000-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s1-chan_reqresp-g1024-p1-w1000 s1-chan_reqresp-g1024-p1-w100 s1-mutex-g1024-p1-w0 s1-rwmutex-g1024-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s2-mutex-r90-g1024-p1-w0
P=8: s2-mutex-r90-g10000-p8-w0 s1-mutex-g10000-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s2-mutex-r50-g10000-p8-w0 s3-chan1024-g10000-p8-w1000 s1-chan_oneway-g1-p8-w100 s2-mutex-r50-g64-p8-w0 s1-rwmutex-g10000-p8-w100 s3-chan0-g10000-p8-w1000 s3-cond64-g1024-p8-w0 s3-chan0-g1024-p8-w1000 s3-cond1024-g10000-p8-w1000 s1-atomic-g4-p8-w0 s1-mutex-g1024-p8-w0 s1-mutex-g256-p8-w0 s3-chan64-g10000-p8-w1000 s3-chan1-g10000-p8-w1000 s3-chan1024-g64-p8-w1000 s1-mutex-g64-p8-w0 s2-mutex-r50-g1024-p8-w0 s3-cond64-g10000-p8-w0
P=4: s1-mutex-g16-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s3-cond1024-g64-p4-w0 s2-mutex-r50-g1024-p4-w0 s3-cond64-g64-p4-w0 s1-mutex-g1024-p4-w0 s2-mutex-r90-g64-p4-w0 s3-chan64-g10000-p4-w0 s1-chan_oneway-g1-p4-w100 s3-chan0-g1024-p4-w0
P=2: s2-rwmutex-r99-g1024-p2-w0 s2-rwmutex-r50-g10000-p2-w0 s1-mutex-g1024-p2-w100 s1-mutex-g10000-p2-w0 s1-chan_oneway-g1024-p2-w100 s1-mutex-g64-p2-w0 s2-rwmutex-r50-g4-p2-w1000 s1-chan_oneway-g16-p2-w0 s1-chan_reqresp-g16-p2-w100 s3-cond1024-g64-p2-w1000 s1-mutex-g4-p2-w100 s3-cond1024-g1024-p2-w0 s1-mutex-g1024-p2-w0 s3-cond1024-g10000-p2-w1000 s3-chan0-g10000-p2-w0 s3-cond1024-g64-p2-w0 s1-rwmutex-g10000-p2-w100 s2-rwmutex-r90-g10000-p2-w1000 s3-chan1024-g1024-p2-w1000 s3-chan1-g10000-p2-w1000 s1-chan_reqresp-g1024-p2-w100 s1-rwmutex-g16-p2-w0 s3-chan1-g4-p2-w1000 s3-cond64-g64-p2-w0 s1-mutex-g16-p2-w0 s3-cond64-g10000-p2-w0 s1-mutex-g256-p2-w0 s1-rwmutex-g1024-p2-w100 s1-rwmutex-g16-p2-w1000 s1-mutex-g16-p2-w1000 s1-rwmutex-g4-p2-w100 s3-chan0-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w1000
```

- round 2 P=1: exit 0, 40 s
  - E1: host load 0.3% (loadavg 0.23 0.92 1.65), extra wait 0s
- round 2 P=8: exit 0, 93 s
  - E1: host load 0.0% (loadavg 1.81 1.95 1.97), extra wait 0s
- round 2 P=4: exit 0, 44 s
  - E1: host load 0.3% (loadavg 0.85 1.70 1.88), extra wait 0s
- round 2 P=2: exit 0, 145 s
  - E1: host load 0.0% (loadavg 0.64 1.48 1.78), extra wait 0s
## Round 3

```
host-check round-3: passed; temperature Tctl 38.875, Tccd1 39.25, Tccd2 31.25
sleep: see host-check
P order: 1 2 4 8
P=1: s1-mutex-g16-p1-w0 s1-chan_reqresp-g1024-p1-w100 s1-mutex-g1024-p1-w0 s1-chan_reqresp-g1024-p1-w1000 s3-cond64-g10000-p1-w0 s2-mutex-r90-g1024-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s2-rwmutex-r99-g10000-p1-w0 s1-rwmutex-g1024-p1-w0
P=2: s3-chan0-g10000-p2-w0 s1-mutex-g64-p2-w0 s3-cond1024-g64-p2-w1000 s1-rwmutex-g16-p2-w0 s1-rwmutex-g4-p2-w100 s3-chan0-g64-p2-w0 s3-cond1024-g1024-p2-w0 s1-mutex-g16-p2-w1000 s3-chan1-g4-p2-w1000 s3-cond1024-g10000-p2-w1000 s1-chan_reqresp-g1024-p2-w100 s2-rwmutex-r50-g10000-p2-w0 s3-chan1024-g1024-p2-w1000 s1-chan_oneway-g16-p2-w0 s3-cond64-g64-p2-w0 s3-cond1024-g64-p2-w0 s1-mutex-g1024-p2-w0 s3-chan1-g10000-p2-w1000 s1-mutex-g16-p2-w0 s2-rwmutex-r50-g4-p2-w1000 s1-chan_reqresp-g16-p2-w100 s3-cond64-g10000-p2-w0 s1-rwmutex-g10000-p2-w100 s1-rwmutex-g1024-p2-w100 s1-mutex-g256-p2-w0 s1-mutex-g4-p2-w100 s2-rwmutex-r90-g10000-p2-w1000 s1-rwmutex-g16-p2-w1000 s1-mutex-g10000-p2-w0 s1-mutex-g1024-p2-w100 s1-chan_oneway-g1024-p2-w100 s2-rwmutex-r99-g1024-p2-w1000 s2-rwmutex-r99-g1024-p2-w0
P=4: s3-chan0-g1024-p4-w0 s1-mutex-g1024-p4-w0 s3-chan64-g10000-p4-w0 s3-cond1024-g64-p4-w0 s3-cond64-g64-p4-w0 s1-chan_oneway-g1-p4-w100 s2-rwmutex-r99-g10000-p4-w1000 s1-mutex-g16-p4-w0 s2-mutex-r50-g1024-p4-w0 s2-mutex-r90-g64-p4-w0
P=8: s2-mutex-r50-g1024-p8-w0 s1-rwmutex-g10000-p8-w100 s3-cond64-g10000-p8-w0 s3-chan0-g10000-p8-w1000 s2-mutex-r90-g10000-p8-w0 s3-cond64-g1024-p8-w0 s3-chan1-g10000-p8-w1000 s2-mutex-r50-g64-p8-w0 s1-mutex-g10000-p8-w0 s3-chan64-g10000-p8-w1000 s1-atomic-g4-p8-w0 s3-chan1024-g10000-p8-w1000 s3-chan0-g1024-p8-w1000 s2-rwmutex-r99-g10000-p8-w1000 s1-mutex-g1024-p8-w0 s3-chan1024-g64-p8-w1000 s1-mutex-g64-p8-w0 s2-mutex-r50-g10000-p8-w0 s3-cond1024-g10000-p8-w1000 s1-mutex-g256-p8-w0 s1-chan_oneway-g1-p8-w100
```

- round 3 P=1: exit 0, 39 s
  - E1: host load 0.3% (loadavg 0.27 1.14 1.63), extra wait 0s
- round 3 P=2: exit 0, 166 s
  - E1: host load 0.0% (loadavg 0.73 1.26 1.59), extra wait 0s
- round 3 P=4: exit 0, 44 s
  - E1: host load 0.0% (loadavg 0.74 1.27 1.57), extra wait 0s
- round 3 P=8: exit 0, 94 s
  - E1: host load 0.0% (loadavg 1.96 2.27 1.96), extra wait 0s
## Round 4

```
host-check round-4: passed; temperature Tctl 39.75, Tccd1 44, Tccd2 32
sleep: see host-check
P order: 1 2 8 4
P=1: s1-mutex-g1024-p1-w0 s2-mutex-r90-g1024-p1-w0 s3-cond64-g10000-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s1-mutex-g16-p1-w0 s1-chan_reqresp-g1024-p1-w100 s2-rwmutex-r99-g10000-p1-w0 s1-chan_reqresp-g1024-p1-w1000 s1-rwmutex-g1024-p1-w0
P=2: s1-rwmutex-g1024-p2-w100 s2-rwmutex-r99-g1024-p2-w1000 s1-rwmutex-g4-p2-w100 s3-chan1-g10000-p2-w1000 s1-mutex-g4-p2-w100 s3-cond64-g10000-p2-w0 s1-mutex-g16-p2-w0 s3-cond1024-g64-p2-w1000 s1-rwmutex-g16-p2-w0 s2-rwmutex-r50-g10000-p2-w0 s2-rwmutex-r90-g10000-p2-w1000 s1-mutex-g10000-p2-w0 s2-rwmutex-r50-g4-p2-w1000 s3-chan0-g10000-p2-w0 s1-mutex-g1024-p2-w100 s3-cond1024-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w0 s1-chan_oneway-g16-p2-w0 s3-cond1024-g1024-p2-w0 s3-chan1-g4-p2-w1000 s3-cond1024-g10000-p2-w1000 s1-mutex-g64-p2-w0 s1-rwmutex-g10000-p2-w100 s3-chan0-g64-p2-w0 s1-mutex-g16-p2-w1000 s1-chan_reqresp-g1024-p2-w100 s1-chan_oneway-g1024-p2-w100 s3-chan1024-g1024-p2-w1000 s3-cond64-g64-p2-w0 s1-mutex-g1024-p2-w0 s1-chan_reqresp-g16-p2-w100 s1-rwmutex-g16-p2-w1000 s1-mutex-g256-p2-w0
P=8: s3-cond1024-g10000-p8-w1000 s3-chan1-g10000-p8-w1000 s1-rwmutex-g10000-p8-w100 s2-mutex-r50-g10000-p8-w0 s1-mutex-g10000-p8-w0 s1-mutex-g64-p8-w0 s1-chan_oneway-g1-p8-w100 s3-cond64-g10000-p8-w0 s3-cond64-g1024-p8-w0 s1-atomic-g4-p8-w0 s2-mutex-r50-g1024-p8-w0 s1-mutex-g1024-p8-w0 s3-chan1024-g64-p8-w1000 s1-mutex-g256-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s3-chan64-g10000-p8-w1000 s2-mutex-r50-g64-p8-w0 s2-mutex-r90-g10000-p8-w0 s3-chan0-g10000-p8-w1000 s3-chan0-g1024-p8-w1000 s3-chan1024-g10000-p8-w1000
P=4: s3-chan64-g10000-p4-w0 s2-mutex-r90-g64-p4-w0 s1-mutex-g16-p4-w0 s2-mutex-r50-g1024-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s1-chan_oneway-g1-p4-w100 s1-mutex-g1024-p4-w0 s3-chan0-g1024-p4-w0 s3-cond1024-g64-p4-w0 s3-cond64-g64-p4-w0
```

- round 4 P=1: exit 0, 39 s
  - E1: host load 0.3% (loadavg 0.50 1.70 1.78), extra wait 0s
- round 4 P=2: exit 0, 146 s
  - E1: host load 0.0% (loadavg 0.65 1.48 1.70), extra wait 0s
- round 4 P=8: exit 0, 92 s
  - E1: host load 0.3% (loadavg 1.87 2.28 2.02), extra wait 0s
- round 4 P=4: exit 0, 44 s
  - E1: host load 0.0% (loadavg 0.99 2.00 1.95), extra wait 0s
## Round 5

```
host-check round-5: passed; temperature Tctl 38.75, Tccd1 37.25, Tccd2 32.25
sleep: see host-check
P order: 2 4 1 8
P=2: s3-chan1024-g1024-p2-w1000 s3-chan0-g64-p2-w0 s1-rwmutex-g10000-p2-w100 s1-mutex-g1024-p2-w100 s3-chan1-g4-p2-w1000 s1-chan_reqresp-g16-p2-w100 s3-cond1024-g64-p2-w1000 s2-rwmutex-r50-g4-p2-w1000 s3-chan1-g10000-p2-w1000 s1-chan_reqresp-g1024-p2-w100 s1-rwmutex-g1024-p2-w100 s1-mutex-g10000-p2-w0 s1-mutex-g16-p2-w0 s2-rwmutex-r50-g10000-p2-w0 s1-mutex-g1024-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s1-mutex-g256-p2-w0 s1-rwmutex-g16-p2-w0 s3-cond1024-g10000-p2-w1000 s3-cond64-g10000-p2-w0 s3-chan0-g10000-p2-w0 s3-cond1024-g64-p2-w0 s1-rwmutex-g16-p2-w1000 s1-mutex-g16-p2-w1000 s1-mutex-g64-p2-w0 s3-cond1024-g1024-p2-w0 s1-mutex-g4-p2-w100 s1-chan_oneway-g16-p2-w0 s1-chan_oneway-g1024-p2-w100 s1-rwmutex-g4-p2-w100 s2-rwmutex-r90-g10000-p2-w1000 s2-rwmutex-r99-g1024-p2-w0 s3-cond64-g64-p2-w0
P=4: s1-mutex-g16-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s1-chan_oneway-g1-p4-w100 s3-chan64-g10000-p4-w0 s3-cond64-g64-p4-w0 s3-cond1024-g64-p4-w0 s1-mutex-g1024-p4-w0 s3-chan0-g1024-p4-w0 s2-mutex-r90-g64-p4-w0 s2-mutex-r50-g1024-p4-w0
P=1: s2-rwmutex-r99-g1024-p1-w1000 s1-mutex-g1024-p1-w0 s2-mutex-r90-g1024-p1-w0 s1-mutex-g16-p1-w0 s1-chan_reqresp-g1024-p1-w1000 s1-rwmutex-g1024-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s3-cond64-g10000-p1-w0 s1-chan_reqresp-g1024-p1-w100
P=8: s2-mutex-r50-g10000-p8-w0 s3-chan1024-g64-p8-w1000 s2-mutex-r50-g1024-p8-w0 s3-cond64-g1024-p8-w0 s1-mutex-g256-p8-w0 s1-atomic-g4-p8-w0 s3-chan0-g10000-p8-w1000 s3-chan1-g10000-p8-w1000 s3-cond1024-g10000-p8-w1000 s1-chan_oneway-g1-p8-w100 s1-rwmutex-g10000-p8-w100 s1-mutex-g10000-p8-w0 s3-chan1024-g10000-p8-w1000 s1-mutex-g1024-p8-w0 s3-chan0-g1024-p8-w1000 s1-mutex-g64-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s2-mutex-r50-g64-p8-w0 s2-mutex-r90-g10000-p8-w0 s3-chan64-g10000-p8-w1000 s3-cond64-g10000-p8-w0
```

- round 5 P=2: exit 0, 145 s
  - E1: host load 0.3% (loadavg 0.69 1.61 1.82), extra wait 0s
- round 5 P=4: exit 0, 44 s
  - E1: host load 0.0% (loadavg 0.78 1.54 1.79), extra wait 0s
- round 5 P=1: exit 0, 40 s
  - E1: host load 0.0% (loadavg 0.32 1.20 1.64), extra wait 0s
- round 5 P=8: exit 0, 93 s
  - E1: host load 2.3% (loadavg 2.23 2.38 2.07), extra wait 0s
- E2 end: host-check end: passed; temperature Tctl 39, Tccd1 40.25, Tccd2 32.25
- finished: 2026-10-19T23:21:55Z
