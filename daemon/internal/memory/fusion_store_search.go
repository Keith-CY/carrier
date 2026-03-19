package memory

import (
	"sort"
	"strings"
	"unicode"
)

type searchCandidate struct {
	hit      SearchHit
	lexical  float64
	semantic float64
}

// Search performs a compact record recall with permission-first filtering.
func (s *Store) Search(opts SearchOptions) []SearchHit {
	s.mu.RLock()

	subject := strings.TrimSpace(opts.Subject)
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		s.mu.RUnlock()
		return nil
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = maxDefaultSearchResults
	}
	if maxResults > maxSearchResults {
		maxResults = maxSearchResults
	}
	allowed := s.allowedScopesForSubjectLocked(subject)
	includeDistilled := optionBool(opts.IncludeDistilled, true)
	includeRaw := optionBool(opts.IncludeRaw, true)

	candidateMultiplier := opts.CandidateMultiplier
	if candidateMultiplier <= 0 {
		candidateMultiplier = defaultSearchMultiplier
	}
	if candidateMultiplier > maxSearchMultiplier {
		candidateMultiplier = maxSearchMultiplier
	}
	candidateLimit := maxResults * candidateMultiplier
	if candidateLimit < minCandidateLimit {
		candidateLimit = minCandidateLimit
	}
	if candidateLimit > maxCandidateLimit {
		candidateLimit = maxCandidateLimit
	}

	lexicalWeight := optionFloat(opts.LexicalWeight, defaultLexicalWeight)
	semanticWeight := optionFloat(opts.SemanticWeight, defaultSemanticWeight)
	enableRerank := optionBool(opts.Rerank, true)
	enableAdaptiveRecall := optionBool(opts.AdaptiveRecall, true)
	if lexicalWeight < 0 {
		lexicalWeight = 0
	}
	if semanticWeight < 0 {
		semanticWeight = 0
	}
	weightSum := lexicalWeight + semanticWeight
	if weightSum == 0 {
		lexicalWeight = defaultLexicalWeight
		semanticWeight = defaultSemanticWeight
		weightSum = lexicalWeight + semanticWeight
	}
	lexicalWeight /= weightSum
	semanticWeight /= weightSum

	candidates := make(map[string]searchCandidate, maxResults)
	usedSQLite := false
	sqliteMinScore := opts.MinScore
	if enableRerank {
		sqliteMinScore = 0
	}
	lowerQuery := strings.ToLower(query)
	if sqliteHits, ok := s.searchSQLiteLocked(allowed, query, candidateLimit, sqliteMinScore); ok {
		for _, hit := range sqliteHits {
			candidates[hit.ID] = searchCandidate{
				hit:     hit,
				lexical: hit.Score,
			}
		}
		usedSQLite = true
	}
	needInMemory := !usedSQLite || (enableAdaptiveRecall && len(candidates) < maxResults)
	if needInMemory {
		inMemoryHits := s.searchInMemoryLocked(allowed, query, candidateLimit, 0)
		for _, hit := range inMemoryHits {
			existing, ok := candidates[hit.ID]
			if !ok || hit.Score > existing.lexical {
				candidates[hit.ID] = searchCandidate{
					hit:     hit,
					lexical: hit.Score,
				}
			}
		}
	}

	results := make([]SearchHit, 0, len(candidates))
	for _, candidate := range candidates {
		contentText := candidate.hit.Snippet
		isDistilled := false
		if rec, ok := s.records[candidate.hit.ID]; ok {
			isDistilled = isDistilledRecord(rec)
			if isDistilled && !includeDistilled {
				continue
			}
			if !isDistilled && !includeRaw {
				continue
			}
			contentText = strings.TrimSpace(rec.ContentSummary)
			if contentText == "" {
				contentText = strings.TrimSpace(rec.ContentRaw)
			}
		} else if !includeRaw {
			continue
		}
		semanticScore := scoreText(contentText, lowerQuery)
		candidate.semantic = semanticScore
		if enableRerank {
			candidate.hit.Score = blendScore(candidate.lexical, candidate.semantic, lexicalWeight, semanticWeight)
		} else {
			candidate.hit.Score = candidate.lexical
		}
		candidate.hit.Score *= distilledScoreMultiplier(query, isDistilled)
		if opts.MinScore > 0 && candidate.hit.Score < opts.MinScore {
			continue
		}
		results = append(results, candidate.hit)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ID < results[j].ID
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	resultIDs := make([]string, 0, len(results))
	for _, hit := range results {
		resultIDs = append(resultIDs, hit.ID)
	}
	s.mu.RUnlock()
	s.recordSearchHits(resultIDs)
	return results
}

func (s *Store) searchInMemoryLocked(allowed map[Scope]struct{}, query string, maxResults int, minScore float64) []SearchHit {
	hits := make([]SearchHit, 0, maxResults)
	lowerQuery := strings.ToLower(query)
	for _, rec := range s.records {
		if rec.ArchivedAt != nil {
			continue
		}
		if !scopeAllowed(allowed, rec.Scope) {
			continue
		}
		text := strings.TrimSpace(rec.ContentSummary)
		if text == "" {
			text = strings.TrimSpace(rec.ContentRaw)
		}
		if text == "" {
			continue
		}
		score := scoreText(text, lowerQuery)
		if score <= 0 {
			continue
		}
		if minScore > 0 && score < minScore {
			continue
		}
		hits = append(hits, SearchHit{
			ID:         rec.ID,
			Scope:      rec.Scope,
			Score:      score,
			Snippet:    clipSnippet(text, defaultSnippetLimit),
			Provenance: rec.Provenance,
		})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxResults {
		hits = hits[:maxResults]
	}
	return hits
}

func scoreText(text, lowerQuery string) float64 {
	lower := strings.ToLower(text)
	lower = strings.TrimSpace(lower)
	if lowerQuery == "" || lower == "" {
		return 0
	}
	parts := tokenizeText(lowerQuery)
	if len(parts) == 0 {
		return 0
	}
	matched := 0.0
	textParts := tokenizeText(lower)
	if len(textParts) == 0 {
		return 0
	}
	textSet := make(map[string]struct{}, len(textParts))
	for _, part := range textParts {
		if part == "" {
			continue
		}
		textSet[part] = struct{}{}
	}
	for _, part := range parts {
		if _, ok := textSet[part]; ok {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return matched / float64(len(parts))
}

func blendScore(lexicalScore, semanticScore, lexicalWeight, semanticWeight float64) float64 {
	if lexicalWeight+semanticWeight == 0 {
		return lexicalScore
	}
	sum := lexicalWeight + semanticWeight
	return (lexicalScore*lexicalWeight + semanticScore*semanticWeight) / sum
}

func optionBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func optionFloat(v *float64, fallback float64) float64 {
	if v == nil {
		return fallback
	}
	return *v
}

func tokenizeText(input string) []string {
	input = strings.ToLower(input)
	return strings.FieldsFunc(input, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

func clipSnippet(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
