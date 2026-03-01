package memory

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestDistillScoreWeightsNormalization(t *testing.T) {
	defaultW := distillScoreWeights(InstanceDistillOptions{})
	if !almostEqual(defaultW.Age, 0.30) || !almostEqual(defaultW.Redundancy, 0.25) || !almostEqual(defaultW.Density, 0.20) || !almostEqual(defaultW.Conflict, 0.15) || !almostEqual(defaultW.SearchFreq, 0.10) {
		t.Fatalf("unexpected defaults: %+v", defaultW)
	}

	override := distillScoreWeights(InstanceDistillOptions{
		DistillScoreWeightsOverride: &DistillScoreWeights{
			Age:        2,
			Redundancy: 1,
			Density:    1,
			Conflict:   0,
			SearchFreq: 0,
		},
	})
	if !almostEqual(override.Age, 0.5) || !almostEqual(override.Redundancy, 0.25) || !almostEqual(override.Density, 0.25) || !almostEqual(override.Conflict, 0) || !almostEqual(override.SearchFreq, 0) {
		t.Fatalf("unexpected normalized override: %+v", override)
	}

	zeroOverride := distillScoreWeights(InstanceDistillOptions{
		DistillScoreWeightsOverride: &DistillScoreWeights{},
	})
	if !almostEqual(zeroOverride.Age, defaultW.Age) || !almostEqual(zeroOverride.Redundancy, defaultW.Redundancy) || !almostEqual(zeroOverride.Density, defaultW.Density) || !almostEqual(zeroOverride.Conflict, defaultW.Conflict) || !almostEqual(zeroOverride.SearchFreq, defaultW.SearchFreq) {
		t.Fatalf("expected fallback defaults for zero override, got %+v", zeroOverride)
	}
}

func TestBuildDistillClustersByScopeAndSimilarity(t *testing.T) {
	opts := InstanceDistillOptions{ClusterSimilarityThreshold: 0.5}
	candidates := []distillCandidate{
		{
			record:       MemoryRecord{ID: "a1", Scope: Scope("agent:a")},
			text:         "deploy tokyo region",
			distillScore: 0.9,
		},
		{
			record:       MemoryRecord{ID: "a2", Scope: Scope("agent:a")},
			text:         "deploy tokyo region rollout",
			distillScore: 0.8,
		},
		{
			record:       MemoryRecord{ID: "a3", Scope: Scope("agent:a")},
			text:         "unrelated banana entry",
			distillScore: 0.7,
		},
		{
			record:       MemoryRecord{ID: "b1", Scope: Scope("shared:ops")},
			text:         "billing retry policy",
			distillScore: 0.95,
		},
		{
			record:       MemoryRecord{ID: "b2", Scope: Scope("shared:ops")},
			text:         "billing retry policy update",
			distillScore: 0.85,
		},
	}

	clusters := buildDistillClusters(context.Background(), candidates, opts, nil)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	byScope := map[Scope]int{}
	for _, cluster := range clusters {
		byScope[cluster.scope] = len(cluster.candidates)
	}
	if byScope[Scope("agent:a")] != 2 {
		t.Fatalf("expected agent:a cluster size 2, got %d", byScope[Scope("agent:a")])
	}
	if byScope[Scope("shared:ops")] != 2 {
		t.Fatalf("expected shared:ops cluster size 2, got %d", byScope[Scope("shared:ops")])
	}
}

func TestBuildDistillClustersUsesEmbedderVectors(t *testing.T) {
	opts := InstanceDistillOptions{ClusterSimilarityThreshold: 0.95}
	candidates := []distillCandidate{
		{
			record:       MemoryRecord{ID: "x1", Scope: Scope("agent:x")},
			text:         "alpha",
			distillScore: 0.9,
		},
		{
			record:       MemoryRecord{ID: "x2", Scope: Scope("agent:x")},
			text:         "beta",
			distillScore: 0.8,
		},
	}
	embedder := func(_ context.Context, _ string) ([]float64, error) {
		return []float64{1, 0}, nil
	}

	clusters := buildDistillClusters(context.Background(), candidates, opts, embedder)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if len(clusters[0].candidates) != 2 {
		t.Fatalf("expected clustered pair, got %d", len(clusters[0].candidates))
	}
}

func TestDistillSummaryForClusterPaths(t *testing.T) {
	cluster := []MemoryRecord{
		{ID: "r1", ContentSummary: "deployment target is tokyo region"},
		{ID: "r2", ContentSummary: "tokyo region deployment approved"},
	}

	okStore := NewStore(
		WithRootDir(t.TempDir()),
		WithDistillSummarizer(func(_ context.Context, _ []MemoryRecord, _ int) (string, error) {
			return "one two three four", nil
		}),
	)
	okOut, okErr := okStore.distillSummaryForCluster(context.Background(), cluster, 2)
	if okErr != nil {
		t.Fatalf("expected nil err, got %v", okErr)
	}
	if okOut != "one two ..." {
		t.Fatalf("unexpected summarizer output: %q", okOut)
	}

	fallbackStore := NewStore(
		WithRootDir(t.TempDir()),
		WithDistillSummarizer(func(_ context.Context, _ []MemoryRecord, _ int) (string, error) {
			return "", errors.New("boom")
		}),
	)
	fallbackOut, fallbackErr := fallbackStore.distillSummaryForCluster(context.Background(), cluster, 30)
	if fallbackErr == nil {
		t.Fatalf("expected summarizer error for fallback path")
	}
	if !strings.Contains(fallbackOut, "distilled from 2 related records") {
		t.Fatalf("expected fallback summary marker, got %q", fallbackOut)
	}

	plainStore := NewStore(WithRootDir(t.TempDir()))
	plainOut, plainErr := plainStore.distillSummaryForCluster(context.Background(), cluster, 30)
	if plainErr != nil {
		t.Fatalf("expected nil err for default summary, got %v", plainErr)
	}
	if !strings.Contains(plainOut, "distilled from 2 related records") {
		t.Fatalf("expected default summary marker, got %q", plainOut)
	}
}

func TestDistillHelperFunctions(t *testing.T) {
	rec := MemoryRecord{
		ContentSummary: "  summary  ",
		ContentRaw:     "raw",
		Tags:           []string{"Decision"},
		Provenance:     "distill:run-1",
	}
	if got := distillRecordText(rec); got != "summary" {
		t.Fatalf("unexpected distillRecordText: %q", got)
	}
	if !isProtectedRecord(rec) {
		t.Fatalf("expected protected record")
	}
	if !isDistilledRecord(rec) {
		t.Fatalf("expected distilled record by provenance")
	}

	if lexicalSemanticSimilarity("", "x") != 0 {
		t.Fatalf("expected zero similarity for empty input")
	}
	lexical := lexicalSemanticSimilarity("deploy tokyo region", "deploy tokyo rollout")
	if lexical <= 0 {
		t.Fatalf("expected lexical similarity > 0, got %f", lexical)
	}

	if cosineSimilarity([]float64{1, 0}, []float64{1, 0}) != 1 {
		t.Fatalf("expected cosine similarity 1 for identical vectors")
	}
	if cosineSimilarity([]float64{1}, []float64{}) != 0 {
		t.Fatalf("expected cosine 0 for mismatched vectors")
	}
	if cosineSimilarity([]float64{0, 0}, []float64{0, 0}) != 0 {
		t.Fatalf("expected cosine 0 for zero vectors")
	}

	a := distillCandidate{record: MemoryRecord{ID: "a"}, text: "alpha beta"}
	b := distillCandidate{record: MemoryRecord{ID: "b"}, text: "alpha gamma"}
	scoreVec := candidateSimilarity(a, b, map[string][]float64{
		"a": []float64{1, 0},
		"b": []float64{1, 0},
	})
	if !almostEqual(scoreVec, 1) {
		t.Fatalf("expected vector similarity 1, got %f", scoreVec)
	}
	scoreLexical := candidateSimilarity(a, b, map[string][]float64{})
	if scoreLexical <= 0 {
		t.Fatalf("expected lexical fallback similarity > 0, got %f", scoreLexical)
	}

	if densityScore(nil) != 0 {
		t.Fatalf("expected density score 0 for empty tokens")
	}
	if conflictScore(nil) != 1 {
		t.Fatalf("expected conflict score 1 for empty tokens")
	}
	if meanTopN([]float64{1, 0.5, 0.2}, 2) <= 0.7 {
		t.Fatalf("expected meanTopN > 0.7")
	}
	if percentile([]float64{1, 3, 2, 4}, 50) != 2.5 {
		t.Fatalf("expected percentile interpolation value 2.5")
	}

	cluster := []MemoryRecord{
		{Confidence: 0.6, Importance: 1},
		{Confidence: 0.8, Importance: 4},
	}
	if !almostEqual(avgConfidence(cluster), 0.7) {
		t.Fatalf("unexpected avg confidence")
	}
	if maxImportance(cluster) != 4 {
		t.Fatalf("unexpected max importance")
	}

	if truncateByWords("a b c d", 2) != "a b ..." {
		t.Fatalf("unexpected truncateByWords output")
	}
	if truncateByWords("a b", 0) != "a b" {
		t.Fatalf("expected no truncation for maxWords<=0")
	}
	if !isExactPhraseQuery(`find "exact"`) {
		t.Fatalf("expected exact phrase query")
	}
	if isExactPhraseQuery("find exact") {
		t.Fatalf("did not expect exact phrase query")
	}
	if distilledScoreMultiplier("query", false) != 1.0 {
		t.Fatalf("expected multiplier 1.0 for raw record")
	}
	if !almostEqual(distilledScoreMultiplier(`"quoted"`, true), defaultExactPhraseDistillMultiplier) {
		t.Fatalf("unexpected exact-phrase multiplier")
	}
	if !almostEqual(distilledScoreMultiplier("plain", true), defaultDistilledSearchMultiplier) {
		t.Fatalf("unexpected distilled multiplier")
	}

	now := time.Now()
	count := countSearchHitsWithin([]time.Time{
		now.Add(-2 * time.Hour),
		now.Add(-20 * time.Minute),
		now.Add(-10 * 24 * time.Hour),
	}, now, 24*time.Hour)
	if count != 2 {
		t.Fatalf("expected 2 recent hits, got %d", count)
	}

	left := canonicalContentSignature("  Hello   world ")
	right := canonicalContentSignature("hello world")
	if left == "" || left != right {
		t.Fatalf("expected canonical signatures to match: %q vs %q", left, right)
	}

	if maxFloat(2.0, 1.0) != 2.0 || maxFloat(1.0, 2.0) != 2.0 {
		t.Fatalf("unexpected maxFloat")
	}
	if maxInt(2, 1) != 2 || maxInt(1, 2) != 2 {
		t.Fatalf("unexpected maxInt")
	}
	if clamp(2, 0, 1) != 1 || clamp(-1, 0, 1) != 0 || clamp(0.5, 0, 1) != 0.5 {
		t.Fatalf("unexpected clamp behavior")
	}
	if maxDurationMS(now, now.Add(-time.Second)) != 0 {
		t.Fatalf("expected non-negative maxDurationMS")
	}
	if maxDurationMS(now, now.Add(1500*time.Millisecond)) != 1500 {
		t.Fatalf("unexpected maxDurationMS positive duration")
	}

	if got := takeFirstN([]string{"a", "b", "c"}, 2); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected takeFirstN output: %#v", got)
	}
	if got := uniqueSortedStrings([]string{"b", "a", "b", "c", "a"}); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected uniqueSortedStrings output: %#v", got)
	}
}

func TestDistillGitHelpers(t *testing.T) {
	repo := t.TempDir()
	initCmd := exec.Command("git", "-C", repo, "init")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Skipf("git unavailable for test: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	if root, ok := detectGitRepositoryRoot(repo); !ok || strings.TrimSpace(root) == "" {
		t.Fatalf("expected git root detection success, got root=%q ok=%v", root, ok)
	}
	if err := ensureGitWorkingTreeClean(repo); err != nil {
		t.Fatalf("expected clean tree, got err=%v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	if err := ensureGitWorkingTreeClean(repo); !errors.Is(err, ErrDistillDirtyTree) {
		t.Fatalf("expected ErrDistillDirtyTree, got %v", err)
	}

	nonRepo := t.TempDir()
	if root, ok := detectGitRepositoryRoot(nonRepo); ok || root != "" {
		t.Fatalf("expected non-repo detection failure, got root=%q ok=%v", root, ok)
	}
}
