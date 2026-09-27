package work

import "testing"

func TestItersFor(t *testing.T) {
	if ItersFor(0, 2) != 0 {
		t.Fatal("W=0 must be zero iterations")
	}
	if ItersFor(1, 1e6) != 1 {
		t.Fatal("positive W needs at least one iteration")
	}
	if n := ItersFor(1000, 2); n != 500 {
		t.Fatalf("1000ns at 2ns/iter = %d", n)
	}
}

func TestCalibrate(t *testing.T) {
	c := Calibrate(3, 10_000)
	if c.NsPerIter <= 0 || len(c.Trials) != 3 {
		t.Fatalf("%+v", c)
	}
	if d := Drift(1, 1.1); d < 0.099 || d > 0.101 {
		t.Fatalf("drift %v", d)
	}
}

func TestCalibrateParallel(t *testing.T) {
	c := CalibrateParallel(4, 3, 10_000)
	if c.NsPerIter <= 0 || len(c.Trials) != 12 {
		t.Fatalf("%+v", c)
	}
	if c := CalibrateParallel(1, 3, 10_000); len(c.Trials) != 3 {
		t.Fatalf("single worker: %+v", c)
	}
}
