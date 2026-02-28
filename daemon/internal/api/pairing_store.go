package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const defaultPairingTTL = 5 * time.Minute

var (
	// ErrPairCodeInvalid indicates a pairing code is missing, expired, or unknown.
	ErrPairCodeInvalid = errors.New("pairing code is invalid or expired")
	// ErrPairCodeRequired indicates code generation/registration input is missing.
	ErrPairCodeRequired = errors.New("pairing code is required")
)

type PairingCodeRecord struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

// PairingCodeStore keeps pairing codes with TTL semantics.
type PairingCodeStore struct {
	mu           sync.Mutex
	now          func() time.Time
	generateCode func() (string, error)
	codes        map[string]time.Time
}

func NewPairingCodeStore(now func() time.Time) *PairingCodeStore {
	return newPairingCodeStore(now, nil)
}

func newPairingCodeStore(now func() time.Time, generateCode func() (string, error)) *PairingCodeStore {
	if now == nil {
		now = time.Now
	}
	if generateCode == nil {
		generateCode = newPairingCodeValue
	}
	return &PairingCodeStore{
		now:          now,
		generateCode: generateCode,
		codes:        make(map[string]time.Time),
	}
}

func (s *PairingCodeStore) Issue(ttl time.Duration) (PairingCodeRecord, error) {
	// Opportunistic cleanup to prevent unbounded memory growth.
	s.CleanupExpired()

	code, err := s.generateCode()
	if err != nil {
		return PairingCodeRecord{}, err
	}
	return s.Register(code, ttl)
}

func (s *PairingCodeStore) Register(code string, ttl time.Duration) (PairingCodeRecord, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return PairingCodeRecord{}, ErrPairCodeRequired
	}
	if ttl <= 0 {
		ttl = defaultPairingTTL
	}
	expiresAt := s.now().Add(ttl).UTC()

	s.mu.Lock()
	s.codes[code] = expiresAt
	s.mu.Unlock()

	return PairingCodeRecord{
		Code:      code,
		ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *PairingCodeStore) VerifyAndConsume(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrPairCodeInvalid
	}

	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt, ok := s.codes[code]
	if !ok {
		return ErrPairCodeInvalid
	}
	if !expiresAt.After(now) {
		delete(s.codes, code)
		return ErrPairCodeInvalid
	}

	delete(s.codes, code)
	return nil
}

func (s *PairingCodeStore) CleanupExpired() int {
	now := s.now()
	removed := 0

	s.mu.Lock()
	for code, expiresAt := range s.codes {
		if !expiresAt.After(now) {
			delete(s.codes, code)
			removed++
		}
	}
	s.mu.Unlock()

	return removed
}

func (s *PairingCodeStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.codes)
}

// List returns all current valid pairing codes.
func (s *PairingCodeStore) List() []PairingCodeRecord {
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	var records []PairingCodeRecord
	for code, expiresAt := range s.codes {
		if expiresAt.After(now) {
			records = append(records, PairingCodeRecord{
				Code:      code,
				ExpiresAt: expiresAt.Format(time.RFC3339Nano),
			})
		}
	}
	return records
}

func newPairingCodeValue() (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", fmt.Errorf("generate pairing code: %w", err)
	}
	return "pair-" + hex.EncodeToString(raw[:]), nil
}
