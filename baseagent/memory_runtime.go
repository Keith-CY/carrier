package baseagent

import (
	"fmt"
	"strings"
)

const defaultMemorySearchResults = 5

func (r *Runtime) extendedMemoryStore() ExtendedMemoryStore {
	if r == nil {
		return nil
	}
	store, _ := r.memory.(ExtendedMemoryStore)
	return store
}

func (r *Runtime) SearchMemory(query, scope string, maxResults int, minScore float64) ([]MemorySearchHit, error) {
	return searchMemoryStore(r.extendedMemoryStore(), baseAgentVirtualID, query, scope, maxResults, minScore)
}

func (r *Runtime) ObserveMemory(toolName, outputSnippet, scope string) (string, error) {
	return observeMemoryStore(r.extendedMemoryStore(), baseAgentVirtualID, toolName, outputSnippet, scope)
}

func searchMemoryStore(store ExtendedMemoryStore, subject, query, scope string, maxResults int, minScore float64) ([]MemorySearchHit, error) {
	if store == nil {
		return nil, fmt.Errorf("memory search is unavailable")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("memory subject is required")
	}
	scope, err := normalizeRequestedMemoryScope(scope, subject)
	if err != nil {
		return nil, err
	}
	if maxResults <= 0 {
		maxResults = defaultMemorySearchResults
	}
	hits, err := store.Search(subject, query, maxResults, minScore)
	if err != nil {
		return nil, err
	}
	if scope != "" {
		hits = filterMemoryHitsByScope(hits, scope)
	}
	return cloneMemorySearchHits(hits), nil
}

func observeMemoryStore(store ExtendedMemoryStore, subject, toolName, outputSnippet, scope string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("memory observation is unavailable")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", fmt.Errorf("memory subject is required")
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", fmt.Errorf("tool name is required")
	}
	outputSnippet = strings.TrimSpace(outputSnippet)
	if outputSnippet == "" {
		return "", fmt.Errorf("output snippet is required")
	}
	scope, err := normalizeRequestedMemoryScope(scope, subject)
	if err != nil {
		return "", err
	}
	return store.Observe(subject, toolName, outputSnippet, scope)
}

func normalizeRequestedMemoryScope(scope, subject string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "", nil
	}
	if scope == "public" {
		return scope, nil
	}
	if scope == "agent:"+strings.TrimSpace(subject) {
		return scope, nil
	}
	if strings.HasPrefix(scope, "shared:") {
		return scope, nil
	}
	if strings.HasPrefix(scope, "agent:") {
		return "", fmt.Errorf("unauthorized memory scope %q", scope)
	}
	return "", fmt.Errorf("unsupported memory scope %q", scope)
}

func filterMemoryHitsByScope(hits []MemorySearchHit, scope string) []MemorySearchHit {
	scope = strings.TrimSpace(scope)
	if scope == "" || len(hits) == 0 {
		return cloneMemorySearchHits(hits)
	}
	out := make([]MemorySearchHit, 0, len(hits))
	for _, hit := range hits {
		if strings.TrimSpace(hit.Scope) == scope {
			out = append(out, hit)
		}
	}
	return out
}

func cloneMemorySearchHits(hits []MemorySearchHit) []MemorySearchHit {
	if len(hits) == 0 {
		return nil
	}
	out := make([]MemorySearchHit, len(hits))
	copy(out, hits)
	return out
}
