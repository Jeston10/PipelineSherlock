package analysis

import "testing"

func TestFingerprint_Deterministic(t *testing.T) {
	a := Fingerprint("proj_1", CategoryTestFailure, "FAIL TestLogin", AlgorithmVersion)
	b := Fingerprint("proj_1", CategoryTestFailure, "FAIL TestLogin", AlgorithmVersion)
	if a != b {
		t.Errorf("fingerprint not deterministic: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Errorf("fingerprint length = %d, want 64 hex chars", len(a))
	}
}

func TestFingerprint_FieldSensitivity(t *testing.T) {
	base := Fingerprint("proj_1", CategoryTestFailure, "sig", AlgorithmVersion)
	cases := map[string]string{
		"different project":   Fingerprint("proj_2", CategoryTestFailure, "sig", AlgorithmVersion),
		"different category":  Fingerprint("proj_1", CategoryCompileError, "sig", AlgorithmVersion),
		"different signature": Fingerprint("proj_1", CategoryTestFailure, "sig2", AlgorithmVersion),
		"different version":   Fingerprint("proj_1", CategoryTestFailure, "sig", AlgorithmVersion+1),
	}
	for name, fp := range cases {
		if fp == base {
			t.Errorf("%s produced the same fingerprint as the base", name)
		}
	}
}

// The separator must prevent field-boundary collisions.
func TestFingerprint_NoBoundaryCollision(t *testing.T) {
	a := Fingerprint("ab", Category("c"), "sig", AlgorithmVersion)
	b := Fingerprint("a", Category("bc"), "sig", AlgorithmVersion)
	if a == b {
		t.Error("field-boundary collision: ('ab','c') and ('a','bc') hashed equal")
	}
}

// Core product property: the same underlying failure in two runs (differing
// only in volatile tokens) must share one fingerprint.
func TestAnalyze_SameFailureConverges(t *testing.T) {
	runA := "--- FAIL: TestLogin (0.00s)\n    login_test.go:21: status = 500, want 200"
	runB := "--- FAIL: TestLogin (3.84s)\n    login_test.go:21: status = 500, want 200"

	da := Analyze("proj_1", runA)
	db := Analyze("proj_1", runB)

	if da.Category != CategoryTestFailure {
		t.Fatalf("category = %q, want test_failure", da.Category)
	}
	if da.Fingerprint != db.Fingerprint {
		t.Errorf("same failure diverged:\n  A sig=%q fp=%s\n  B sig=%q fp=%s",
			da.NormalizedSignature, da.Fingerprint, db.NormalizedSignature, db.Fingerprint)
	}
	if da.AlgorithmVersion != AlgorithmVersion {
		t.Errorf("algorithm version = %d, want %d", da.AlgorithmVersion, AlgorithmVersion)
	}
	if da.EvidenceHash == "" || len(da.EvidenceHash) != 64 {
		t.Errorf("evidence hash = %q, want 64 hex chars", da.EvidenceHash)
	}
}

func TestAnalyze_DistinctFailuresDiffer(t *testing.T) {
	login := Analyze("proj_1", "--- FAIL: TestLogin (0.0s)\n    want 200")
	logout := Analyze("proj_1", "--- FAIL: TestLogout (0.0s)\n    want 204")
	if login.Fingerprint == logout.Fingerprint {
		t.Error("distinct test failures produced the same fingerprint")
	}
}
