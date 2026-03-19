package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	for _, rec := range distilled {
		result.OutputIDs = append(result.OutputIDs, rec.ID)
	}
	sort.Strings(result.OutputIDs)
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
		raw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", "", fmt.Errorf("marshal distill manifest: %w", err)
		}
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
