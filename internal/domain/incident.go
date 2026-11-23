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
