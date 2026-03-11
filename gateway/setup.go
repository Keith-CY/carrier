package gateway

import (
	"sync"
	"time"
)

// ProviderType is a supported provider.
type ProviderType string

const (
	ProviderTelegram ProviderType = "telegram"
	ProviderDiscord  ProviderType = "discord"
	ProviderFeishu   ProviderType = "feishu"
	ProviderDummy    ProviderType = "dummy"
)

// IsValidProviderType returns true for valid providers.
func IsValidProviderType(p string) bool {
	if ProviderType(p) == ProviderDummy {
		return true
	}
	desc, ok := LookupChannelDescriptor(p)
	if !ok {
		return false
	}
	// Preserve existing behavior: only canonical lowercase provider IDs are accepted.
	if p != string(desc.ID) {
		return false
	}
	return desc.Capabilities.SupportsProviderSetup
}

// ProviderConfig stores provider setup configuration.
type ProviderConfig struct {
	Provider      ProviderType `json:"provider"`
	Token         string       `json:"-"` // never serialized
	WebhookSecret string       `json:"-"` // never serialized
	ConfiguredAt  string       `json:"configured_at"`
}

// RedactedProviderConfig is safe to return in API responses.
type RedactedProviderConfig struct {
	Provider     ProviderType `json:"provider"`
	ConfiguredAt string       `json:"configured_at"`
}

// SetupStore holds provider setup state.
type SetupStore struct {
	mu     sync.RWMutex
	config *ProviderConfig
}

// NewSetupStore creates a new setup store.
func NewSetupStore() *SetupStore {
	return &SetupStore{}
}

// Configure sets the provider configuration.
func (s *SetupStore) Configure(provider ProviderType, token, webhookSecret string) *ProviderConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := &ProviderConfig{
		Provider:      provider,
		Token:         token,
		WebhookSecret: webhookSecret,
		ConfiguredAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.config = cfg
	return cfg
}

// GetConfig returns the current config or nil.
func (s *SetupStore) GetConfig() *ProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil {
		return nil
	}
	copy := *s.config
	return &copy
}

// GetRedacted returns the config with secrets redacted.
func (s *SetupStore) GetRedacted() *RedactedProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config == nil {
		return nil
	}
	return &RedactedProviderConfig{
		Provider:     s.config.Provider,
		ConfiguredAt: s.config.ConfiguredAt,
	}
}

// IsConfigured returns true if a provider is configured.
func (s *SetupStore) IsConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config != nil
}
