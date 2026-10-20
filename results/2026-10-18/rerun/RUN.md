# Run 2026-10-18 — cell rerun

- rerun of: `results/2026-10-18` (first pass, read only); list: `cells.txt`; links: `rerun_of.json`
- set: `main` (75 cells x 5 rounds)
- seed: `3181962264`
- image: `go-sync-bench` (sha256:3dc28e0046965f2f34fee3321d044346f23b21591bbeacc76f8ee8ef8a7903cb)
- go: go1.27.1
- profile: `linux-metal` (isolated 8-15, system 0-7)
- host: AMD Ryzen 9 5950X 16-Core Processor, 16 CPUs online; kernel 7.0.0-38-generic; docker 29.9.1
- P groups: 1 2 4 8; cpusets: {"1":"8","2":"8-9","4":"8-11","8":"8-15"}
- cool-down: 60s after every container; E1 enforced: 1 (limit 100% for up to 600s)
- E2: host-check start: passed; temperature Tctl 37.75, Tccd1 37, Tccd2 30.75; enforced: 1; records `host-check/`, `environment.json`
- started: 2026-10-18T19:17:51Z

## Round 1

```
host-check round-1: passed; temperature Tctl 40.875, Tccd1 43.5, Tccd2 31.5
sleep: see host-check
P order: 2 8 4 1
P=2: s1-chan_oneway-g16-p2-w0 s3-cond1024-g1024-p2-w0 s1-mutex-g16-p2-w1000 s1-rwmutex-g256-p2-w0 s3-chan1024-g10000-p2-w1000 s1-chan_reqresp-g1024-p2-w100 s2-rwmutex-r99-g10000-p2-w1000 s1-mutex-g1024-p2-w0 s1-rwmutex-g10000-p2-w0 s3-cond1024-g10000-p2-w0 s3-cond64-g64-p2-w0 s3-cond1024-g10000-p2-w1000 s1-rwmutex-g10000-p2-w100 s1-chan_reqresp-g256-p2-w100 s1-chan_oneway-g1024-p2-w100 s3-chan64-g10000-p2-w1000 s1-rwmutex-g1024-p2-w100 s1-mutex-g64-p2-w0 s2-rwmutex-r90-g1024-p2-w0 s1-mutex-g4-p2-w0 s1-atomic-g64-p2-w0 s1-chan_reqresp-g10000-p2-w0 s1-mutex-g1024-p2-w100 s1-mutex-g16-p2-w0 s2-rwmutex-r99-g1024-p2-w0 s1-chan_reqresp-g16-p2-w0 s3-chan0-g10000-p2-w1000 s2-rwmutex-r99-g1024-p2-w1000 s1-mutex-g4-p2-w100 s3-chan1-g10000-p2-w1000 s1-mutex-g256-p2-w0 s2-rwmutex-r99-g64-p2-w0
P=8: s2-rwmutex-r99-g10000-p8-w1000 s3-cond64-g10000-p8-w1000 s3-chan0-g10000-p8-w1000 s3-cond1024-g10000-p8-w0 s3-chan1024-g10000-p8-w1000 s2-mutex-r99-g64-p8-w0 s2-mutex-r90-g10000-p8-w0 s1-rwmutex-g256-p8-w0 s2-mutex-r90-g1024-p8-w0 s3-cond1024-g1024-p8-w0 s3-cond1024-g10000-p8-w1000 s3-chan64-g64-p8-w1000 s3-chan1024-g64-p8-w1000 s1-rwmutex-g10000-p8-w0 s3-chan1-g10000-p8-w1000 s3-cond64-g10000-p8-w0 s3-chan1024-g1024-p8-w1000 s1-mutex-g1024-p8-w0 s3-chan64-g1024-p8-w1000 s1-mutex-g1024-p8-w100 s1-rwmutex-g10000-p8-w100 s2-mutex-r50-g10000-p8-w0 s2-mutex-r50-g64-p8-w0 s3-chan0-g1024-p8-w1000 s2-mutex-r99-g10000-p8-w0 s2-mutex-r90-g64-p8-w0 s2-mutex-r50-g1024-p8-w0 s3-cond64-g1024-p8-w0
P=4: s2-rwmutex-r99-g10000-p4-w1000 s1-chan_reqresp-g10000-p4-w100 s3-cond1024-g64-p4-w0 s1-chan_oneway-g1-p4-w0 s2-mutex-r50-g1024-p4-w0 s3-chan0-g10000-p4-w1000 s1-chan_oneway-g1-p4-w100 s2-chan_owner-r99-g10000-p4-w0 s1-mutex-g16-p4-w0 s1-rwmutex-g10000-p4-w0 s1-rwmutex-g16-p4-w0
P=1: s1-mutex-g1024-p1-w0 s3-cond64-g64-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s3-cond64-g1024-p1-w1000
```

- round 1 P=2: exit 0, 142 s
  - E1: host load 0.0% (loadavg 0.64 0.66 0.29), extra wait 0s
- round 1 P=8: exit 0, 123 s
  - E1: host load 0.3% (loadavg 2.13 2.25 1.04), extra wait 0s
- round 1 P=4: exit 0, 49 s
  - E1: host load 0.3% (loadavg 1.09 2.00 1.10), extra wait 0s
- round 1 P=1: exit 0, 18 s
  - E1: host load 0.3% (loadavg 0.39 1.58 1.02), extra wait 0s
## Round 2

```
host-check round-2: passed; temperature Tctl 37.75, Tccd1 38.5, Tccd2 30.25
sleep: see host-check
P order: 4 2 1 8
P=4: s1-mutex-g16-p4-w0 s3-chan0-g10000-p4-w1000 s2-chan_owner-r99-g10000-p4-w0 s2-mutex-r50-g1024-p4-w0 s1-chan_reqresp-g10000-p4-w100 s3-cond1024-g64-p4-w0 s1-chan_oneway-g1-p4-w100 s1-chan_oneway-g1-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s1-rwmutex-g10000-p4-w0 s1-rwmutex-g16-p4-w0
P=2: s1-mutex-g64-p2-w0 s1-chan_reqresp-g256-p2-w100 s1-mutex-g4-p2-w100 s3-cond1024-g10000-p2-w0 s3-chan1-g10000-p2-w1000 s1-chan_reqresp-g16-p2-w0 s1-chan_reqresp-g1024-p2-w100 s3-chan1024-g10000-p2-w1000 s3-cond1024-g10000-p2-w1000 s1-chan_reqresp-g10000-p2-w0 s1-mutex-g16-p2-w0 s2-rwmutex-r99-g10000-p2-w1000 s2-rwmutex-r99-g1024-p2-w1000 s1-rwmutex-g256-p2-w0 s3-cond1024-g1024-p2-w0 s2-rwmutex-r99-g64-p2-w0 s1-chan_oneway-g16-p2-w0 s2-rwmutex-r90-g1024-p2-w0 s1-rwmutex-g1024-p2-w100 s1-rwmutex-g10000-p2-w0 s1-atomic-g64-p2-w0 s3-cond64-g64-p2-w0 s1-mutex-g256-p2-w0 s1-chan_oneway-g1024-p2-w100 s1-mutex-g1024-p2-w0 s3-chan0-g10000-p2-w1000 s3-chan64-g10000-p2-w1000 s1-rwmutex-g10000-p2-w100 s2-rwmutex-r99-g1024-p2-w0 s1-mutex-g16-p2-w1000 s1-mutex-g1024-p2-w100 s1-mutex-g4-p2-w0
P=1: s1-mutex-g1024-p1-w0 s3-cond64-g64-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s3-cond64-g1024-p1-w1000
P=8: s3-cond64-g10000-p8-w1000 s3-chan0-g1024-p8-w1000 s3-cond1024-g1024-p8-w0 s2-mutex-r90-g64-p8-w0 s2-mutex-r90-g10000-p8-w0 s1-mutex-g1024-p8-w100 s3-cond1024-g10000-p8-w1000 s3-cond64-g1024-p8-w0 s2-mutex-r99-g10000-p8-w0 s3-chan1024-g10000-p8-w1000 s3-chan1024-g64-p8-w1000 s3-chan64-g1024-p8-w1000 s1-rwmutex-g10000-p8-w100 s3-chan1-g10000-p8-w1000 s1-rwmutex-g256-p8-w0 s2-mutex-r50-g1024-p8-w0 s3-cond1024-g10000-p8-w0 s2-mutex-r50-g10000-p8-w0 s3-cond64-g10000-p8-w0 s3-chan1024-g1024-p8-w1000 s2-mutex-r99-g64-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s1-rwmutex-g10000-p8-w0 s1-mutex-g1024-p8-w0 s3-chan0-g10000-p8-w1000 s2-mutex-r90-g1024-p8-w0 s2-mutex-r50-g64-p8-w0 s3-chan64-g64-p8-w1000
```

- round 2 P=4: exit 0, 48 s
  - E1: host load 0.0% (loadavg 0.63 1.45 1.05), extra wait 0s
- round 2 P=2: exit 0, 161 s
  - E1: host load 0.0% (loadavg 0.61 1.33 1.11), extra wait 0s
- round 2 P=1: exit 0, 18 s
  - E1: host load 0.3% (loadavg 0.29 1.08 1.04), extra wait 0s
- round 2 P=8: exit 0, 124 s
  - E1: host load 0.3% (loadavg 2.31 2.63 1.72), extra wait 0s
## Round 3

```
host-check round-3: passed; temperature Tctl 38.375, Tccd1 39.5, Tccd2 31.25
sleep: see host-check
P order: 4 1 2 8
P=4: s2-chan_owner-r99-g10000-p4-w0 s1-mutex-g16-p4-w0 s2-mutex-r50-g1024-p4-w0 s3-chan0-g10000-p4-w1000 s1-rwmutex-g10000-p4-w0 s1-chan_reqresp-g10000-p4-w100 s1-chan_oneway-g1-p4-w100 s2-rwmutex-r99-g10000-p4-w1000 s3-cond1024-g64-p4-w0 s1-rwmutex-g16-p4-w0 s1-chan_oneway-g1-p4-w0
P=1: s2-rwmutex-r99-g10000-p1-w0 s1-mutex-g1024-p1-w0 s3-cond64-g64-p1-w0 s3-cond64-g1024-p1-w1000
P=2: s1-chan_reqresp-g1024-p2-w100 s1-rwmutex-g10000-p2-w100 s1-atomic-g64-p2-w0 s1-mutex-g4-p2-w0 s3-chan1024-g10000-p2-w1000 s3-cond64-g64-p2-w0 s2-rwmutex-r99-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s1-rwmutex-g1024-p2-w100 s1-chan_oneway-g16-p2-w0 s1-chan_reqresp-g16-p2-w0 s1-rwmutex-g256-p2-w0 s2-rwmutex-r99-g1024-p2-w0 s3-chan64-g10000-p2-w1000 s1-mutex-g16-p2-w0 s1-mutex-g1024-p2-w0 s1-mutex-g16-p2-w1000 s2-rwmutex-r99-g10000-p2-w1000 s1-chan_reqresp-g256-p2-w100 s2-rwmutex-r90-g1024-p2-w0 s1-rwmutex-g10000-p2-w0 s1-chan_reqresp-g10000-p2-w0 s1-mutex-g64-p2-w0 s3-cond1024-g1024-p2-w0 s3-cond1024-g10000-p2-w0 s3-chan1-g10000-p2-w1000 s1-mutex-g1024-p2-w100 s1-mutex-g4-p2-w100 s3-chan0-g10000-p2-w1000 s1-mutex-g256-p2-w0 s3-cond1024-g10000-p2-w1000 s1-chan_oneway-g1024-p2-w100
P=8: s2-rwmutex-r99-g10000-p8-w1000 s3-cond64-g10000-p8-w0 s3-chan1024-g1024-p8-w1000 s1-rwmutex-g10000-p8-w0 s1-rwmutex-g256-p8-w0 s3-chan1024-g64-p8-w1000 s2-mutex-r90-g1024-p8-w0 s2-mutex-r99-g64-p8-w0 s3-cond1024-g10000-p8-w1000 s3-chan0-g1024-p8-w1000 s1-mutex-g1024-p8-w0 s2-mutex-r50-g1024-p8-w0 s2-mutex-r90-g64-p8-w0 s3-chan0-g10000-p8-w1000 s3-chan1024-g10000-p8-w1000 s2-mutex-r50-g64-p8-w0 s3-cond1024-g1024-p8-w0 s2-mutex-r50-g10000-p8-w0 s3-chan64-g64-p8-w1000 s3-chan64-g1024-p8-w1000 s1-mutex-g1024-p8-w100 s3-chan1-g10000-p8-w1000 s2-mutex-r99-g10000-p8-w0 s3-cond64-g10000-p8-w1000 s2-mutex-r90-g10000-p8-w0 s1-rwmutex-g10000-p8-w100 s3-cond64-g1024-p8-w0 s3-cond1024-g10000-p8-w0
```

- round 3 P=4: exit 0, 48 s
  - E1: host load 0.3% (loadavg 0.93 2.16 1.67), extra wait 0s
- round 3 P=1: exit 0, 18 s
  - E1: host load 0.0% (loadavg 0.38 1.72 1.55), extra wait 0s
- round 3 P=2: exit 0, 162 s
  - E1: host load 0.3% (loadavg 0.73 1.49 1.51), extra wait 0s
- round 3 P=8: exit 0, 123 s
  - E1: host load 0.3% (loadavg 2.41 2.88 2.12), extra wait 0s
## Round 4

```
host-check round-4: passed; temperature Tctl 38.75, Tccd1 42.25, Tccd2 31
sleep: see host-check
P order: 2 8 1 4
P=2: s1-chan_oneway-g1024-p2-w100 s2-rwmutex-r99-g1024-p2-w0 s3-cond1024-g10000-p2-w0 s1-chan_reqresp-g256-p2-w100 s1-chan_oneway-g16-p2-w0 s3-chan64-g10000-p2-w1000 s3-cond1024-g1024-p2-w0 s1-rwmutex-g256-p2-w0 s3-cond1024-g10000-p2-w1000 s1-chan_reqresp-g16-p2-w0 s1-rwmutex-g1024-p2-w100 s3-chan1024-g10000-p2-w1000 s2-rwmutex-r99-g10000-p2-w1000 s2-rwmutex-r90-g1024-p2-w0 s1-mutex-g16-p2-w1000 s1-mutex-g256-p2-w0 s3-chan0-g10000-p2-w1000 s1-mutex-g64-p2-w0 s1-chan_reqresp-g10000-p2-w0 s1-mutex-g1024-p2-w100 s1-rwmutex-g10000-p2-w100 s3-chan1-g10000-p2-w1000 s1-mutex-g16-p2-w0 s1-mutex-g4-p2-w0 s1-atomic-g64-p2-w0 s2-rwmutex-r99-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s1-mutex-g4-p2-w100 s3-cond64-g64-p2-w0 s1-mutex-g1024-p2-w0 s1-rwmutex-g10000-p2-w0 s1-chan_reqresp-g1024-p2-w100
P=8: s3-chan64-g64-p8-w1000 s3-chan1024-g10000-p8-w1000 s3-chan1024-g64-p8-w1000 s3-cond64-g10000-p8-w1000 s1-rwmutex-g10000-p8-w100 s3-chan1024-g1024-p8-w1000 s3-cond64-g10000-p8-w0 s1-rwmutex-g256-p8-w0 s2-mutex-r99-g10000-p8-w0 s1-mutex-g1024-p8-w100 s3-cond1024-g10000-p8-w0 s1-mutex-g1024-p8-w0 s2-mutex-r90-g64-p8-w0 s3-cond64-g1024-p8-w0 s3-chan0-g1024-p8-w1000 s2-rwmutex-r99-g10000-p8-w1000 s2-mutex-r50-g1024-p8-w0 s3-chan0-g10000-p8-w1000 s2-mutex-r50-g10000-p8-w0 s2-mutex-r99-g64-p8-w0 s3-cond1024-g1024-p8-w0 s2-mutex-r90-g10000-p8-w0 s3-chan64-g1024-p8-w1000 s3-cond1024-g10000-p8-w1000 s2-mutex-r50-g64-p8-w0 s3-chan1-g10000-p8-w1000 s2-mutex-r90-g1024-p8-w0 s1-rwmutex-g10000-p8-w0
P=1: s3-cond64-g64-p1-w0 s1-mutex-g1024-p1-w0 s3-cond64-g1024-p1-w1000 s2-rwmutex-r99-g10000-p1-w0
P=4: s2-chan_owner-r99-g10000-p4-w0 s1-chan_oneway-g1-p4-w0 s1-rwmutex-g16-p4-w0 s1-chan_oneway-g1-p4-w100 s1-mutex-g16-p4-w0 s1-chan_reqresp-g10000-p4-w100 s3-cond1024-g64-p4-w0 s3-chan0-g10000-p4-w1000 s2-mutex-r50-g1024-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s1-rwmutex-g10000-p4-w0
```

- round 4 P=2: exit 0, 151 s
  - E1: host load 0.0% (loadavg 0.74 2.07 1.97), extra wait 0s
- round 4 P=8: exit 0, 124 s
  - E1: host load 0.0% (loadavg 2.25 3.11 2.46), extra wait 0s
- round 4 P=1: exit 0, 17 s
  - E1: host load 0.0% (loadavg 0.66 2.42 2.27), extra wait 0s
- round 4 P=4: exit 0, 49 s
  - E1: host load 0.0% (loadavg 0.76 2.05 2.16), extra wait 0s
## Round 5

```
host-check round-5: passed; temperature Tctl 38.375, Tccd1 38, Tccd2 31.75
sleep: see host-check
P order: 8 1 2 4
P=8: s3-chan1024-g64-p8-w1000 s2-rwmutex-r99-g10000-p8-w1000 s2-mutex-r50-g1024-p8-w0 s3-cond1024-g10000-p8-w0 s2-mutex-r90-g10000-p8-w0 s2-mutex-r90-g1024-p8-w0 s2-mutex-r50-g10000-p8-w0 s1-rwmutex-g10000-p8-w0 s3-cond1024-g10000-p8-w1000 s3-cond64-g1024-p8-w0 s3-chan0-g1024-p8-w1000 s2-mutex-r50-g64-p8-w0 s3-chan64-g64-p8-w1000 s3-chan0-g10000-p8-w1000 s1-mutex-g1024-p8-w100 s2-mutex-r99-g10000-p8-w0 s1-rwmutex-g256-p8-w0 s3-cond64-g10000-p8-w1000 s3-chan1-g10000-p8-w1000 s3-cond64-g10000-p8-w0 s3-chan1024-g10000-p8-w1000 s1-rwmutex-g10000-p8-w100 s3-cond1024-g1024-p8-w0 s3-chan1024-g1024-p8-w1000 s1-mutex-g1024-p8-w0 s2-mutex-r99-g64-p8-w0 s2-mutex-r90-g64-p8-w0 s3-chan64-g1024-p8-w1000
P=1: s1-mutex-g1024-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s3-cond64-g1024-p1-w1000 s3-cond64-g64-p1-w0
P=2: s1-mutex-g4-p2-w100 s3-chan1-g10000-p2-w1000 s1-mutex-g1024-p2-w0 s1-chan_reqresp-g16-p2-w0 s1-chan_oneway-g1024-p2-w100 s1-rwmutex-g10000-p2-w0 s3-chan1024-g10000-p2-w1000 s3-cond64-g64-p2-w0 s1-chan_reqresp-g1024-p2-w100 s1-mutex-g64-p2-w0 s3-chan0-g10000-p2-w1000 s2-rwmutex-r90-g1024-p2-w0 s1-chan_reqresp-g256-p2-w100 s1-rwmutex-g256-p2-w0 s2-rwmutex-r99-g1024-p2-w0 s2-rwmutex-r99-g10000-p2-w1000 s1-mutex-g1024-p2-w100 s1-rwmutex-g1024-p2-w100 s1-atomic-g64-p2-w0 s3-cond1024-g1024-p2-w0 s1-mutex-g256-p2-w0 s2-rwmutex-r99-g64-p2-w0 s1-mutex-g16-p2-w0 s3-cond1024-g10000-p2-w1000 s1-mutex-g4-p2-w0 s1-chan_oneway-g16-p2-w0 s3-cond1024-g10000-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s1-rwmutex-g10000-p2-w100 s1-mutex-g16-p2-w1000 s1-chan_reqresp-g10000-p2-w0 s3-chan64-g10000-p2-w1000
P=4: s1-mutex-g16-p4-w0 s3-cond1024-g64-p4-w0 s1-chan_reqresp-g10000-p4-w100 s1-rwmutex-g10000-p4-w0 s1-chan_oneway-g1-p4-w100 s2-mutex-r50-g1024-p4-w0 s1-rwmutex-g16-p4-w0 s1-chan_oneway-g1-p4-w0 s2-chan_owner-r99-g10000-p4-w0 s2-rwmutex-r99-g10000-p4-w1000 s3-chan0-g10000-p4-w1000
```

- round 5 P=8: exit 0, 123 s
  - E1: host load 0.3% (loadavg 2.31 3.15 2.64), extra wait 0s
- round 5 P=1: exit 0, 18 s
  - E1: host load 0.3% (loadavg 0.74 2.48 2.44), extra wait 0s
- round 5 P=2: exit 0, 154 s
  - E1: host load 0.3% (loadavg 0.75 1.89 2.23), extra wait 0s
- round 5 P=4: exit 0, 48 s
  - E1: host load 0.3% (loadavg 0.72 1.66 2.12), extra wait 0s
- E2 end: host-check end: passed; temperature Tctl 38.375, Tccd1 47.5, Tccd2 30.5
- finished: 2026-10-18T20:07:41Z
