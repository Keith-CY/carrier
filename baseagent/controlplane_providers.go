package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ToolDescriptor is a compact tool description used in provider prompting.
type ToolDescriptor struct {
	Name        string
	Description string
}

// ProviderRequest is the normalized provider request envelope for agent replies.
type ProviderRequest struct {
	SystemPrompt string
	UserMessage  string
	History      []ConversationMessage
	Tools        []ToolDescriptor
}

// Provider abstracts a chat backend.
type Provider interface {
	Name() string
	Reply(ctx context.Context, req ProviderRequest) (string, error)
}

// ChainProvider tries providers in order until one succeeds.
type ChainProvider struct {
	name      string
	providers []Provider
}

func NewChainProvider(name string, providers ...Provider) *ChainProvider {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "chain"
	}
	filtered := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		filtered = append(filtered, p)
	}
	return &ChainProvider{name: name, providers: filtered}
}

func (p *ChainProvider) Name() string {
	return p.name
}

func (p *ChainProvider) Reply(ctx context.Context, req ProviderRequest) (string, error) {
	if p == nil || len(p.providers) == 0 {
		return "", fmt.Errorf("no providers in chain")
	}
	var lastErr error
	for _, provider := range p.providers {
		reply, err := provider.Reply(ctx, req)
		if err == nil && strings.TrimSpace(reply) != "" {
			return strings.TrimSpace(reply), nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("provider chain returned no content")
}

// StaticProvider always returns the same response and can be used as a fallback.
type StaticProvider struct {
	name    string
	message string
}

func NewStaticProvider(name, message string) *StaticProvider {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "static"
	}
	return &StaticProvider{
		name:    name,
		message: strings.TrimSpace(message),
	}
}

func (p *StaticProvider) Name() string {
	return p.name
}

func (p *StaticProvider) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	if strings.TrimSpace(p.message) == "" {
		return "", fmt.Errorf("static provider has no message")
	}
	return p.message, nil
}

// ProviderManager manages providers and active routing.
type ProviderManager struct {
	mu        sync.RWMutex
	providers map[string]Provider
	active    string
}

func NewProviderManager(defaultProvider Provider) *ProviderManager {
	pm := &ProviderManager{
		providers: map[string]Provider{},
	}
	if defaultProvider != nil {
		name := strings.ToLower(strings.TrimSpace(defaultProvider.Name()))
		if name == "" {
			name = "default"
		}
		pm.providers[name] = defaultProvider
		pm.active = name
	}
	return pm
}

func (pm *ProviderManager) RegisterProvider(provider Provider) error {
	if pm == nil {
		return fmt.Errorf("provider manager is nil")
	}
	if provider == nil {
		return fmt.Errorf("provider is required")
	}
	name := strings.ToLower(strings.TrimSpace(provider.Name()))
	if name == "" {
		return fmt.Errorf("provider name is required")
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.providers[name] = provider
	if pm.active == "" {
		pm.active = name
	}
	return nil
}

func (pm *ProviderManager) SetActiveProvider(name string) error {
	if pm == nil {
		return fmt.Errorf("provider manager is nil")
	}
	name = strings.ToLower(strings.TrimSpace(name))
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.providers[name]; !ok {
		return fmt.Errorf("provider not found: %s", name)
	}
	pm.active = name
	return nil
}

func (pm *ProviderManager) ActiveProviderName() string {
	if pm == nil {
		return ""
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.active
}

func (pm *ProviderManager) ListProviders() []string {
	if pm == nil {
		return nil
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]string, 0, len(pm.providers))
	for name := range pm.providers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (pm *ProviderManager) Reply(ctx context.Context, req ProviderRequest) (string, error) {
	if pm == nil {
		return "", fmt.Errorf("provider manager is nil")
	}
	pm.mu.RLock()
	active := pm.active
	provider := pm.providers[active]
	pm.mu.RUnlock()
	if provider == nil {
		return "", fmt.Errorf("no active provider is configured")
	}
	return provider.Reply(ctx, req)
}

// LLMProviderAdapter wraps baseagent requestLLMCompletion as a provider.
type LLMProviderAdapter struct {
	name          string
	historyWindow int
}

func NewLLMProviderAdapter(name string, historyWindow int) *LLMProviderAdapter {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "llm"
	}
	if historyWindow <= 0 {
		historyWindow = 8
	}
	return &LLMProviderAdapter{name: name, historyWindow: historyWindow}
}

func (p *LLMProviderAdapter) Name() string {
	return p.name
}

func (p *LLMProviderAdapter) Reply(ctx context.Context, req ProviderRequest) (string, error) {
	user := strings.TrimSpace(req.UserMessage)
	if user == "" {
		return "", fmt.Errorf("empty user message")
	}

	prompt := user
	if history := renderProviderHistory(req.History, p.historyWindow); history != "" {
		prompt = "Conversation context:\n" + history + "\n\nCurrent user message:\n" + user
	}
	if len(req.Tools) > 0 {
		toolLines := make([]string, 0, len(req.Tools))
		for _, t := range req.Tools {
			name := strings.TrimSpace(t.Name)
			if name == "" {
				continue
			}
			desc := strings.TrimSpace(t.Description)
			if desc == "" {
				toolLines = append(toolLines, "- "+name)
			} else {
				toolLines = append(toolLines, "- "+name+": "+desc)
			}
		}
		if len(toolLines) > 0 {
			prompt += "\n\nAvailable built-in tools:\n" + strings.Join(toolLines, "\n")
		}
	}

	return requestLLMCompletion(ctx, req.SystemPrompt, prompt)
}

func renderProviderHistory(history []ConversationMessage, window int) string {
	if len(history) == 0 {
		return ""
	}
	if window <= 0 {
		window = len(history)
	}
	start := len(history) - window
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(history[start:]))
	for _, msg := range history[start:] {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		if len(content) > 220 {
			content = content[:220] + "..."
		}
		lines = append(lines, role+": "+content)
	}
	return strings.Join(lines, "\n")
}
