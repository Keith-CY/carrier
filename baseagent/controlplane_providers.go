package baseagent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxProviderHistoryMessageLength = 220

type ProviderErrorKind string

const (
	ProviderErrorRetriable    ProviderErrorKind = "retriable"
	ProviderErrorNonRetriable ProviderErrorKind = "non_retriable"
)

type classifiedProviderError struct {
	kind ProviderErrorKind
	err  error
}

func (e *classifiedProviderError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *classifiedProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *classifiedProviderError) Kind() ProviderErrorKind {
	if e == nil {
		return ProviderErrorRetriable
	}
	return e.kind
}

func RetriableProviderError(err error) error {
	if err == nil {
		return nil
	}
	return &classifiedProviderError{kind: ProviderErrorRetriable, err: err}
}

func NonRetriableProviderError(err error) error {
	if err == nil {
		return nil
	}
	return &classifiedProviderError{kind: ProviderErrorNonRetriable, err: err}
}

// ToolDescriptor is a compact tool description used in provider prompting.
type ToolDescriptor struct {
	Name        string
	Description string
}

// ProviderRequest is the normalized provider request envelope for agent replies.
type ProviderRequest struct {
	SystemPrompt    string
	UserMessage     string
	History         []ConversationMessage
	Tools           []ToolDescriptor
	StructuredTools []StructuredToolDescriptor
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
	cooldown  time.Duration
	now       func() time.Time
	blocked   map[string]time.Time
}

func NewProviderManager(defaultProvider Provider) *ProviderManager {
	pm := &ProviderManager{
		providers: map[string]Provider{},
		cooldown:  time.Minute,
		now:       time.Now,
		blocked:   map[string]time.Time{},
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
	if err := providerContextError(ctx); err != nil {
		return "", err
	}
	candidates := pm.availableProviders()
	if len(candidates) == 0 {
		return "", fmt.Errorf("no active provider is configured")
	}
	var lastErr error
	for _, candidate := range candidates {
		if err := providerContextError(ctx); err != nil {
			return "", err
		}
		reply, err := candidate.provider.Reply(ctx, req)
		if err == nil && strings.TrimSpace(reply) != "" {
			pm.markProviderSuccess(candidate.name)
			return strings.TrimSpace(reply), nil
		}
		if err != nil {
			if ctxErr := context.Cause(ctx); ctxErr != nil {
				return "", ctxErr
			}
			pm.markProviderFailure(candidate.name, err)
			lastErr = err
			continue
		}
		pm.markProviderFailure(candidate.name, nil)
		lastErr = fmt.Errorf("provider %s returned no content", candidate.name)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("provider chain returned no content")
}

func (pm *ProviderManager) ReplyWithTools(ctx context.Context, req StructuredToolRequest) (StructuredToolReply, error) {
	if pm == nil {
		return StructuredToolReply{}, fmt.Errorf("provider manager is nil")
	}
	if err := providerContextError(ctx); err != nil {
		return StructuredToolReply{}, err
	}
	candidates := pm.availableProviders()
	if len(candidates) == 0 {
		return StructuredToolReply{}, fmt.Errorf("no active provider is configured")
	}
	var lastErr error
	for _, candidate := range candidates {
		if err := providerContextError(ctx); err != nil {
			return StructuredToolReply{}, err
		}
		reply, err := pm.replyWithToolsFromProvider(ctx, candidate.provider, req)
		if err == nil && structuredReplyHasContent(reply) {
			pm.markProviderSuccess(candidate.name)
			return reply, nil
		}
		if err != nil {
			if ctxErr := context.Cause(ctx); ctxErr != nil {
				return StructuredToolReply{}, ctxErr
			}
			pm.markProviderFailure(candidate.name, err)
			lastErr = err
			continue
		}
		pm.markProviderFailure(candidate.name, nil)
		lastErr = fmt.Errorf("provider %s returned no tool reply", candidate.name)
	}
	if lastErr != nil {
		return StructuredToolReply{}, lastErr
	}
	return StructuredToolReply{}, fmt.Errorf("provider chain returned no tool reply")
}

type providerCandidate struct {
	name     string
	provider Provider
}

func (pm *ProviderManager) availableProviders() []providerCandidate {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	now := time.Now()
	if pm.now != nil {
		now = pm.now()
	}

	orderedNames := make([]string, 0, len(pm.providers))
	for name := range pm.providers {
		orderedNames = append(orderedNames, name)
	}
	sort.Strings(orderedNames)

	candidates := make([]providerCandidate, 0, len(orderedNames))
	appendCandidate := func(name string) {
		provider := pm.providers[name]
		if provider == nil {
			return
		}
		if until, ok := pm.blocked[name]; ok && until.After(now) {
			return
		}
		candidates = append(candidates, providerCandidate{name: name, provider: provider})
	}

	active := strings.TrimSpace(pm.active)
	if active != "" {
		appendCandidate(active)
	}
	for _, name := range orderedNames {
		if name == active {
			continue
		}
		appendCandidate(name)
	}
	return candidates
}

func (pm *ProviderManager) markProviderFailure(name string, err error) {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if !providerErrorShouldCooldown(err) {
		delete(pm.blocked, name)
		return
	}
	if pm.cooldown <= 0 {
		return
	}
	now := time.Now()
	if pm.now != nil {
		now = pm.now()
	}
	pm.blocked[name] = now.Add(pm.cooldown)
}

func (pm *ProviderManager) markProviderSuccess(name string) {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.blocked, name)
	if name != "" {
		pm.active = name
	}
}

func (pm *ProviderManager) replyWithToolsFromProvider(ctx context.Context, provider Provider, req StructuredToolRequest) (StructuredToolReply, error) {
	if aware, ok := provider.(ToolAwareProvider); ok {
		return aware.ReplyWithTools(ctx, req)
	}
	return replyWithToolsViaTextProvider(ctx, provider, req)
}

func structuredReplyHasContent(reply StructuredToolReply) bool {
	if strings.TrimSpace(reply.Content) != "" {
		return true
	}
	return len(reply.ToolCalls) > 0
}

func providerContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return ctx.Err()
}

func providerErrorShouldCooldown(err error) bool {
	if err == nil {
		return true
	}
	var classified *classifiedProviderError
	if errors.As(err, &classified) {
		return classified.Kind() != ProviderErrorNonRetriable
	}
	return true
}
