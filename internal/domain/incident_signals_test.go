package domain

import (
	"testing"
	"time"
)

func occ(ref, sha string) IncidentOccurrence {
	return IncidentOccurrence{Ref: ref, SHA: sha, ObservedAt: time.Unix(0, 0).UTC()}
}

func TestAnalyzeIncident_Signals(t *testing.T) {
	tests := []struct {
		name            string
		occurrences     []IncidentOccurrence
		wantDistinctRef int
		wantDistinctSHA int
		wantFlaky       bool
		wantRegression  bool
	}{
		{
			name:            "single occurrence: neither signal",
			occurrences:     []IncidentOccurrence{occ("main", "aaa")},
			wantDistinctRef: 1,
			wantDistinctSHA: 1,
			wantFlaky:       false,
			wantRegression:  false,
		},
		{
			name:            "same commit twice: flaky candidate only",
			occurrences:     []IncidentOccurrence{occ("main", "aaa"), occ("main", "aaa")},
			wantDistinctRef: 1,
			wantDistinctSHA: 1,
			wantFlaky:       true,
			wantRegression:  false,
		},
		{
			name:            "two commits one ref: regression candidate only",
			occurrences:     []IncidentOccurrence{occ("main", "aaa"), occ("main", "bbb")},
			wantDistinctRef: 1,
			wantDistinctSHA: 2,
			wantFlaky:       false,
			wantRegression:  true,
		},
		{
			name: "repeat on one commit plus another commit: both signals",
			occurrences: []IncidentOccurrence{
				occ("main", "aaa"), occ("main", "aaa"), occ("feature", "ccc"),
			},
			wantDistinctRef: 2,
			wantDistinctSHA: 2,
			wantFlaky:       true,
			wantRegression:  true,
		},
		{
			name:            "no occurrences: zero signals",
			occurrences:     nil,
			wantDistinctRef: 0,
			wantDistinctSHA: 0,
			wantFlaky:       false,
			wantRegression:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeIncident(Incident{Fingerprint: "fp"}, tt.occurrences)

			if got.Signals.DistinctRefs != tt.wantDistinctRef {
				t.Errorf("DistinctRefs = %d, want %d", got.Signals.DistinctRefs, tt.wantDistinctRef)
			}
			if got.Signals.DistinctSHAs != tt.wantDistinctSHA {
				t.Errorf("DistinctSHAs = %d, want %d", got.Signals.DistinctSHAs, tt.wantDistinctSHA)
			}
			if got.Signals.FlakySuspected != tt.wantFlaky {
				t.Errorf("FlakySuspected = %v, want %v", got.Signals.FlakySuspected, tt.wantFlaky)
			}
			if got.Signals.RegressionSuspected != tt.wantRegression {
				t.Errorf("RegressionSuspected = %v, want %v", got.Signals.RegressionSuspected, tt.wantRegression)
			}
			// The header and occurrences pass through unchanged.
			if got.Fingerprint != "fp" {
				t.Errorf("header not preserved: Fingerprint = %q", got.Fingerprint)
			}
			if len(got.Occurrences) != len(tt.occurrences) {
				t.Errorf("Occurrences length = %d, want %d", len(got.Occurrences), len(tt.occurrences))
			}
		})
	}
}

func TestAnalyzeIncident_AffectedRefsSortedDeterministically(t *testing.T) {
	// feature has 1 occurrence, main has 3 (two distinct SHAs). main must come
	// first (more occurrences); ties would break by ref name ascending.
	occurrences := []IncidentOccurrence{
		occ("feature", "zzz"),
		occ("main", "aaa"),
		occ("main", "aaa"),
		occ("main", "bbb"),
	}

	got := AnalyzeIncident(Incident{}, occurrences)

	if len(got.AffectedRefs) != 2 {
		t.Fatalf("AffectedRefs length = %d, want 2", len(got.AffectedRefs))
	}
	if got.AffectedRefs[0].Ref != "main" {
		t.Errorf("AffectedRefs[0].Ref = %q, want main", got.AffectedRefs[0].Ref)
	}
	if got.AffectedRefs[0].OccurrenceCount != 3 || got.AffectedRefs[0].DistinctSHAs != 2 {
		t.Errorf("main summary = %+v, want count 3 / 2 distinct SHAs", got.AffectedRefs[0])
	}
	if got.AffectedRefs[1].Ref != "feature" || got.AffectedRefs[1].OccurrenceCount != 1 {
		t.Errorf("AffectedRefs[1] = %+v, want feature count 1", got.AffectedRefs[1])
	}
}
