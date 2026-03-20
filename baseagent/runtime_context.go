package baseagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultRuntimeContextClass         = "general"
	defaultRuntimeContextRedactionMode = "hidden"
)

type SharedInstruction struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

type RuntimeContextEntry struct {
	Key           string `json:"key"`
	Value         any    `json:"value,omitempty"`
	Source        string `json:"source,omitempty"`
	Class         string `json:"class,omitempty"`
	RedactionMode string `json:"redactionMode,omitempty"`
}

type RuntimeContextManifestEntry struct {
	Key           string `json:"key"`
	Source        string `json:"source,omitempty"`
	Class         string `json:"class,omitempty"`
	RedactionMode string `json:"redactionMode,omitempty"`
	Digest        string `json:"digest,omitempty"`
	ValueType     string `json:"valueType,omitempty"`
}

type RuntimeContextManifest struct {
	Entries []RuntimeContextManifestEntry `json:"entries,omitempty"`
}

type runtimeContextState struct {
	entries  []RuntimeContextEntry
	manifest RuntimeContextManifest
	byKey    map[string]RuntimeContextEntry
}

type runtimeContextKey struct{}

func NormalizeSharedInstructions(in []SharedInstruction) []SharedInstruction {
	if len(in) == 0 {
		return nil
	}
	out := make([]SharedInstruction, 0, len(in))
	for _, item := range in {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		out = append(out, SharedInstruction{
			ID:      strings.TrimSpace(item.ID),
			Title:   strings.TrimSpace(item.Title),
			Content: content,
			Source:  strings.TrimSpace(item.Source),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeRuntimeContextEntries(in []RuntimeContextEntry) []RuntimeContextEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]RuntimeContextEntry, 0, len(in))
	seen := map[string]int{}
	for _, item := range in {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		normalized := RuntimeContextEntry{
			Key:           key,
			Value:         item.Value,
			Source:        strings.TrimSpace(item.Source),
			Class:         firstNonEmptyRuntimeContextValue(strings.TrimSpace(item.Class), defaultRuntimeContextClass),
			RedactionMode: firstNonEmptyRuntimeContextValue(strings.TrimSpace(item.RedactionMode), defaultRuntimeContextRedactionMode),
		}
		if idx, ok := seen[strings.ToLower(key)]; ok {
			out[idx] = normalized
			continue
		}
		seen[strings.ToLower(key)] = len(out)
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeRuntimeContextManifest(in RuntimeContextManifest) RuntimeContextManifest {
	if len(in.Entries) == 0 {
		return RuntimeContextManifest{}
	}
	normalized := make([]RuntimeContextManifestEntry, 0, len(in.Entries))
	seen := map[string]int{}
	for _, item := range in.Entries {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		entry := RuntimeContextManifestEntry{
			Key:           key,
			Source:        strings.TrimSpace(item.Source),
			Class:         firstNonEmptyRuntimeContextValue(strings.TrimSpace(item.Class), defaultRuntimeContextClass),
			RedactionMode: firstNonEmptyRuntimeContextValue(strings.TrimSpace(item.RedactionMode), defaultRuntimeContextRedactionMode),
			Digest:        strings.TrimSpace(item.Digest),
			ValueType:     strings.TrimSpace(item.ValueType),
		}
		lowerKey := strings.ToLower(key)
		if idx, ok := seen[lowerKey]; ok {
			normalized[idx] = entry
			continue
		}
		seen[lowerKey] = len(normalized)
		normalized = append(normalized, entry)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i].Key) < strings.ToLower(normalized[j].Key)
	})
	return RuntimeContextManifest{Entries: normalized}
}

func BuildRuntimeContextManifest(entries []RuntimeContextEntry) RuntimeContextManifest {
	normalized := NormalizeRuntimeContextEntries(entries)
	if len(normalized) == 0 {
		return RuntimeContextManifest{}
	}
	manifest := RuntimeContextManifest{Entries: make([]RuntimeContextManifestEntry, 0, len(normalized))}
	for _, item := range normalized {
		manifest.Entries = append(manifest.Entries, RuntimeContextManifestEntry{
			Key:           item.Key,
			Source:        item.Source,
			Class:         item.Class,
			RedactionMode: item.RedactionMode,
			Digest:        runtimeContextDigest(item.Value),
			ValueType:     runtimeContextValueType(item.Value),
		})
	}
	return NormalizeRuntimeContextManifest(manifest)
}

func WithRuntimeContext(ctx context.Context, entries []RuntimeContextEntry) context.Context {
	normalized := NormalizeRuntimeContextEntries(entries)
	if len(normalized) == 0 {
		return ctx
	}
	state := runtimeContextState{
		entries:  append([]RuntimeContextEntry(nil), normalized...),
		manifest: BuildRuntimeContextManifest(normalized),
		byKey:    make(map[string]RuntimeContextEntry, len(normalized)),
	}
	for _, item := range normalized {
		state.byKey[strings.ToLower(item.Key)] = item
	}
	return context.WithValue(ctx, runtimeContextKey{}, state)
}

func RuntimeContextEntriesFromContext(ctx context.Context) []RuntimeContextEntry {
	state, ok := ctx.Value(runtimeContextKey{}).(runtimeContextState)
	if !ok || len(state.entries) == 0 {
		return nil
	}
	return append([]RuntimeContextEntry(nil), state.entries...)
}

func RuntimeContextManifestFromContext(ctx context.Context) RuntimeContextManifest {
	state, ok := ctx.Value(runtimeContextKey{}).(runtimeContextState)
	if !ok {
		return RuntimeContextManifest{}
	}
	return NormalizeRuntimeContextManifest(state.manifest)
}

func RuntimeContextValue(ctx context.Context, key string) (any, bool) {
	state, ok := ctx.Value(runtimeContextKey{}).(runtimeContextState)
	if !ok || len(state.byKey) == 0 {
		return nil, false
	}
	entry, exists := state.byKey[strings.ToLower(strings.TrimSpace(key))]
	if !exists {
		return nil, false
	}
	return entry.Value, true
}

func composeSharedInstructionSystemPrompt(basePrompt string, instructions []SharedInstruction) string {
	basePrompt = strings.TrimSpace(basePrompt)
	instructions = NormalizeSharedInstructions(instructions)
	if len(instructions) == 0 {
		return basePrompt
	}
	lines := make([]string, 0, len(instructions)+1)
	lines = append(lines, "Shared instructions:")
	for _, instruction := range instructions {
		label := firstNonEmptyRuntimeContextValue(instruction.Title, instruction.ID, instruction.Source)
		if label != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", label, instruction.Content))
			continue
		}
		lines = append(lines, "- "+instruction.Content)
	}
	block := strings.Join(lines, "\n")
	if basePrompt == "" {
		return block
	}
	return basePrompt + "\n\n" + block
}

func runtimeContextDigest(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(fmt.Sprintf("%v", value))
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:12])
}

func runtimeContextValueType(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case int, int8, int16, int32, int64:
		return "integer"
	case uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case []string:
		return "array[string]"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", typed)
	}
}

func firstNonEmptyRuntimeContextValue(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
