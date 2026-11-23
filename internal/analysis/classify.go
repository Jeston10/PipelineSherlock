package analysis

import (
	"regexp"
	"strings"
)

// Category is a coarse, deterministic classification of a CI failure.
type Category string

const (
	CategoryOOM             Category = "oom"
	CategoryTimeout         Category = "timeout"
	CategoryInfraError      Category = "infra_error"
	CategoryDependencyError Category = "dependency_error"
	CategoryCompileError    Category = "compile_error"
	CategoryLintError       Category = "lint_error"
	CategoryTestFailure     Category = "test_failure"
	CategoryUnknown         Category = "unknown"
)

// Classification is the result of inspecting a trace.
type Classification struct {
	Category Category
	// SignatureLine is the single most salient raw line (the one that matched a
	// rule). Its normalization becomes the fingerprint signature.
	SignatureLine string
	// Evidence is a short human-readable excerpt (the matched line plus a little
	// following context), for display and storage.
	Evidence string
}

const (
	maxEvidenceLen   = 1000
	evidenceFollowUp = 2 // lines of context captured after the matched line
)

// rule couples a category with a case-insensitive pattern. Rules are evaluated
// in priority order: environmental root causes (OOM, timeout, infra) before
// build/test symptoms, because a symptom is often downstream of the real cause.
type rule struct {
	category Category
	re       *regexp.Regexp
}

var rules = []rule{
	{CategoryOOM, regexp.MustCompile(`(?i)(out of memory|oomkilled|cannot allocate memory|runtime: out of memory|exit code 137)`)},
	{CategoryTimeout, regexp.MustCompile(`(?i)(timed out|timeout|deadline exceeded|context deadline|exceeded the time limit)`)},
	{CategoryInfraError, regexp.MustCompile(`(?i)(connection refused|no space left on device|could not resolve host|dial tcp|network is unreachable|temporary failure in name resolution|i/o timeout|503 service)`)},
	{CategoryDependencyError, regexp.MustCompile(`(?i)(could not find a version|no matching version|eresolve|could not resolve dependency|unknown revision|cannot find package|module .* not found|npm err!.*404)`)},
	{CategoryCompileError, regexp.MustCompile(`(?i)(syntaxerror|undefined:|cannot find name|error ts\d+|compilation failed|compile error|cannot use .* as |does not implement |expected .*, found )`)},
	{CategoryLintError, regexp.MustCompile(`(?i)(eslint|golangci-lint|gofmt|lint error|lint failed|prettier)`)},
	{CategoryTestFailure, regexp.MustCompile(`(?i)(--- fail|\bfail\b|assertionerror|expected .* to (be|equal)|tests? failed|✗|panic: .*test)`)},
}

// Classify inspects raw trace text and returns the first matching category by
// priority. When nothing matches it returns CategoryUnknown with the last
// non-empty line as the signature, so there is always something to fingerprint.
func Classify(raw string) Classification {
	lines := strings.Split(raw, "\n")

	for _, r := range rules {
		if idx := firstMatch(lines, r.re); idx >= 0 {
			return Classification{
				Category:      r.category,
				SignatureLine: strings.TrimSpace(lines[idx]),
				Evidence:      excerpt(lines, idx),
			}
		}
	}

	// Fallback: use the last non-empty line.
	if idx := lastNonEmpty(lines); idx >= 0 {
		return Classification{
			Category:      CategoryUnknown,
			SignatureLine: strings.TrimSpace(lines[idx]),
			Evidence:      excerpt(lines, idx),
		}
	}
	return Classification{Category: CategoryUnknown}
}

func firstMatch(lines []string, re *regexp.Regexp) int {
	for i, line := range lines {
		if re.MatchString(line) {
			return i
		}
	}
	return -1
}

func lastNonEmpty(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

// excerpt returns the matched line plus up to evidenceFollowUp following
// non-empty lines, trimmed and length-capped.
func excerpt(lines []string, idx int) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(lines[idx]))
	added := 0
	for i := idx + 1; i < len(lines) && added < evidenceFollowUp; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(t)
		added++
	}
	out := b.String()
	if len(out) > maxEvidenceLen {
		out = out[:maxEvidenceLen]
	}
	return out
}
