package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func normalizeDistillOptions(opts InstanceDistillOptions) InstanceDistillOptions {
	opts.InstanceID = strings.TrimSpace(opts.InstanceID)
	opts.Scope = normalizeScope(opts.Scope)
	opts.Actor = strings.TrimSpace(opts.Actor)
	opts.RequestID = strings.TrimSpace(opts.RequestID)
	opts.Reason = strings.TrimSpace(opts.Reason)
	if opts.SkipRecentHours <= 0 {
		opts.SkipRecentHours = defaultDistillSkipRecentHours
	}
	if opts.MaxSourceRecords <= 0 {
		opts.MaxSourceRecords = defaultDistillMaxSourceRecords
	}
	if opts.MaxSummaryTokens <= 0 {
		opts.MaxSummaryTokens = defaultDistillMaxSummaryTokens
	}
	if opts.ClusterSimilarityThreshold <= 0 {
		opts.ClusterSimilarityThreshold = defaultDistillClusterSimilarity
	}
	if opts.ClusterSimilarityThreshold > 1 {
		opts.ClusterSimilarityThreshold = 1
	}
	if opts.DistillScoreThreshold <= 0 {
		opts.DistillScoreThreshold = defaultDistillScoreThreshold
	}
	if opts.DistillScoreThreshold > 1 {
		opts.DistillScoreThreshold = 1
	}
	return opts
}

func distillScoreWeights(opts InstanceDistillOptions) DistillScoreWeights {
	if opts.DistillScoreWeightsOverride != nil {
		w := *opts.DistillScoreWeightsOverride
		sum := w.Age + w.Redundancy + w.Density + w.Conflict + w.SearchFreq
		if sum > 0 {
			w.Age /= sum
			w.Redundancy /= sum
			w.Density /= sum
			w.Conflict /= sum
			w.SearchFreq /= sum
			return w
		}
	}
	return DistillScoreWeights{
		Age:        0.30,
		Redundancy: 0.25,
		Density:    0.20,
		Conflict:   0.15,
		SearchFreq: 0.10,
	}
}

func (s *Store) collectDistillCandidates(opts InstanceDistillOptions) ([]distillCandidate, []string, error) {
	if opts.InstanceID == "" {
		return nil, nil, fmt.Errorf("instanceId is required")
	}
	now := s.now()
	cutoff := now.Add(-time.Duration(opts.SkipRecentHours) * time.Hour)
	minAgeCutoff := now.Add(-time.Duration(opts.MinSourceAgeDays) * 24 * time.Hour)

	s.mu.RLock()
	defer s.mu.RUnlock()

	allowed := s.allowedScopesForSubjectLocked(opts.InstanceID)
	if opts.Scope != "" {
		if !scopeAllowed(allowed, opts.Scope) {
			return nil, nil, ErrMountDenied
		}
		allowed = map[Scope]struct{}{opts.Scope: {}}
	}

	candidates := make([]distillCandidate, 0, len(s.records))
	for _, rec := range s.records {
		if rec.ArchivedAt != nil {
			continue
		}
		if !scopeAllowed(allowed, rec.Scope) {
			continue
		}
		if !opts.Force && isProtectedRecord(rec) {
			continue
		}
		lastUpdated := rec.UpdatedAt
		if lastUpdated.IsZero() {
			lastUpdated = rec.CreatedAt
		}
		if !opts.Force {
			if !lastUpdated.IsZero() && lastUpdated.After(cutoff) {
				continue
			}
			if opts.MinSourceAgeDays > 0 && !lastUpdated.IsZero() && lastUpdated.After(minAgeCutoff) {
				continue
			}
		}
		text := distillRecordText(rec)
		if text == "" {
			continue
		}
		ageDays := 0.0
		if !lastUpdated.IsZero() {
			ageDays = now.Sub(lastUpdated).Hours() / 24
		}
		candidates = append(candidates, distillCandidate{
			record:        rec,
			text:          text,
			tokens:        tokenizeText(text),
			ageDays:       ageDays,
			searchCount7d: countSearchHitsWithin(s.searchHits[rec.ID], now, distillSearchWindow),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].record.UpdatedAt.Before(candidates[j].record.UpdatedAt)
	})
	if len(candidates) > opts.MaxSourceRecords {
		candidates = candidates[:opts.MaxSourceRecords]
	}
	warnings := make([]string, 0, 1)
	if len(candidates) == 0 {
		warnings = append(warnings, "no candidates matched filters")
	}
	return candidates, warnings, nil
}

func dedupeDistillCandidates(candidates []distillCandidate) ([]distillCandidate, []string) {
	seen := make(map[string]int, len(candidates))
	kept := make([]distillCandidate, 0, len(candidates))
	removed := make([]string, 0)
	for _, candidate := range candidates {
		sig := canonicalContentSignature(candidate.text)
		if sig == "" {
			sig = canonicalContentSignature(candidate.record.ContentRaw + " " + candidate.record.ContentSummary)
		}
		if sig == "" {
			sig = candidate.record.ID
		}
		if _, exists := seen[sig]; exists {
			removed = append(removed, candidate.record.ID)
			continue
		}
		seen[sig] = len(kept)
		kept = append(kept, candidate)
	}
	return kept, removed
}

func scoreAndFilterDistillCandidates(candidates []distillCandidate, opts InstanceDistillOptions, weights DistillScoreWeights) {
	if len(candidates) == 0 {
		return
	}
	maxAge := 0.0
	searchCounts := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		if c.ageDays > maxAge {
			maxAge = c.ageDays
		}
		searchCounts = append(searchCounts, float64(c.searchCount7d))
	}
	p95Search := percentile(searchCounts, 95)
	scopeGroups := make(map[Scope][]int)
	for idx := range candidates {
		scopeGroups[candidates[idx].record.Scope] = append(scopeGroups[candidates[idx].record.Scope], idx)
	}
	for _, group := range scopeGroups {
		for _, i := range group {
			neighbor := make([]float64, 0, len(group)-1)
			for _, j := range group {
				if i == j {
					continue
				}
				neighbor = append(neighbor, lexicalSemanticSimilarity(candidates[i].text, candidates[j].text))
			}
			candidates[i].avgNeighborSim = meanTopN(neighbor, 5)
		}
	}
	minAgeNorm := float64(opts.MinSourceAgeDays)
	maxAgeNorm := maxFloat(maxAge, minAgeNorm+1)
	for idx := range candidates {
		c := &candidates[idx]
		c.ageScore = clamp((c.ageDays-minAgeNorm)/(maxAgeNorm-minAgeNorm), 0, 1)
		c.redundancyScore = clamp((c.avgNeighborSim-0.3)/(0.95-0.3), 0, 1)
		c.densityScore = densityScore(c.tokens)
		c.conflictScore = conflictScore(c.tokens)
		if p95Search <= 0 {
			c.searchFreqScore = 1
		} else {
			c.searchFreqScore = 1 - clamp(float64(c.searchCount7d)/p95Search, 0, 1)
		}
		c.distillScore = weights.Age*c.ageScore +
			weights.Redundancy*c.redundancyScore +
			weights.Density*c.densityScore +
			weights.Conflict*c.conflictScore +
			weights.SearchFreq*c.searchFreqScore
	}
}

func buildDistillClusters(
	ctx context.Context,
	candidates []distillCandidate,
	opts InstanceDistillOptions,
	embedder DistillEmbedderFunc,
) []distillCluster {
	if len(candidates) == 0 {
		return nil
	}
	scopeGroups := make(map[Scope][]distillCandidate)
	for _, candidate := range candidates {
		scopeGroups[candidate.record.Scope] = append(scopeGroups[candidate.record.Scope], candidate)
	}
	out := make([]distillCluster, 0)
	for scope, group := range scopeGroups {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].distillScore == group[j].distillScore {
				return group[i].record.ID < group[j].record.ID
			}
			return group[i].distillScore > group[j].distillScore
		})
		vectors := map[string][]float64{}
		if embedder != nil {
			for _, g := range group {
				vec, err := embedder(ctx, g.text)
				if err == nil && len(vec) > 0 {
					vectors[g.record.ID] = vec
				}
			}
		}
		visited := make([]bool, len(group))
		for i := range group {
			if visited[i] {
				continue
			}
			cluster := []distillCandidate{group[i]}
			visited[i] = true
			for j := i + 1; j < len(group); j++ {
				if visited[j] {
					continue
				}
				score := candidateSimilarity(group[i], group[j], vectors)
				if score >= opts.ClusterSimilarityThreshold {
					cluster = append(cluster, group[j])
					visited[j] = true
				}
			}
			if len(cluster) < 2 {
				continue
			}
			out = append(out, distillCluster{
				scope:      scope,
				candidates: cluster,
			})
		}
	}
	return out
}

func lexicalSemanticSimilarity(a, b string) float64 {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return 0
	}
	at := tokenizeText(a)
	bt := tokenizeText(b)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(at))
	setB := make(map[string]struct{}, len(bt))
	for _, t := range at {
		setA[t] = struct{}{}
	}
	for _, t := range bt {
		setB[t] = struct{}{}
	}
	inter := 0
	for token := range setA {
		if _, ok := setB[token]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union <= 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func candidateSimilarity(a, b distillCandidate, vectors map[string][]float64) float64 {
	vecA := vectors[a.record.ID]
	vecB := vectors[b.record.ID]
	if len(vecA) > 0 && len(vecA) == len(vecB) {
		return cosineSimilarity(vecA, vecB)
	}
	return lexicalSemanticSimilarity(a.text, b.text)
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return clamp(dot/(math.Sqrt(normA)*math.Sqrt(normB)), 0, 1)
}

func densityScore(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0
	}
	unique := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		unique[t] = struct{}{}
	}
	density := float64(len(unique)) / float64(len(tokens))
	return clamp(density/0.65, 0, 1)
}

func conflictScore(tokens []string) float64 {
	if len(tokens) == 0 {
		return 1
	}
	conflictWords := map[string]struct{}{
		"but":          {},
		"however":      {},
		"conflict":     {},
		"uncertain":    {},
		"inconsistent": {},
		"contradict":   {},
	}
	conflicts := 0
	for _, token := range tokens {
		if _, ok := conflictWords[token]; ok {
			conflicts++
		}
	}
	ratio := float64(conflicts) / float64(len(tokens))
	return 1 - clamp(ratio, 0, 1)
}

func meanTopN(values []float64, n int) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.SliceStable(values, func(i, j int) bool { return values[i] > values[j] })
	if n <= 0 || n > len(values) {
		n = len(values)
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += values[i]
	}
	return sum / float64(n)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	pos := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return sorted[lower]
	}
	ratio := pos - float64(lower)
	return sorted[lower] + (sorted[upper]-sorted[lower])*ratio
}
