package domain

import "time"

// Incident is a read model: a group of failure occurrences that share one
// deterministic fingerprint. It is the user-facing unit of "this failure has
// happened N times since first seen", built by aggregating failure_occurrences
// under a failure_fingerprints row.
type Incident struct {
	FingerprintID       string
	ProjectID           ProjectID
	Fingerprint         string
	Category            string
	NormalizedSignature string
	AlgorithmVersion    int
	OccurrenceCount     int64
	FirstSeenAt         time.Time
	LastSeenAt          time.Time
}

// IncidentOccurrence is a single observed instance of a fingerprinted failure,
// enriched with the pipeline/job context it was seen in. It is a read model
// assembled by joining failure_occurrences to its job and pipeline.
type IncidentOccurrence struct {
	ObservedAt       time.Time
	Ref              string
	SHA              string
	JobName          string
	Stage            string
	GitLabJobID      int64
	GitLabPipelineID int64
	EvidenceExcerpt  string
}

// RefSummary aggregates one incident's occurrences on a single ref (branch or
// tag), so a reader can see where a failure concentrates.
type RefSummary struct {
	Ref             string
	OccurrenceCount int
	DistinctSHAs    int
}

// IncidentSignals are deterministic, heuristic flags computed purely from an
// incident's recorded failure occurrences. They are *candidates for review*,
// not verdicts: the system records only failures (not successes), so these
// signals describe the shape of the failure history rather than proving a
// test is flaky or a change is a regression.
type IncidentSignals struct {
	// DistinctRefs is the number of distinct refs the failure was seen on.
	DistinctRefs int
	// DistinctSHAs is the number of distinct commits the failure was seen on.
	DistinctSHAs int
	// RepeatedOnSameCommit is true when some single commit produced this
	// failure in more than one pipeline run — a nondeterminism (flaky) signal.
	RepeatedOnSameCommit bool
	// SpansMultipleCommits is true when the failure recurs across two or more
	// distinct commits — a persistence (regression) signal.
	SpansMultipleCommits bool
	// FlakySuspected mirrors RepeatedOnSameCommit: the same commit failed in
	// more than one run, which suggests the failure is nondeterministic.
	FlakySuspected bool
	// RegressionSuspected mirrors SpansMultipleCommits: the failure keeps
	// recurring across commits, which suggests a persistent regression.
	RegressionSuspected bool
}

// IncidentDetail is the full read model for a single incident: its header
// aggregate, the enriched occurrence history, a per-ref breakdown, and the
// heuristic signals.
type IncidentDetail struct {
	Incident
	Occurrences  []IncidentOccurrence
	AffectedRefs []RefSummary
	Signals      IncidentSignals
}
