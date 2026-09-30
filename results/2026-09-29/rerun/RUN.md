# Run 2026-09-29 — cell rerun

- rerun of: `results/2026-09-29` (first pass, read only); list: `cells.txt`; links: `rerun_of.json`
- set: `main` (88 cells x 5 rounds)
- seed: `3652243538`
- image: `go-sync-bench` (sha256:eabc1573265a3c5a7b9c9a717971630fb8f9effbc2a5e29a02081500668547ee)
- go: go1.27.1
- host: Apple M4 Pro, 12 CPUs; docker 29.8.0
- P groups: 1 2 4 8; cpusets: {"1":"4","2":"4-5","4":"4-7","8":"4-11"}
- cool-down: 60s after every container; E1 enforced: 1 (limit 100% for up to 600s)
- system sleep (pmset): 0; enforced (sleep must be 0): 1
- started: 2026-09-30T01:38:39Z

## Round 1

```
batt: Now drawing from 'AC Power'
therm: Note: No thermal warning level has been recorded Note: No performance warning level has been recorded Note: No CPU power status has been recorded 
sleep: 0
P order: 1 2 8 4
P=1: s2-rwmutex-r99-g10000-p1-w0 s1-mutex-g10000-p1-w0 s1-mutex-g16-p1-w0 s1-mutex-g64-p1-w0 s2-mutex-r90-g64-p1-w0 s2-rwmutex-r99-g64-p1-w0 s2-mutex-r90-g1024-p1-w0 s1-rwmutex-g16-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s2-mutex-r50-g64-p1-w0 s3-cond64-g4-p1-w0 s1-rwmutex-g10000-p1-w0 s2-chan_owner-r99-g10000-p1-w0 s2-mutex-r99-g1024-p1-w0 s1-mutex-g1024-p1-w0 s2-mutex-r99-g64-p1-w0 s1-rwmutex-g64-p1-w0
P=2: s3-chan0-g10000-p2-w0 s1-chan_oneway-g64-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s1-chan_oneway-g16-p2-w100 s1-chan_oneway-g64-p2-w100 s1-chan_oneway-g1024-p2-w0 s3-cond64-g64-p2-w0 s1-chan_reqresp-g1024-p2-w0 s3-chan1-g4-p2-w1000 s2-chan_owner-r90-g4-p2-w0 s2-chan_owner-r99-g1024-p2-w0 s3-cond64-g10000-p2-w0 s3-chan1024-g64-p2-w0 s2-chan_owner-r99-g10000-p2-w0 s3-cond1024-g10000-p2-w1000 s1-mutex-g4-p2-w0 s1-chan_reqresp-g1-p2-w0 s3-chan0-g1024-p2-w1000 s1-chan_reqresp-g256-p2-w100 s2-rwmutex-r90-g4-p2-w0 s1-chan_oneway-g10000-p2-w1000 s2-chan_owner-r99-g4-p2-w0 s3-cond64-g64-p2-w1000 s1-mutex-g10000-p2-w100 s1-mutex-g1024-p2-w0 s1-chan_oneway-g256-p2-w0 s1-chan_reqresp-g4-p2-w0 s2-rwmutex-r99-g10000-p2-w1000 s2-rwmutex-r50-g1024-p2-w0 s2-rwmutex-r50-g10000-p2-w0 s2-chan_owner-r50-g10000-p2-w0 s1-mutex-g16-p2-w100 s1-rwmutex-g10000-p2-w100 s1-rwmutex-g16-p2-w1000 s1-chan_oneway-g1-p2-w100 s3-cond1024-g10000-p2-w0 s3-cond64-g1024-p2-w0 s1-mutex-g1024-p2-w100 s3-chan64-g4-p2-w1000 s2-rwmutex-r90-g10000-p2-w0 s2-rwmutex-r90-g1024-p2-w0 s3-cond1024-g64-p2-w1000 s1-chan_reqresp-g256-p2-w0 s3-cond1024-g1024-p2-w0 s2-chan_owner-r50-g4-p2-w1000 s2-chan_owner-r90-g64-p2-w0 s1-atomic-g10000-p2-w0
P=8: s2-rwmutex-r99-g64-p8-w0 s3-cond1024-g4-p8-w1000 s2-rwmutex-r50-g10000-p8-w0 s1-atomic-g1024-p8-w0 s3-chan64-g10000-p8-w0 s3-cond1024-g1024-p8-w1000 s3-chan0-g10000-p8-w1000 s2-rwmutex-r99-g10000-p8-w1000 s1-chan_oneway-g256-p8-w0 s3-cond1024-g4-p8-w0 s2-chan_owner-r50-g64-p8-w0
P=4: s3-chan0-g1024-p4-w1000 s1-rwmutex-g256-p4-w0 s2-rwmutex-r50-g10000-p4-w0 s1-chan_oneway-g10000-p4-w0 s1-rwmutex-g64-p4-w0 s1-chan_reqresp-g4-p4-w100 s3-cond1024-g1024-p4-w0 s2-chan_owner-r99-g10000-p4-w0 s1-chan_reqresp-g256-p4-w100 s3-cond1024-g4-p4-w0 s1-chan_oneway-g4-p4-w0 s2-chan_owner-r90-g64-p4-w0 s2-chan_owner-r99-g1024-p4-w0
```

- round 1 P=1: exit 0, 75 s
  - E1: host load 29.1% (loadavg 1.65 2.01 1.97), extra wait 0s
- round 1 P=2: exit 0, 206 s
  - E1: host load 59.4% (loadavg 1.60 2.06 2.01), extra wait 0s
- round 1 P=8: exit 0, 49 s
  - E1: host load 17.8% (loadavg 1.73 2.15 2.06), extra wait 0s
- round 1 P=4: exit 0, 57 s
  - E1: host load 18.2% (loadavg 2.02 2.17 2.08), extra wait 0s
## Round 2

```
batt: Now drawing from 'AC Power'
therm: Note: No thermal warning level has been recorded Note: No performance warning level has been recorded Note: No CPU power status has been recorded 
sleep: 0
P order: 1 8 2 4
P=1: s1-mutex-g1024-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s1-mutex-g10000-p1-w0 s2-mutex-r99-g1024-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s1-mutex-g16-p1-w0 s2-mutex-r90-g64-p1-w0 s3-cond64-g4-p1-w0 s2-rwmutex-r99-g64-p1-w0 s1-mutex-g64-p1-w0 s1-rwmutex-g64-p1-w0 s1-rwmutex-g10000-p1-w0 s1-rwmutex-g16-p1-w0 s2-mutex-r99-g64-p1-w0 s2-mutex-r90-g1024-p1-w0 s2-chan_owner-r99-g10000-p1-w0 s2-mutex-r50-g64-p1-w0
P=8: s3-chan0-g10000-p8-w1000 s2-rwmutex-r99-g64-p8-w0 s2-rwmutex-r50-g10000-p8-w0 s3-cond1024-g1024-p8-w1000 s2-chan_owner-r50-g64-p8-w0 s3-chan64-g10000-p8-w0 s3-cond1024-g4-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s1-atomic-g1024-p8-w0 s1-chan_oneway-g256-p8-w0 s3-cond1024-g4-p8-w1000
P=2: s3-cond64-g1024-p2-w0 s2-chan_owner-r99-g1024-p2-w0 s2-rwmutex-r90-g1024-p2-w0 s1-mutex-g1024-p2-w100 s1-chan_oneway-g64-p2-w0 s2-chan_owner-r50-g4-p2-w1000 s1-chan_oneway-g256-p2-w0 s1-chan_reqresp-g1024-p2-w0 s3-chan64-g4-p2-w1000 s3-chan0-g10000-p2-w0 s3-chan1024-g64-p2-w0 s2-chan_owner-r99-g4-p2-w0 s1-chan_reqresp-g4-p2-w0 s1-chan_reqresp-g1-p2-w0 s2-rwmutex-r50-g1024-p2-w0 s3-cond1024-g10000-p2-w1000 s3-chan1-g4-p2-w1000 s1-rwmutex-g10000-p2-w100 s1-mutex-g1024-p2-w0 s2-chan_owner-r99-g10000-p2-w0 s1-chan_oneway-g16-p2-w100 s1-atomic-g10000-p2-w0 s2-chan_owner-r90-g64-p2-w0 s1-rwmutex-g16-p2-w1000 s3-cond64-g10000-p2-w0 s3-cond1024-g1024-p2-w0 s3-cond1024-g10000-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s3-chan0-g1024-p2-w1000 s3-cond64-g64-p2-w1000 s1-mutex-g16-p2-w100 s1-chan_reqresp-g256-p2-w100 s1-chan_oneway-g1024-p2-w0 s1-chan_oneway-g1-p2-w100 s1-mutex-g10000-p2-w100 s2-rwmutex-r90-g10000-p2-w0 s1-chan_oneway-g10000-p2-w1000 s1-mutex-g4-p2-w0 s3-cond1024-g64-p2-w1000 s2-rwmutex-r99-g10000-p2-w1000 s2-chan_owner-r90-g4-p2-w0 s1-chan_reqresp-g256-p2-w0 s1-chan_oneway-g64-p2-w100 s2-rwmutex-r50-g10000-p2-w0 s2-rwmutex-r90-g4-p2-w0 s3-cond64-g64-p2-w0 s2-chan_owner-r50-g10000-p2-w0
P=4: s2-rwmutex-r50-g10000-p4-w0 s3-chan0-g1024-p4-w1000 s3-cond1024-g4-p4-w0 s1-rwmutex-g256-p4-w0 s2-chan_owner-r99-g1024-p4-w0 s2-chan_owner-r90-g64-p4-w0 s1-rwmutex-g64-p4-w0 s1-chan_oneway-g10000-p4-w0 s3-cond1024-g1024-p4-w0 s1-chan_reqresp-g4-p4-w100 s2-chan_owner-r99-g10000-p4-w0 s1-chan_oneway-g4-p4-w0 s1-chan_reqresp-g256-p4-w100
```

- round 2 P=1: exit 0, 75 s
  - E1: host load 39.0% (loadavg 1.28 1.82 1.95), extra wait 0s
- round 2 P=8: exit 0, 48 s
  - E1: host load 11.9% (loadavg 1.88 2.03 2.02), extra wait 0s
- round 2 P=2: exit 0, 207 s
  - E1: host load 25.8% (loadavg 1.85 2.06 2.04), extra wait 0s
- round 2 P=4: exit 0, 57 s
  - E1: host load 13.0% (loadavg 1.89 2.09 2.06), extra wait 0s
## Round 3

```
batt: Now drawing from 'AC Power'
therm: Note: No thermal warning level has been recorded Note: No performance warning level has been recorded Note: No CPU power status has been recorded 
sleep: 0
P order: 1 4 2 8
P=1: s1-rwmutex-g10000-p1-w0 s2-mutex-r90-g1024-p1-w0 s2-chan_owner-r99-g10000-p1-w0 s1-mutex-g16-p1-w0 s3-cond64-g4-p1-w0 s1-mutex-g64-p1-w0 s2-mutex-r99-g64-p1-w0 s2-mutex-r90-g64-p1-w0 s2-mutex-r50-g64-p1-w0 s1-rwmutex-g16-p1-w0 s1-mutex-g1024-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s1-mutex-g10000-p1-w0 s2-rwmutex-r99-g64-p1-w0 s2-mutex-r99-g1024-p1-w0 s1-rwmutex-g64-p1-w0 s2-rwmutex-r99-g10000-p1-w0
P=4: s1-rwmutex-g64-p4-w0 s2-rwmutex-r50-g10000-p4-w0 s3-cond1024-g1024-p4-w0 s3-cond1024-g4-p4-w0 s2-chan_owner-r99-g1024-p4-w0 s2-chan_owner-r90-g64-p4-w0 s1-rwmutex-g256-p4-w0 s1-chan_oneway-g10000-p4-w0 s1-chan_reqresp-g4-p4-w100 s1-chan_oneway-g4-p4-w0 s2-chan_owner-r99-g10000-p4-w0 s3-chan0-g1024-p4-w1000 s1-chan_reqresp-g256-p4-w100
P=2: s1-chan_oneway-g64-p2-w0 s3-chan0-g10000-p2-w0 s1-mutex-g1024-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s2-rwmutex-r50-g10000-p2-w0 s2-chan_owner-r50-g4-p2-w1000 s1-mutex-g1024-p2-w100 s1-chan_oneway-g256-p2-w0 s2-chan_owner-r90-g4-p2-w0 s2-chan_owner-r50-g10000-p2-w0 s3-chan64-g4-p2-w1000 s2-rwmutex-r90-g1024-p2-w0 s3-cond1024-g10000-p2-w1000 s3-cond1024-g1024-p2-w0 s1-mutex-g10000-p2-w100 s1-mutex-g16-p2-w100 s3-cond64-g64-p2-w0 s3-chan1-g4-p2-w1000 s1-chan_oneway-g64-p2-w100 s1-chan_reqresp-g4-p2-w0 s1-rwmutex-g10000-p2-w100 s1-mutex-g4-p2-w0 s1-chan_oneway-g1-p2-w100 s2-chan_owner-r99-g10000-p2-w0 s1-rwmutex-g16-p2-w1000 s1-atomic-g10000-p2-w0 s3-cond1024-g64-p2-w1000 s2-rwmutex-r90-g4-p2-w0 s1-chan_reqresp-g256-p2-w0 s2-chan_owner-r90-g64-p2-w0 s3-chan0-g1024-p2-w1000 s1-chan_oneway-g10000-p2-w1000 s1-chan_reqresp-g1024-p2-w0 s3-cond64-g10000-p2-w0 s2-chan_owner-r99-g4-p2-w0 s3-cond64-g64-p2-w1000 s2-chan_owner-r99-g1024-p2-w0 s1-chan_reqresp-g256-p2-w100 s1-chan_oneway-g16-p2-w100 s1-chan_reqresp-g1-p2-w0 s2-rwmutex-r50-g1024-p2-w0 s3-chan1024-g64-p2-w0 s2-rwmutex-r99-g10000-p2-w1000 s3-cond64-g1024-p2-w0 s2-rwmutex-r90-g10000-p2-w0 s1-chan_oneway-g1024-p2-w0 s3-cond1024-g10000-p2-w0
P=8: s1-chan_oneway-g256-p8-w0 s3-chan64-g10000-p8-w0 s2-rwmutex-r50-g10000-p8-w0 s3-cond1024-g4-p8-w0 s2-rwmutex-r99-g64-p8-w0 s3-chan0-g10000-p8-w1000 s3-cond1024-g1024-p8-w1000 s2-rwmutex-r99-g10000-p8-w1000 s3-cond1024-g4-p8-w1000 s2-chan_owner-r50-g64-p8-w0 s1-atomic-g1024-p8-w0
```

- round 3 P=1: exit 0, 75 s
  - E1: host load 13.3% (loadavg 1.42 1.86 1.97), extra wait 0s
- round 3 P=4: exit 0, 57 s
  - E1: host load 13.9% (loadavg 1.60 1.94 1.99), extra wait 0s
- round 3 P=2: exit 0, 206 s
  - E1: host load 35.9% (loadavg 2.16 2.36 2.18), extra wait 0s
- round 3 P=8: exit 0, 48 s
  - E1: host load 47.0% (loadavg 2.41 2.58 2.30), extra wait 0s
## Round 4

```
batt: Now drawing from 'AC Power'
therm: Note: No thermal warning level has been recorded Note: No performance warning level has been recorded Note: No CPU power status has been recorded 
sleep: 0
P order: 8 2 1 4
P=8: s2-rwmutex-r50-g10000-p8-w0 s1-atomic-g1024-p8-w0 s2-rwmutex-r99-g64-p8-w0 s3-cond1024-g1024-p8-w1000 s3-chan0-g10000-p8-w1000 s2-rwmutex-r99-g10000-p8-w1000 s3-chan64-g10000-p8-w0 s3-cond1024-g4-p8-w0 s1-chan_oneway-g256-p8-w0 s3-cond1024-g4-p8-w1000 s2-chan_owner-r50-g64-p8-w0
P=2: s2-chan_owner-r99-g4-p2-w0 s3-chan0-g1024-p2-w1000 s1-chan_reqresp-g1024-p2-w0 s3-chan0-g10000-p2-w0 s1-chan_reqresp-g256-p2-w100 s1-atomic-g10000-p2-w0 s1-chan_oneway-g256-p2-w0 s2-rwmutex-r90-g10000-p2-w0 s2-rwmutex-r99-g1024-p2-w1000 s2-rwmutex-r50-g10000-p2-w0 s1-chan_oneway-g64-p2-w0 s2-chan_owner-r99-g10000-p2-w0 s2-rwmutex-r90-g4-p2-w0 s1-mutex-g1024-p2-w0 s1-mutex-g4-p2-w0 s1-mutex-g10000-p2-w100 s3-cond64-g64-p2-w1000 s3-cond64-g64-p2-w0 s3-cond1024-g1024-p2-w0 s3-cond1024-g10000-p2-w0 s3-cond64-g1024-p2-w0 s1-mutex-g1024-p2-w100 s2-chan_owner-r99-g1024-p2-w0 s2-rwmutex-r99-g10000-p2-w1000 s1-chan_oneway-g64-p2-w100 s1-chan_reqresp-g256-p2-w0 s3-cond64-g10000-p2-w0 s1-chan_reqresp-g4-p2-w0 s1-rwmutex-g16-p2-w1000 s1-chan_oneway-g16-p2-w100 s1-chan_reqresp-g1-p2-w0 s3-cond1024-g10000-p2-w1000 s2-rwmutex-r90-g1024-p2-w0 s3-cond1024-g64-p2-w1000 s1-rwmutex-g10000-p2-w100 s3-chan1024-g64-p2-w0 s1-chan_oneway-g10000-p2-w1000 s1-chan_oneway-g1-p2-w100 s1-mutex-g16-p2-w100 s2-chan_owner-r90-g64-p2-w0 s2-rwmutex-r50-g1024-p2-w0 s2-chan_owner-r90-g4-p2-w0 s3-chan1-g4-p2-w1000 s3-chan64-g4-p2-w1000 s1-chan_oneway-g1024-p2-w0 s2-chan_owner-r50-g4-p2-w1000 s2-chan_owner-r50-g10000-p2-w0
P=1: s2-rwmutex-r99-g10000-p1-w0 s1-rwmutex-g16-p1-w0 s1-rwmutex-g64-p1-w0 s3-cond64-g4-p1-w0 s2-chan_owner-r99-g10000-p1-w0 s1-mutex-g10000-p1-w0 s2-mutex-r90-g1024-p1-w0 s2-mutex-r99-g64-p1-w0 s2-mutex-r99-g1024-p1-w0 s1-mutex-g64-p1-w0 s2-mutex-r50-g64-p1-w0 s2-mutex-r90-g64-p1-w0 s1-rwmutex-g10000-p1-w0 s2-rwmutex-r99-g64-p1-w0 s1-mutex-g16-p1-w0 s1-mutex-g1024-p1-w0 s2-rwmutex-r99-g1024-p1-w1000
P=4: s2-rwmutex-r50-g10000-p4-w0 s2-chan_owner-r99-g1024-p4-w0 s1-chan_oneway-g10000-p4-w0 s1-chan_reqresp-g256-p4-w100 s3-chan0-g1024-p4-w1000 s2-chan_owner-r90-g64-p4-w0 s1-chan_reqresp-g4-p4-w100 s2-chan_owner-r99-g10000-p4-w0 s3-cond1024-g4-p4-w0 s1-chan_oneway-g4-p4-w0 s1-rwmutex-g256-p4-w0 s3-cond1024-g1024-p4-w0 s1-rwmutex-g64-p4-w0
```

- round 4 P=8: exit 0, 48 s
  - E1: host load 12.0% (loadavg 2.00 2.51 2.32), extra wait 0s
- round 4 P=2: exit 0, 207 s
  - E1: host load 39.3% (loadavg 1.12 1.98 2.14), extra wait 0s
- round 4 P=1: exit 0, 75 s
  - E1: host load 33.8% (loadavg 1.55 1.84 2.06), extra wait 0s
- round 4 P=4: exit 0, 57 s
  - E1: host load 19.2% (loadavg 2.13 2.09 2.14), extra wait 0s
## Round 5

```
batt: Now drawing from 'AC Power'
therm: Note: No thermal warning level has been recorded Note: No performance warning level has been recorded Note: No CPU power status has been recorded 
sleep: 0
P order: 1 4 2 8
P=1: s1-rwmutex-g10000-p1-w0 s2-rwmutex-r99-g64-p1-w0 s2-mutex-r90-g64-p1-w0 s2-rwmutex-r99-g1024-p1-w1000 s2-mutex-r50-g64-p1-w0 s1-mutex-g10000-p1-w0 s1-rwmutex-g16-p1-w0 s1-mutex-g1024-p1-w0 s1-mutex-g16-p1-w0 s1-mutex-g64-p1-w0 s3-cond64-g4-p1-w0 s2-rwmutex-r99-g10000-p1-w0 s1-rwmutex-g64-p1-w0 s2-mutex-r90-g1024-p1-w0 s2-chan_owner-r99-g10000-p1-w0 s2-mutex-r99-g64-p1-w0 s2-mutex-r99-g1024-p1-w0
P=4: s1-chan_reqresp-g256-p4-w100 s1-chan_reqresp-g4-p4-w100 s1-rwmutex-g64-p4-w0 s1-chan_oneway-g10000-p4-w0 s1-chan_oneway-g4-p4-w0 s2-rwmutex-r50-g10000-p4-w0 s3-cond1024-g4-p4-w0 s2-chan_owner-r90-g64-p4-w0 s2-chan_owner-r99-g1024-p4-w0 s2-chan_owner-r99-g10000-p4-w0 s3-chan0-g1024-p4-w1000 s1-rwmutex-g256-p4-w0 s3-cond1024-g1024-p4-w0
P=2: s1-mutex-g16-p2-w100 s3-chan0-g10000-p2-w0 s1-chan_reqresp-g1-p2-w0 s3-chan64-g4-p2-w1000 s1-mutex-g1024-p2-w100 s3-cond64-g1024-p2-w0 s3-cond1024-g10000-p2-w1000 s1-chan_reqresp-g256-p2-w0 s1-atomic-g10000-p2-w0 s3-cond1024-g64-p2-w1000 s2-chan_owner-r99-g4-p2-w0 s1-chan_oneway-g1024-p2-w0 s1-mutex-g1024-p2-w0 s3-cond64-g64-p2-w1000 s1-chan_oneway-g256-p2-w0 s2-rwmutex-r50-g10000-p2-w0 s1-mutex-g10000-p2-w100 s1-chan_reqresp-g256-p2-w100 s2-chan_owner-r90-g4-p2-w0 s1-chan_oneway-g64-p2-w100 s1-chan_oneway-g16-p2-w100 s1-chan_reqresp-g4-p2-w0 s2-chan_owner-r50-g10000-p2-w0 s3-chan1024-g64-p2-w0 s3-chan1-g4-p2-w1000 s3-cond64-g10000-p2-w0 s1-chan_reqresp-g1024-p2-w0 s1-chan_oneway-g10000-p2-w1000 s3-chan0-g1024-p2-w1000 s2-rwmutex-r90-g1024-p2-w0 s1-mutex-g4-p2-w0 s1-rwmutex-g16-p2-w1000 s2-rwmutex-r99-g1024-p2-w1000 s1-rwmutex-g10000-p2-w100 s3-cond1024-g1024-p2-w0 s2-rwmutex-r90-g4-p2-w0 s1-chan_oneway-g1-p2-w100 s2-chan_owner-r99-g1024-p2-w0 s2-chan_owner-r99-g10000-p2-w0 s2-rwmutex-r99-g10000-p2-w1000 s3-cond64-g64-p2-w0 s3-cond1024-g10000-p2-w0 s2-rwmutex-r50-g1024-p2-w0 s2-chan_owner-r90-g64-p2-w0 s2-rwmutex-r90-g10000-p2-w0 s2-chan_owner-r50-g4-p2-w1000 s1-chan_oneway-g64-p2-w0
P=8: s3-cond1024-g4-p8-w1000 s1-atomic-g1024-p8-w0 s2-rwmutex-r50-g10000-p8-w0 s3-cond1024-g4-p8-w0 s2-chan_owner-r50-g64-p8-w0 s3-cond1024-g1024-p8-w1000 s1-chan_oneway-g256-p8-w0 s2-rwmutex-r99-g10000-p8-w1000 s3-chan64-g10000-p8-w0 s2-rwmutex-r99-g64-p8-w0 s3-chan0-g10000-p8-w1000
```

- round 5 P=1: exit 0, 75 s
  - E1: host load 9.0% (loadavg 1.12 1.72 1.98), extra wait 0s
- round 5 P=4: exit 0, 57 s
  - E1: host load 17.6% (loadavg 1.52 1.79 1.98), extra wait 0s
- round 5 P=2: exit 0, 206 s
  - E1: host load 22.9% (loadavg 1.58 1.86 1.97), extra wait 0s
- round 5 P=8: exit 0, 49 s
  - E1: host load 27.6% (loadavg 1.95 2.12 2.07), extra wait 0s
- finished: 2026-09-30T02:31:37Z
