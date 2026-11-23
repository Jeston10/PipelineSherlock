// Package analysis turns raw CI job traces into a deterministic failure
// diagnosis: a normalized signature, a category, and a stable fingerprint.
// Everything here is pure (no I/O, no clock, no randomness) so the same trace
// always yields the same fingerprint — that determinism is the whole point.
package analysis

import (
	"regexp"
	"strings"
)

// Volatile tokens are replaced with stable placeholders so that two runs of the
// same underlying failure normalize to an identical signature. Order matters:
// more specific patterns run before more general ones (e.g. durations before
// bare numbers, hex before numbers).
var (
	ansiRE      = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	uuidRE      = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	timestampRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`)
	addrRE      = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	durationRE  = regexp.MustCompile(`\b\d+(\.\d+)?(ns|µs|us|ms|s|m|h)\b`)
	hexRE       = regexp.MustCompile(`\b[0-9a-f]{7,}\b`)
	numberRE    = regexp.MustCompile(`\b\d+\b`)
	wsRE        = regexp.MustCompile(`\s+`)
)

// Normalize reduces raw log text to a stable signature by stripping ANSI codes
// and replacing volatile tokens (timestamps, durations, hashes, addresses,
// numbers) with placeholders, then collapsing whitespace. It is deterministic
// and idempotent: Normalize(Normalize(x)) == Normalize(x).
func Normalize(raw string) string {
	s := ansiRE.ReplaceAllString(raw, "")
	s = uuidRE.ReplaceAllString(s, "<UUID>")
	s = timestampRE.ReplaceAllString(s, "<TS>")
	s = addrRE.ReplaceAllString(s, "<ADDR>")
	s = durationRE.ReplaceAllString(s, "<DUR>")
	s = hexRE.ReplaceAllString(s, "<HEX>")
	s = numberRE.ReplaceAllString(s, "<N>")
	s = wsRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
