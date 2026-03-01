package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDistillSkipRecentHours       = 48
	defaultDistillMaxSourceRecords      = 500
	defaultDistillMaxSummaryTokens      = 220
	defaultDistillClusterSimilarity     = 0.82
	defaultDistillScoreThreshold        = 0.62
	defaultDistilledSearchMultiplier    = 1.25
	defaultExactPhraseDistillMultiplier = 1.05
	distillSearchWindow                 = 7 * 24 * time.Hour
)

var defaultDistillProtectedTags = map[string]struct{}{
	"user_preference": {},
	"promise":         {},
	"decision":        {},
	"commitment":      {},
	"policy":          {},
	"critical":        {},
	"pinned":          {},
	"immutable":       {},
}

type distillCandidate struct {
	record          MemoryRecord
	text            string
	tokens          []string
	ageDays         float64
	searchCount7d   int
	avgNeighborSim  float64
	ageScore        float64
	redundancyScore float64
	densityScore    float64
	conflictScore   float64
	searchFreqScore float64
	distillScore    float64
}

type distillCluster struct {
	scope      Scope
	candidates []distillCandidate
}

// DistillForInstance runs memory distillation for one instance/scope.
func (s *Store) DistillForInstance(ctx context.Context, opts InstanceDistillOptions) (DistillRunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeDistillOptions(opts)
	now := s.now()
	runID := "distill_" + shortDigest(fmt.Sprintf("%s|%s|%d", opts.InstanceID, opts.Scope, now.UnixNano()))
	result := DistillRunResult{
		RunID:        runID,
		InstanceID:   opts.InstanceID,
		Scope:        normalizeScope(opts.Scope),
		Status:       "running",
		Reason:       strings.TrimSpace(opts.Reason),
		DryRun:       opts.DryRun,
		StartedAt:    now,
		ScoreWeights: distillScoreWeights(opts),
	}

	lockKey := buildDistillLockKey(opts.InstanceID, opts.Scope)
	if err := s.acquireDistillLock(lockKey, runID); err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, err.Error())
		result.CompletedAt = s.now()
		result.DurationMs = maxDurationMS(result.StartedAt, result.CompletedAt)
		return result, err
	}
	defer s.releaseDistillLock(lockKey, runID)

	var (
		gitRoot   string
		gitBacked bool
	)
	rootDir := s.rootDirPath()
	if rootDir != "" {
		gitRoot, gitBacked = detectGitRepositoryRoot(rootDir)
	}
	if gitBacked && !opts.DryRun {
		if err := ensureGitWorkingTreeClean(gitRoot); err != nil {
			result.Status = "failed"
			result.Errors = append(result.Errors, err.Error())
			result.CompletedAt = s.now()
			result.DurationMs = maxDurationMS(result.StartedAt, result.CompletedAt)
			s.persistDistillRunResult(result)
			return result, err
		}
	}

	candidates, warn, err := s.collectDistillCandidates(opts)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, err.Error())
		result.CompletedAt = s.now()
		result.DurationMs = maxDurationMS(result.StartedAt, result.CompletedAt)
		s.persistDistillRunResult(result)
		return result, err
	}
	result.Warnings = append(result.Warnings, warn...)

	candidates, dedupRemoved := dedupeDistillCandidates(candidates)
	scoreAndFilterDistillCandidates(candidates, opts, result.ScoreWeights)
	qualified := make([]distillCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.distillScore >= opts.DistillScoreThreshold {
			qualified = append(qualified, c)
		}
	}

	clusters := buildDistillClusters(ctx, qualified, opts, s.distillEmbedder)
	result.Clustered = len(clusters)
	clusterSourceIDs := make([]string, 0)
	clusterMap := make(map[string][]string, len(clusters))
	distilled := make([]MemoryRecord, 0, len(clusters))
	for idx, cluster := range clusters {
		source := make([]string, 0, len(cluster.candidates))
		rawCluster := make([]MemoryRecord, 0, len(cluster.candidates))
		for _, c := range cluster.candidates {
			source = append(source, c.record.ID)
			rawCluster = append(rawCluster, c.record)
		}
		sort.Strings(source)
		clusterID := fmt.Sprintf("cluster-%d", idx+1)
		clusterMap[clusterID] = source
		clusterSourceIDs = append(clusterSourceIDs, source...)
		summary, summaryErr := s.distillSummaryForCluster(ctx, rawCluster, opts.MaxSummaryTokens)
		if summaryErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("cluster %s summarization fallback: %v", clusterID, summaryErr))
		}
		digest := distillSourceDigest(source)
		distilledID := "rec_" + shortDigest(fmt.Sprintf("%s|%s|%s", runID, cluster.scope, digest))
		rec := MemoryRecord{
			ID:             distilledID,
			Scope:          cluster.scope,
			Type:           RecordTypeNote,
			ContentSummary: summary,
			Tags: []string{
				"distill",
				"distill:v1",
				"distill_source_count:" + strconv.Itoa(len(source)),
				"distill_source_digest:" + digest,
			},
			Provenance: "distill:" + runID,
			Confidence: avgConfidence(rawCluster),
			Importance: maxImportance(rawCluster),
			CreatedAt:  s.now(),
			UpdatedAt:  s.now(),
		}
		distilled = append(distilled, rec)
	}
	sort.Strings(clusterSourceIDs)
	clusterSourceIDs = uniqueSortedStrings(clusterSourceIDs)
	dedupRemoved = uniqueSortedStrings(dedupRemoved)
	consumedSources := uniqueSortedStrings(append(append([]string{}, clusterSourceIDs...), dedupRemoved...))

	result.Planned = len(consumedSources)
	result.Created = len(distilled)
	result.Removed = len(consumedSources)
	result.Unchanged = maxInt(0, len(candidates)-len(clusterSourceIDs))
	result.SampleSource = takeFirstN(consumedSources, 5)
	for _, rec := range distilled {
		result.SampleOutput = append(result.SampleOutput, rec.ID)
	}
	result.SampleOutput = takeFirstN(result.SampleOutput, 5)

	if opts.DryRun {
		result.Status = "planned"
		result.CompletedAt = s.now()
		result.DurationMs = maxDurationMS(result.StartedAt, result.CompletedAt)
		s.persistDistillRunResult(result)
		return result, nil
	}

	manifest := DistillSourceManifest{
		RunID:        runID,
		InstanceID:   opts.InstanceID,
		Scope:        normalizeScope(opts.Scope),
		CreatedAt:    s.now(),
		SourceIDs:    consumedSources,
		ClusterMap:   clusterMap,
		SourceDigest: distillSourceDigest(consumedSources),
	}
	manifestRef, manifestStore, applyErr := s.applyDistillMutation(result, distilled, consumedSources, manifest, gitBacked, gitRoot)
	if applyErr != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, applyErr.Error())
		result.CompletedAt = s.now()
		result.DurationMs = maxDurationMS(result.StartedAt, result.CompletedAt)
		s.persistDistillRunResult(result)
		return result, applyErr
	}
	result.ManifestRef = manifestRef
	result.ManifestStore = manifestStore
	result.Status = "completed"
	result.CompletedAt = s.now()
	result.DurationMs = maxDurationMS(result.StartedAt, result.CompletedAt)
	s.persistDistillRunResult(result)
	s.recordAudit(opts.RequestID, opts.Actor, "instance_distill", opts.InstanceID, auditResultSuccess, fmt.Sprintf("run=%s removed=%d created=%d", result.RunID, result.Removed, result.Created))
	return result, nil
}

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
		if !scopeAllowed(allowed, opts.Scope) && !opts.Force {
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
		if !lastUpdated.IsZero() && lastUpdated.After(cutoff) {
			continue
		}
		if opts.MinSourceAgeDays > 0 && !lastUpdated.IsZero() && lastUpdated.After(minAgeCutoff) {
			continue
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
	minAge := math.MaxFloat64
	searchCounts := make([]float64, 0, len(candidates))
	for _, c := range candidates {
		if c.ageDays > maxAge {
			maxAge = c.ageDays
		}
		if c.ageDays < minAge {
			minAge = c.ageDays
		}
		searchCounts = append(searchCounts, float64(c.searchCount7d))
	}
	if minAge == math.MaxFloat64 {
		minAge = 0
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

func (s *Store) distillSummaryForCluster(ctx context.Context, cluster []MemoryRecord, maxSummaryTokens int) (string, error) {
	if len(cluster) == 0 {
		return "", nil
	}
	if s.distillSummarizer != nil {
		out, err := s.distillSummarizer(ctx, cluster, maxSummaryTokens)
		out = strings.TrimSpace(out)
		if err == nil && out != "" {
			return truncateByWords(out, maxSummaryTokens), nil
		}
		return truncateByWords(defaultDistillSummary(cluster), maxSummaryTokens), err
	}
	return truncateByWords(defaultDistillSummary(cluster), maxSummaryTokens), nil
}

func defaultDistillSummary(cluster []MemoryRecord) string {
	parts := make([]string, 0, len(cluster))
	for _, rec := range cluster {
		text := distillRecordText(rec)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return ""
	}
	sort.SliceStable(parts, func(i, j int) bool {
		return len(parts[i]) > len(parts[j])
	})
	primary := parts[0]
	if len(parts) == 1 {
		return primary
	}
	return primary + " [distilled from " + strconv.Itoa(len(parts)) + " related records]"
}

func (s *Store) applyDistillMutation(
	result DistillRunResult,
	distilled []MemoryRecord,
	consumedSources []string,
	manifest DistillSourceManifest,
	gitBacked bool,
	gitRoot string,
) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, rec := range distilled {
		s.records[rec.ID] = rec
		s.syncRecordToSQLiteLocked(rec)
		if err := s.writeStableTruthRecordLocked(rec); err != nil {
			return "", "", err
		}
	}
	for _, id := range consumedSources {
		delete(s.records, id)
		s.deleteRecordFromSQLiteLocked(id)
	}

	manifestRef := ""
	manifestStore := "sqlite"
	s.distillManifests[manifest.RunID] = manifest
	s.syncDistillManifestToSQLiteLocked(manifest)
	if gitBacked {
		refPath := filepath.Join(gitRoot, ".carrier", "distill", "manifests", manifest.RunID+".json")
		if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
			return "", "", err
		}
		raw, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(refPath, raw, 0o644); err != nil {
			return "", "", err
		}
		manifestRef = refPath
		manifestStore = "git"
	}
	result.ManifestRef = manifestRef
	result.ManifestStore = manifestStore
	s.distillRuns[result.RunID] = result
	if err := s.persistStateLocked(); err != nil {
		return "", "", err
	}
	return manifestRef, manifestStore, nil
}

func (s *Store) persistDistillRunResult(result DistillRunResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.distillRuns[result.RunID] = result
	_ = s.persistStateLocked()
}

func (s *Store) rootDirPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.rootDir)
}

func (s *Store) recordSearchHits(recordIDs []string) {
	if len(recordIDs) == 0 {
		return
	}
	now := s.now()
	cutoff := now.Add(-distillSearchWindow)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range recordIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		existing := append(s.searchHits[id], now)
		pruned := existing[:0]
		for _, ts := range existing {
			if ts.After(cutoff) {
				pruned = append(pruned, ts)
			}
		}
		s.searchHits[id] = append([]time.Time(nil), pruned...)
	}
}

func (s *Store) acquireDistillLock(lockKey, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingRun, exists := s.activeDistillRuns[lockKey]; exists && existingRun != "" {
		return ErrDistillBusy
	}
	s.activeDistillRuns[lockKey] = runID
	return nil
}

func (s *Store) releaseDistillLock(lockKey, runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.activeDistillRuns[lockKey]; current == runID {
		delete(s.activeDistillRuns, lockKey)
	}
}

func buildDistillLockKey(instanceID string, scope Scope) string {
	scope = normalizeScope(scope)
	if scope != "" {
		return fmt.Sprintf("distill:%s", scope)
	}
	return fmt.Sprintf("distill:instance:%s", strings.TrimSpace(instanceID))
}

func distillRecordText(rec MemoryRecord) string {
	text := strings.TrimSpace(rec.ContentSummary)
	if text != "" {
		return text
	}
	return strings.TrimSpace(rec.ContentRaw)
}

func canonicalContentSignature(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}
	normalized := strings.Join(tokenizeText(lower), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func countSearchHitsWithin(samples []time.Time, now time.Time, window time.Duration) int {
	if len(samples) == 0 {
		return 0
	}
	cutoff := now.Add(-window)
	count := 0
	for _, ts := range samples {
		if ts.After(cutoff) {
			count++
		}
	}
	return count
}

func isProtectedRecord(rec MemoryRecord) bool {
	for _, tag := range rec.Tags {
		if _, protected := defaultDistillProtectedTags[strings.ToLower(strings.TrimSpace(tag))]; protected {
			return true
		}
	}
	return false
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

func distillSourceDigest(sourceIDs []string) string {
	ids := append([]string(nil), sourceIDs...)
	sort.Strings(ids)
	return shortDigest(strings.Join(ids, ","))
}

func avgConfidence(cluster []MemoryRecord) float64 {
	if len(cluster) == 0 {
		return 0
	}
	sum := 0.0
	for _, rec := range cluster {
		sum += rec.Confidence
	}
	return sum / float64(len(cluster))
}

func maxImportance(cluster []MemoryRecord) int {
	maxV := 0
	for _, rec := range cluster {
		if rec.Importance > maxV {
			maxV = rec.Importance
		}
	}
	return maxV
}

func detectGitRepositoryRoot(rootDir string) (string, bool) {
	if strings.TrimSpace(rootDir) == "" {
		return "", false
	}
	cmd := exec.Command("git", "-C", rootDir, "rev-parse", "--show-toplevel")
	raw, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(raw))
	if root == "" {
		return "", false
	}
	return root, true
}

func ensureGitWorkingTreeClean(gitRoot string) error {
	cmd := exec.Command("git", "-C", gitRoot, "status", "--porcelain")
	raw, err := cmd.Output()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(raw)) != "" {
		return ErrDistillDirtyTree
	}
	return nil
}

func truncateByWords(text string, maxWords int) string {
	if maxWords <= 0 {
		return strings.TrimSpace(text)
	}
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) <= maxWords {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts[:maxWords], " ") + " ..."
}

func takeFirstN(in []string, n int) []string {
	if n <= 0 || len(in) == 0 {
		return nil
	}
	if len(in) <= n {
		out := make([]string, len(in))
		copy(out, in)
		return out
	}
	out := make([]string, n)
	copy(out, in[:n])
	return out
}

func uniqueSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:0]
	var prev string
	for i, item := range in {
		if i == 0 || item != prev {
			out = append(out, item)
			prev = item
		}
	}
	return append([]string(nil), out...)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxDurationMS(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func isDistilledRecord(rec MemoryRecord) bool {
	if strings.HasPrefix(strings.TrimSpace(rec.Provenance), "distill:") {
		return true
	}
	for _, tag := range rec.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), "distill") {
			return true
		}
	}
	return false
}

func distilledScoreMultiplier(query string, isDistilled bool) float64 {
	if !isDistilled {
		return 1.0
	}
	if isExactPhraseQuery(query) {
		return defaultExactPhraseDistillMultiplier
	}
	return defaultDistilledSearchMultiplier
}

func isExactPhraseQuery(query string) bool {
	query = strings.TrimSpace(query)
	return strings.Contains(query, "\"")
}
