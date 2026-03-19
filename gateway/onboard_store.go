package gateway

import "sync"

// OnboardStep is a step in the onboard state machine.
type OnboardStep string

const (
	OnboardIdle             OnboardStep = "idle"
	OnboardChannelSelect    OnboardStep = "channel_select"
	OnboardChannelToken     OnboardStep = "channel_token"
	OnboardAgentSelected    OnboardStep = "agent_selected"
	OnboardProviderSelected OnboardStep = "provider_selected"
	OnboardAuthConfigured   OnboardStep = "auth_configured"
	OnboardEnvConfigured    OnboardStep = "env_configured"
	OnboardInstalling       OnboardStep = "installing"
	OnboardDone             OnboardStep = "done"
)

// OnboardSession is the state for a single chat's onboard flow.
type OnboardSession struct {
	Step                OnboardStep
	InstanceID          string
	SelectedAgent       string
	SelectedAgentName   string
	SelectedChannel     string
	ChannelToken        string
	ChannelSetupPending bool
	SelectedProvider    string
	WorkspacePath       string
	EnvVars             map[string]string
}

// OnboardStore tracks per-session onboard state.
type OnboardStore struct {
	mu       sync.Mutex
	sessions map[string]*OnboardSession
}

// NewOnboardStore creates a new onboard store.
func NewOnboardStore() *OnboardStore {
	return &OnboardStore{sessions: make(map[string]*OnboardSession)}
}

func (s *OnboardStore) get(key string) *OnboardSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[key]
}

func (s *OnboardStore) start(key string) *OnboardSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := &OnboardSession{Step: OnboardIdle, EnvVars: make(map[string]string)}
	s.sessions[key] = sess
	return sess
}

func (s *OnboardStore) update(key string, fn func(*OnboardSession)) *OnboardSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[key]
	if sess == nil {
		sess = &OnboardSession{Step: OnboardIdle, EnvVars: make(map[string]string)}
		s.sessions[key] = sess
	}
	fn(sess)
	return sess
}

func (s *OnboardStore) clear(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

func (s *OnboardStore) hasActive(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[key]
	return sess != nil && sess.Step != OnboardIdle && sess.Step != OnboardDone
}
