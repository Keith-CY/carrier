package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultSessionTTL      = 30 * 24 * time.Hour
	sessionCleanupInterval = 1 * time.Hour
)

// SessionRecord represents a paired session.
type SessionRecord struct {
	Provider     string `json:"provider"`
	ChatID       string `json:"chatId"`
	SessionToken string `json:"sessionToken"`
	CreatedAt    string `json:"createdAt"`
	LastSeenAt   string `json:"lastSeenAt"`
}

// SessionStore manages pairing sessions with file persistence.
type SessionStore struct {
	mu              sync.Mutex
	sessions        map[string]*SessionRecord // key: "provider:chatId"
	sessionTTL      time.Duration
	persistencePath string
	now             func() time.Time
	dirty           bool // pending save
	quit            chan struct{}
	done            chan struct{}
}

// NewSessionStore creates a new session store. If persistencePath is non-empty,
// sessions are loaded from and saved to that file. ttl defaults to 30 days if zero.
func NewSessionStore(persistencePath string, ttl time.Duration, now func() time.Time) *SessionStore {
	if ttl == 0 {
		ttl = defaultSessionTTL
	}
	if now == nil {
		now = time.Now
	}
	s := &SessionStore{
		sessions:        make(map[string]*SessionRecord),
		sessionTTL:      ttl,
		persistencePath: persistencePath,
		now:             now,
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
	}
	if persistencePath != "" {
		s.loadSessions()
	}
	go s.background()
	return s
}

func sessionKey(provider, chatID string) string {
	return fmt.Sprintf("%s:%s", provider, chatID)
}

// CreateSession creates or refreshes a session for the given provider+chatId.
func (s *SessionStore) CreateSession(provider, chatID string) *SessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(provider, chatID)
	now := s.now().UTC().Format(time.RFC3339Nano)
	existing := s.sessions[key]
	createdAt := now
	token := "session-" + uuid.New().String()
	if existing != nil {
		createdAt = existing.CreatedAt
		token = existing.SessionToken
	}
	rec := &SessionRecord{
		Provider:     provider,
		ChatID:       chatID,
		SessionToken: token,
		CreatedAt:    createdAt,
		LastSeenAt:   now,
	}
	s.sessions[key] = rec
	s.dirty = true
	return rec
}

// GetSession retrieves a session or returns nil.
func (s *SessionStore) GetSession(provider, chatID string) *SessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.sessions[sessionKey(provider, chatID)]
	if rec == nil {
		return nil
	}
	copy := *rec
	return &copy
}

// Touch updates the LastSeenAt timestamp.
func (s *SessionStore) Touch(provider, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(provider, chatID)
	rec := s.sessions[key]
	if rec == nil {
		return
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if rec.LastSeenAt != now {
		rec.LastSeenAt = now
		s.dirty = true
	}
}

// Cleanup removes sessions older than TTL. Returns number removed.
func (s *SessionStore) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-s.sessionTTL)
	removed := 0
	for key, rec := range s.sessions {
		t, err := time.Parse(time.RFC3339Nano, rec.LastSeenAt)
		if err != nil || t.Before(cutoff) {
			delete(s.sessions, key)
			removed++
		}
	}
	if removed > 0 {
		s.dirty = true
	}
	return removed
}

// StartPeriodicCleanup starts background cleanup.
func (s *SessionStore) StartPeriodicCleanup() *SessionStore {
	// cleanup runs in the background() goroutine automatically via the ticker
	return s
}

// Stop halts background goroutines.
func (s *SessionStore) Stop() {
	close(s.quit)
	<-s.done
}

// background runs cleanup + periodic save.
func (s *SessionStore) background() {
	defer close(s.done)
	cleanupTicker := time.NewTicker(sessionCleanupInterval)
	saveTicker := time.NewTicker(5 * time.Second)
	defer cleanupTicker.Stop()
	defer saveTicker.Stop()

	for {
		select {
		case <-cleanupTicker.C:
			s.Cleanup()
		case <-saveTicker.C:
			s.mu.Lock()
			if s.dirty && s.persistencePath != "" {
				s.dirty = false
				s.mu.Unlock()
				if err := s.persistSessions(); err != nil {
					log.Printf("[gateway/session] save error: %v", err)
				}
			} else {
				s.mu.Unlock()
			}
		case <-s.quit:
			// Final save
			s.mu.Lock()
			if s.dirty && s.persistencePath != "" {
				s.dirty = false
				s.mu.Unlock()
				if err := s.persistSessions(); err != nil {
					log.Printf("[gateway/session] final save error: %v", err)
				}
			} else {
				s.mu.Unlock()
			}
			return
		}
	}
}

func (s *SessionStore) loadSessions() {
	data, err := os.ReadFile(s.persistencePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[gateway/session] load error: %v", err)
		}
		return
	}
	var records []*SessionRecord
	if err := json.Unmarshal(data, &records); err != nil {
		log.Printf("[gateway/session] parse error (starting fresh): %v", err)
		return
	}
	for _, r := range records {
		if r.Provider == "" || r.ChatID == "" || r.SessionToken == "" || r.CreatedAt == "" || r.LastSeenAt == "" {
			log.Printf("[gateway/session] skipping malformed record: %+v", r)
			continue
		}
		s.sessions[sessionKey(r.Provider, r.ChatID)] = r
	}
}

func (s *SessionStore) persistSessions() error {
	s.mu.Lock()
	records := make([]*SessionRecord, 0, len(s.sessions))
	for _, r := range s.sessions {
		cp := *r
		records = append(records, &cp)
	}
	s.mu.Unlock()

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.persistencePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := s.persistencePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.persistencePath)
}
