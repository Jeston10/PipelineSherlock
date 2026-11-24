package domain

import "sort"

// AnalyzeIncident assembles the full IncidentDetail from an incident header and
// its occurrence history. It is a pure function — no clock, no I/O, no
// randomness — so the same inputs always yield the same affected-ref breakdown
// and signals, in keeping with the project's deterministic-analysis guarantee.
//
// The occurrences are taken as given (the caller decides ordering and limit);
// AffectedRefs is returned in a stable order (most occurrences first, then ref
// name) so responses are reproducible.
func AnalyzeIncident(inc Incident, occurrences []IncidentOccurrence) IncidentDetail {
	return IncidentDetail{
		Incident:     inc,
		Occurrences:  occurrences,
		AffectedRefs: summarizeRefs(occurrences),
		Signals:      analyzeSignals(occurrences),
	}
}

// summarizeRefs groups occurrences by ref, counting occurrences and distinct
// commits per ref. The result is sorted by occurrence count (descending) then
// ref name (ascending) for a deterministic order.
func summarizeRefs(occurrences []IncidentOccurrence) []RefSummary {
	if len(occurrences) == 0 {
		return nil
	}

	type agg struct {
		count int
		shas  map[string]struct{}
	}
	byRef := make(map[string]*agg)
	order := make([]string, 0) // first-seen ref order, for stable tie-breaking input

	for _, o := range occurrences {
		a, ok := byRef[o.Ref]
		if !ok {
			a = &agg{shas: make(map[string]struct{})}
			byRef[o.Ref] = a
			order = append(order, o.Ref)
		}
		a.count++
		a.shas[o.SHA] = struct{}{}
	}

	out := make([]RefSummary, 0, len(byRef))
	for _, ref := range order {
		a := byRef[ref]
		out = append(out, RefSummary{
			Ref:             ref,
			OccurrenceCount: a.count,
			DistinctSHAs:    len(a.shas),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurrenceCount != out[j].OccurrenceCount {
			return out[i].OccurrenceCount > out[j].OccurrenceCount
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

// analyzeSignals derives the heuristic flaky / regression signals from the
// occurrence history. The signals are intentionally conservative and are
// documented as candidates for review, not conclusions.
func analyzeSignals(occurrences []IncidentOccurrence) IncidentSignals {
	refs := make(map[string]struct{})
	shas := make(map[string]struct{})
	shaCounts := make(map[string]int)

	for _, o := range occurrences {
		refs[o.Ref] = struct{}{}
		shas[o.SHA] = struct{}{}
		shaCounts[o.SHA]++
	}

	repeatedOnSameCommit := false
	for _, n := range shaCounts {
		if n >= 2 {
			repeatedOnSameCommit = true
			break
		}
	}
	spansMultipleCommits := len(shas) >= 2

	return IncidentSignals{
		DistinctRefs:         len(refs),
		DistinctSHAs:         len(shas),
		RepeatedOnSameCommit: repeatedOnSameCommit,
		SpansMultipleCommits: spansMultipleCommits,
		FlakySuspected:       repeatedOnSameCommit,
		RegressionSuspected:  spansMultipleCommits,
	}
}
