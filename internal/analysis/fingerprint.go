package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
)

// AlgorithmVersion identifies the fingerprinting algorithm. It is stored
// alongside each fingerprint so the signature scheme can evolve without
// colliding with fingerprints produced by an earlier version.
const AlgorithmVersion = 1

// Diagnosis is the full deterministic result of analysing one failed job trace.
type Diagnosis struct {
	Category            Category
	NormalizedSignature string
	Fingerprint         string
	Evidence            string
	EvidenceHash        string
	AlgorithmVersion    int
}

// fieldSep separates fields in the fingerprint pre-image so that, e.g.,
// ("a","bc") and ("ab","c") cannot produce the same hash.
var fieldSep = []byte{0}

// Fingerprint computes a stable hex SHA-256 over the project, category,
// normalized signature and algorithm version. The same inputs always produce
// the same fingerprint; any field differing produces a different one.
func Fingerprint(projectID string, category Category, normalizedSignature string, version int) string {
	h := sha256.New()
	io.WriteString(h, projectID)
	h.Write(fieldSep)
	io.WriteString(h, string(category))
	h.Write(fieldSep)
	io.WriteString(h, normalizedSignature)
	h.Write(fieldSep)
	io.WriteString(h, "v"+strconv.Itoa(version))
	return hex.EncodeToString(h.Sum(nil))
}

// Analyze is the package entry point: it classifies the trace, normalizes the
// salient line into a signature, and computes the fingerprint and an evidence
// hash. It is pure and deterministic.
func Analyze(projectID, rawTrace string) Diagnosis {
	c := Classify(rawTrace)
	sig := Normalize(c.SignatureLine)
	return Diagnosis{
		Category:            c.Category,
		NormalizedSignature: sig,
		Fingerprint:         Fingerprint(projectID, c.Category, sig, AlgorithmVersion),
		Evidence:            c.Evidence,
		EvidenceHash:        hashString(c.Evidence),
		AlgorithmVersion:    AlgorithmVersion,
	}
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
