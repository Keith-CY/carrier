package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	telegramPairSessionTTL   = 5 * time.Minute
	telegramPairWaitTimeout  = 60 * time.Second
	telegramPairPollTimeoutS = 20
)

type telegramPairSession struct {
	ID           string
	PairCode     string
	Token        string
	NextOffset   int64
	PairedChatID string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type telegramPairStore struct {
	mu       sync.Mutex
	sessions map[string]*telegramPairSession
}

func newTelegramPairStore() *telegramPairStore {
	return &telegramPairStore{
		sessions: map[string]*telegramPairSession{},
	}
}

func (s *telegramPairStore) create(token string, nextOffset int64) (*telegramPairSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(time.Now().UTC())

	sessionID, err := randomHex(12)
	if err != nil {
		return nil, err
	}
	codeSuffix, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	session := &telegramPairSession{
		ID:         "tgpair-" + sessionID,
		PairCode:   "tg-" + codeSuffix,
		Token:      strings.TrimSpace(token),
		NextOffset: nextOffset,
		CreatedAt:  now,
		ExpiresAt:  now.Add(telegramPairSessionTTL),
	}
	s.sessions[session.ID] = session
	copy := *session
	return &copy, nil
}

func (s *telegramPairStore) get(sessionID string) (*telegramPairSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.pruneExpiredLocked(now)
	session, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return nil, false
	}
	copy := *session
	return &copy, true
}

func (s *telegramPairStore) save(session *telegramPairSession) {
	if s == nil || session == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = &telegramPairSession{
		ID:           session.ID,
		PairCode:     session.PairCode,
		Token:        session.Token,
		NextOffset:   session.NextOffset,
		PairedChatID: session.PairedChatID,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    session.ExpiresAt,
	}
}

func (s *telegramPairStore) pruneExpiredLocked(now time.Time) {
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

func randomHex(numBytes int) (string, error) {
	if numBytes <= 0 {
		return "", fmt.Errorf("numBytes must be positive")
	}
	raw := make([]byte, numBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

type telegramPairInitRequest struct {
	Token string `json:"token"`
}

func handleTelegramPairInit(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, store *telegramPairStore) {
	if store == nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "telegram pairing store is not initialized"))
		return
	}
	var req telegramPairInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "telegram token is required"))
		return
	}

	api := newTelegramBotAPI(token, cfg.TelegramAPIBaseURL, nil)
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	updates, err := api.GetUpdates(ctx, 0, 1)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_TELEGRAM_AUTH", fmt.Sprintf("telegram token validation failed: %v", err)))
		return
	}
	nextOffset := int64(0)
	for _, update := range updates {
		if id := telegramUpdateID(update); id >= nextOffset {
			nextOffset = id + 1
		}
	}

	session, err := store.create(token, nextOffset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"sessionId": session.ID,
		"pairCode":  session.PairCode,
		"command":   "/pair " + session.PairCode,
		"expiresAt": session.ExpiresAt.Format(time.RFC3339Nano),
	})
}

type telegramPairWaitRequest struct {
	SessionID string `json:"sessionId"`
}

func handleTelegramPairWait(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, store *telegramPairStore) {
	if store == nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "telegram pairing store is not initialized"))
		return
	}
	var req telegramPairWaitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "sessionId is required"))
		return
	}
	session, ok := store.get(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_PAIR_SESSION_NOT_FOUND", "telegram pair session not found or expired"))
		return
	}
	if strings.TrimSpace(session.PairedChatID) != "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"paired":    true,
			"chatId":    session.PairedChatID,
			"pairCode":  session.PairCode,
		})
		return
	}

	api := newTelegramBotAPI(session.Token, cfg.TelegramAPIBaseURL, nil)
	offset := session.NextOffset
	deadline := time.Now().UTC().Add(telegramPairWaitTimeout)

	for {
		if time.Now().UTC().After(deadline) {
			session.NextOffset = offset
			store.save(session)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"paired":    false,
				"pairCode":  session.PairCode,
				"message":   "pairing not detected yet; try again after sending the pair command",
			})
			return
		}

		remaining := time.Until(deadline)
		pollSeconds := telegramPairPollTimeoutS
		if remaining < time.Duration(pollSeconds)*time.Second {
			pollSeconds = int(remaining.Seconds())
			if pollSeconds < 1 {
				pollSeconds = 1
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(pollSeconds+8)*time.Second)
		updates, err := api.GetUpdates(ctx, offset, pollSeconds)
		cancel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_TELEGRAM_API", fmt.Sprintf("telegram getUpdates failed: %v", err)))
			return
		}

		foundChatID := ""
		for _, update := range updates {
			if id := telegramUpdateID(update); id >= offset {
				offset = id + 1
			}
			msg := ParseTelegramMessage(update)
			if msg == nil {
				continue
			}
			if telegramPairCommandMatched(msg.RawText, session.PairCode) {
				foundChatID = strings.TrimSpace(msg.ChatID)
				if foundChatID != "" {
					break
				}
			}
		}
		if foundChatID != "" {
			session.NextOffset = offset
			session.PairedChatID = foundChatID
			store.save(session)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"paired":    true,
				"chatId":    foundChatID,
				"pairCode":  session.PairCode,
			})
			return
		}
		session.NextOffset = offset
		store.save(session)
	}
}

func telegramPairCommandMatched(rawText, pairCode string) bool {
	parsed := parseCommandText(rawText)
	if parsed == nil {
		return false
	}
	switch parsed.command {
	case "/pair", "/start":
	default:
		return false
	}
	if len(parsed.args) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parsed.args[0]), strings.TrimSpace(pairCode))
}
