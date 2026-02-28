// Package pairing provides time-limited pairing code generation for daemon-gateway handshake.
package pairing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// DefaultTTL is the default time-to-live for a pairing code.
	DefaultTTL = 5 * time.Minute
	// CodeLength is the number of random bytes (hex-encoded = 2x characters).
	CodeLength = 4 // 4 bytes = 8 hex chars
)

// Store holds the current pairing code with automatic expiry and regeneration.
type Store struct {
	mu      sync.RWMutex
	code    string
	expires time.Time
	ttl     time.Duration
	now     func() time.Time
}

// Option configures a Store.
type Option func(*Store)

// WithTTL sets a custom TTL for pairing codes.
func WithTTL(d time.Duration) Option {
	return func(s *Store) { s.ttl = d }
}

// WithNow overrides the time source (for testing).
func WithNow(fn func() time.Time) Option {
	return func(s *Store) { s.now = fn }
}

// NewStore creates a new pairing code store and generates the initial code.
func NewStore(opts ...Option) (*Store, error) {
	s := &Store{
		ttl: DefaultTTL,
		now: time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	if err := s.regenerate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Code returns the current pairing code, regenerating if expired.
func (s *Store) Code() (string, error) {
	s.mu.RLock()
	if s.now().Before(s.expires) {
		code := s.code
		s.mu.RUnlock()
		return code, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock.
	if s.now().Before(s.expires) {
		return s.code, nil
	}
	if err := s.regenerate(); err != nil {
		return "", err
	}
	return s.code, nil
}

func (s *Store) regenerate() error {
	b := make([]byte, CodeLength)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return fmt.Errorf("pairing: generate code: %w", err)
	}
	s.code = hex.EncodeToString(b)
	s.expires = s.now().Add(s.ttl)
	return nil
}
