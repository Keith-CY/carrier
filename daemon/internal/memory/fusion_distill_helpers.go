package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

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
		return fmt.Errorf("failed to check git status: %w", err)
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
