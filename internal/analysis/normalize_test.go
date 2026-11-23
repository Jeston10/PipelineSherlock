package analysis

import "testing"

func TestNormalize_ReplacesVolatileTokens(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"timestamp", "2026-01-02T10:04:05Z starting", "<TS> starting"},
		{"duration", "test took 1.23s", "test took <DUR>"},
		{"ms duration", "elapsed 450ms", "elapsed <DUR>"},
		{"uuid", "run 123e4567-e89b-12d3-a456-426614174000 done", "run <UUID> done"},
		{"hex sha", "commit a1b2c3d4e5f6 built", "commit <HEX> built"},
		{"address", "panic at 0xc0000b00f8", "panic at <ADDR>"},
		{"plain number", "exit code 137", "exit code <N>"},
		{"collapse whitespace", "a\t b\n  c", "a b c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalize(tc.in); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalize_StripsANSI(t *testing.T) {
	in := "\x1b[31mFAIL\x1b[0m some_test"
	if got := Normalize(in); got != "FAIL some_test" {
		t.Errorf("Normalize = %q, want %q", got, "FAIL some_test")
	}
}

func TestNormalize_Idempotent(t *testing.T) {
	in := "2026-01-02T10:04:05Z job 42 failed after 1.5s at 0xdeadbeef"
	once := Normalize(in)
	twice := Normalize(once)
	if once != twice {
		t.Errorf("not idempotent: %q != %q", once, twice)
	}
}

// The core property: the same failure from two different runs (different
// timestamps, durations, and ids) must normalize to the same signature.
func TestNormalize_SameFailureDifferentRunsConverge(t *testing.T) {
	runA := "2026-01-02T10:04:05Z FAIL TestLogin took 1.20s (worker 0xaa11)"
	runB := "2026-03-09T22:18:41Z FAIL TestLogin took 3.84s (worker 0xff99)"
	if Normalize(runA) != Normalize(runB) {
		t.Errorf("same failure did not converge:\n  A=%q\n  B=%q", Normalize(runA), Normalize(runB))
	}
}

func TestNormalize_DistinctFailuresStayDistinct(t *testing.T) {
	a := Normalize("FAIL TestLogin assertion failed")
	b := Normalize("FAIL TestLogout assertion failed")
	if a == b {
		t.Errorf("distinct failures collapsed: %q", a)
	}
}
