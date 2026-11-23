package analysis

import (
	"strings"
	"testing"
)

func TestClassify_Categories(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Category
	}{
		{"oom", "fatal error: runtime: out of memory\nexiting", CategoryOOM},
		{"timeout", "Error: context deadline exceeded while waiting", CategoryTimeout},
		{"infra", "dial tcp 10.0.0.1:5432: connect: connection refused", CategoryInfraError},
		{"dependency", "npm ERR! code ERESOLVE\nnpm ERR! Could not resolve dependency", CategoryDependencyError},
		{"compile", "./main.go:10:2: undefined: fmt.Printlnx", CategoryCompileError},
		{"lint", "golangci-lint run\nfile.go:3: ineffectual assignment", CategoryLintError},
		{"test", "--- FAIL: TestLogin (0.00s)\n    login_test.go:21: want 200", CategoryTestFailure},
		{"unknown", "building image\npushing layers\nall quiet", CategoryUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.raw)
			if got.Category != tc.want {
				t.Errorf("Classify category = %q, want %q (signature=%q)", got.Category, tc.want, got.SignatureLine)
			}
		})
	}
}

// Root cause beats symptom: an OOM that also produces a test FAIL downstream
// should classify as OOM, not test_failure.
func TestClassify_PriorityRootCauseWins(t *testing.T) {
	raw := strings.Join([]string{
		"--- FAIL: TestHeavy (12.00s)",
		"fatal error: runtime: out of memory",
		"exit status 2",
	}, "\n")
	if got := Classify(raw); got.Category != CategoryOOM {
		t.Errorf("category = %q, want oom (root cause over symptom)", got.Category)
	}
}

func TestClassify_SignatureAndEvidence(t *testing.T) {
	raw := strings.Join([]string{
		"ok  pkg/one",
		"--- FAIL: TestLogin (0.00s)",
		"    login_test.go:21: status = 500, want 200",
		"FAIL",
	}, "\n")
	got := Classify(raw)
	if got.Category != CategoryTestFailure {
		t.Fatalf("category = %q, want test_failure", got.Category)
	}
	if got.SignatureLine != "--- FAIL: TestLogin (0.00s)" {
		t.Errorf("signature = %q, want the FAIL line", got.SignatureLine)
	}
	if !strings.Contains(got.Evidence, "status = 500, want 200") {
		t.Errorf("evidence missing context line: %q", got.Evidence)
	}
}

func TestClassify_UnknownUsesLastLine(t *testing.T) {
	raw := "step one\nstep two\nmysterious final state"
	got := Classify(raw)
	if got.Category != CategoryUnknown {
		t.Fatalf("category = %q, want unknown", got.Category)
	}
	if got.SignatureLine != "mysterious final state" {
		t.Errorf("signature = %q, want last non-empty line", got.SignatureLine)
	}
}
