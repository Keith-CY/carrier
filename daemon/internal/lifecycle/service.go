package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/memory"
	"carrier/daemon/internal/runtimecheck"
)

var (
	ErrAgentNotFound              = errors.New("agent not found")
	ErrNotInstalled               = errors.New("agent is not installed")
	ErrAlreadyRunning             = errors.New("agent already running")
	ErrAlreadyStopped             = errors.New("agent already stopped")
	ErrCrashLoop                  = errors.New("agent is in crash loop cooldown")
	ErrAgentRunning               = errors.New("agent is running; stop it before upgrading")
	ErrUpgradeNotSupported        = errors.New("agent manifest does not define an upgrade command")
	ErrMissingRequiredEnv         = errors.New("missing required environment variables")
	ErrPortConflict               = errors.New("port conflict detected")
	ErrRuntimePrerequisites       = errors.New("runtime prerequisites failed")
	ErrIsolationUnavailable       = errors.New("isolation backend is unavailable")
	ErrIsolationStartFailed       = errors.New("isolation runtime start failed")
	ErrRemoteDiagnosisNotNeeded   = errors.New("remote diagnosis is not required for this agent")
	ErrUpgradeFailed              = errors.New("agent upgrade failed")
	ErrUpgradeStrategyUnsupported = errors.New("upgrade strategy is not supported")
	ErrAgentAlreadyRunning        = errors.New("cannot re-register manifest while agent is running")
)

const (
	defaultCrashLoopThreshold = 3
	defaultCrashLoopWindow    = 5 * time.Minute
	defaultCrashLoopCooldown  = 5 * time.Minute
	defaultCommandTimeout     = 20 * time.Minute
)

var (
	listenTCP       = net.Listen
	portOccupantFor = describePortOccupant
)

type Service struct {
	mu                 sync.RWMutex
	states             map[string]AgentState
	manifests          map[string]manifest.Manifest
	memoryLinks        map[string][]string
	logs               map[string][]string
	handoffs           map[string]DiagnosisHandoff
	auditLogs          []AuditLog
	auditLogLimit      int
	triager            baseagent.Triager
	checker            runtimecheck.Checker
	runner             commandexec.Runner
	diagnoseDir        string
	auditLogDir        string
	logLimit           int
	handoffTTL         time.Duration
	now                func() time.Time
	idGenerator        func(prefix string) string
	restarts           map[string][]time.Time
	cooldowns          map[string]time.Time
	crashLoopThreshold int
	crashLoopWindow    time.Duration
	crashLoopCooldown  time.Duration
	memoryStore        *memory.Store
	stateFile          *StateFile
	pendingPersisted   map[string]*PersistedAgentState // loaded on startup, applied during RegisterManifest
	processManager     ProcessController
	processLogDir      string
	backoffPolicy      BackoffPolicy
	backoffStates      map[string]BackoffState
	exitCodes          map[string]*int
	evidenceCollector  *EvidenceCollector
	commandTimeout     time.Duration
	alertManager       *AlertManager
	webhookManager     *WebhookManager
}

func NewService(triager baseagent.Triager, opts ...Option) *Service {
	if triager == nil {
		triager = baseagent.NoopTriager{}
	}

	processLogDir := filepath.Join(os.TempDir(), "carrier-daemon-process-logs")
	exitCodes := make(map[string]*int)
	logs := make(map[string][]string)

	svc := &Service{
		states:             make(map[string]AgentState),
		manifests:          make(map[string]manifest.Manifest),
		memoryLinks:        make(map[string][]string),
		logs:               logs,
		handoffs:           make(map[string]DiagnosisHandoff),
		auditLogs:          make([]AuditLog, 0, 128),
		auditLogLimit:      1000,
		triager:            triager,
		checker:            runtimecheck.NewHostChecker(),
		runner:             commandexec.NewShellRunner(),
		diagnoseDir:        filepath.Join(os.TempDir(), "carrier-daemon-diagnose"),
		auditLogDir:        filepath.Join(os.TempDir(), "carrier-daemon-audit"),
		logLimit:           1000,
		handoffTTL:         24 * time.Hour,
		now:                time.Now,
		restarts:           make(map[string][]time.Time),
		cooldowns:          make(map[string]time.Time),
		crashLoopThreshold: defaultCrashLoopThreshold,
		crashLoopWindow:    defaultCrashLoopWindow,
		crashLoopCooldown:  defaultCrashLoopCooldown,
		processLogDir:      processLogDir,
		backoffPolicy:      DefaultBackoffPolicy(),
		backoffStates:      make(map[string]BackoffState),
		exitCodes:          exitCodes,
		commandTimeout:     loadCommandTimeoutFromEnv(os.Getenv("CARRIER_COMMAND_TIMEOUT")),
		alertManager:       NewAlertManager(false, nil),
		webhookManager:     NewWebhookManager("", nil),
	}
	svc.processManager = NewProcessManager(processLogDir)
	svc.evidenceCollector = NewEvidenceCollector(logs, exitCodes, 1000)
	svc.idGenerator = func(prefix string) string {
		var buf [16]byte
		if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
			panic("crypto/rand failed: " + err.Error())
		}
		return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(buf[:]))
	}
	for _, opt := range opts {
		opt(svc)
	}
	_ = cleanupExpiredRollbackSnapshots()

	// Load persisted state if configured
	if svc.stateFile != nil {
		if err := svc.loadPersistedState(); err != nil {
			// Log but don't fail - start with empty state if load fails
			// In production, you might want to handle this differently
			_ = err
		}
	}

	return svc
}

func (s *Service) RegisterManifest(m manifest.Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Guard: reject re-registration when the agent is currently running.
	if existing, ok := s.states[m.ID]; ok && existing.Runtime == RuntimeStateRunning {
		return ErrAgentAlreadyRunning
	}

	// Preserve runtime state for non-running agents that already exist.
	if existing, ok := s.states[m.ID]; ok {
		existing.Name = m.Name
		existing.Version = m.Version
		existing.UpdatedAt = s.now()
		s.states[m.ID] = existing
		s.manifests[m.ID] = m
		return nil
	}

	s.manifests[m.ID] = m
	s.states[m.ID] = AgentState{
		ID:        m.ID,
		Name:      m.Name,
		Version:   m.Version,
		Install:   InstallStateNotInstalled,
		Runtime:   RuntimeStateStopped,
		Health:    HealthStateUnknown,
		Ports:     []int{},
		UpdatedAt: s.now(),
	}
	if _, ok := s.memoryLinks[m.ID]; !ok {
		s.memoryLinks[m.ID] = nil
	}
	s.logs[m.ID] = nil
	s.backoffStates[m.ID] = BackoffState{}

	// Apply any pending persisted state (crash-loop cooldown, restart timestamps)
	// from a prior daemon run. If none exists, initialise to zero values.
	if pState, ok := s.pendingPersisted[m.ID]; ok {
		state := s.states[m.ID]
		s.applyPersistedState(m.ID, pState, &state)
		s.states[m.ID] = state
		delete(s.pendingPersisted, m.ID)
	} else {
		if _, ok := s.restarts[m.ID]; !ok {
			s.restarts[m.ID] = nil
		}
		if _, ok := s.cooldowns[m.ID]; !ok {
			s.cooldowns[m.ID] = time.Time{}
		}
	}

	return nil
}

func (s *Service) ListAgents() []AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.states))
	for id := range s.states {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]AgentState, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.states[id])
	}

	return out
}

// RunningAgentsCount returns the number of agents currently in running state.
func (s *Service) RunningAgentsCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, state := range s.states {
		if state.Runtime == RuntimeStateRunning {
			count++
		}
	}
	return count
}

// AgentName returns the manifest name for an agent ID, or the ID when no name is available.
func (s *Service) AgentName(agentID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.manifests[agentID]
	if !ok || m.Name == "" {
		return agentID
	}
	return m.Name
}

func (s *Service) Status(agentID string) (AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[agentID]
	if !ok {
		return AgentState{}, ErrAgentNotFound
	}

	return state, nil
}

func (s *Service) Logs(agentID string, tail int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs, ok := s.logs[agentID]
	if !ok {
		return nil, ErrAgentNotFound
	}
	tail = boundTail(tail)
	if tail <= 0 || tail >= len(logs) {
		return append([]string(nil), logs...), nil
	}

	start := len(logs) - tail
	return append([]string(nil), logs[start:]...), nil
}

// MergedLogs returns the last `tail` log lines merged from all agents, sorted
// lexicographically (which equals chronological order for ISO-8601 prefixed
// lines produced by the process logger).
func (s *Service) MergedLogs(tail int) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tail = boundTail(tail)

	var merged []string
	for _, lines := range s.logs {
		merged = append(merged, lines...)
	}
	sort.Strings(merged)

	if tail > 0 && tail < len(merged) {
		merged = merged[len(merged)-tail:]
	}
	return merged
}

// MaxTailLines is the upper bound accepted for the tail parameter.
// Set to 1000 as a memory/performance guard: higher values would require
// buffering proportionally more log data in memory per request.
const MaxTailLines = 1000

func boundTail(n int) int {
	if n <= 0 {
		return 0
	}
	if n > MaxTailLines {
		return MaxTailLines
	}
	return n
}

func (s *Service) HandleFailure(ctx context.Context, agentID, lastError string) (baseagent.TriageResult, error) {
	s.mu.Lock()
	state, ok := s.states[agentID]
	if !ok {
		s.mu.Unlock()
		return baseagent.TriageResult{}, ErrAgentNotFound
	}

	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = lastError
	state.UpdatedAt = s.now()
	s.states[agentID] = state

	// Collect exit code from process manager
	if pm, ok := s.processManager.(*ProcessManager); ok {
		if exitCode := pm.GetExitCode(agentID); exitCode != nil {
			s.exitCodes[agentID] = exitCode
		}
	}
	s.mu.Unlock()

	// Collect comprehensive evidence
	evidence := s.evidenceCollector.Collect(agentID, lastError)
	baseEvidence := evidence.ToBaseAgentEvidence(lastError)

	triage, err := s.triager.Analyze(ctx, baseEvidence)
	if err != nil {
		return baseagent.TriageResult{}, err
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.LastTriageSummary = triage.Summary
	state.NeedsRemoteDiagnosis = triage.RequiresRemoteDiagnosis
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "base-agent", "triage", agentID, AuditResultSuccess, "", triage.Summary)

	// Persist triage information to disk
	s.saveState()

	return triage, nil
}

// MemoryStore returns the memory store, or nil if none was configured.
func (s *Service) MemoryStore() *memory.Store { return s.memoryStore }

func (s *Service) Diagnose(agentID string) (string, error) {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return "", err
	}

	s.mu.RLock()
	logs := append([]string(nil), s.logs[agentID]...)
	s.mu.RUnlock()

	if err := os.MkdirAll(s.diagnoseDir, 0o700); err != nil {
		s.recordAudit("", "system", "diagnose", agentID, AuditResultFailure, "E_DIAG_DIR", err.Error())
		return "", fmt.Errorf("create diagnose dir: %w", err)
	}

	fileName := fmt.Sprintf("%s-diagnose-%s.zip", agentID, s.now().UTC().Format("2006-01-02T15-04-05Z"))
	filePath := filepath.Join(s.diagnoseDir, fileName)
	if err := s.writeDiagnoseZip(filePath, m, state, logs, s.now()); err != nil {
		s.recordAudit("", "system", "diagnose", agentID, AuditResultFailure, "E_DIAG_WRITE", err.Error())
		return "", err
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.LastDiagnoseFile = filePath
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "diagnose", agentID, AuditResultSuccess, "", filePath)

	return filePath, nil
}

func (s *Service) CreateRemoteDiagnosisHandoff(agentID string, consent bool, actor, requestID string) (DiagnosisHandoff, error) {
	_, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return DiagnosisHandoff{}, err
	}
	if !state.NeedsRemoteDiagnosis {
		s.recordAudit(requestID, actor, "remote_diagnosis_consent", agentID, AuditResultFailure, "E_REMOTE_DIAG_NOT_NEEDED", "remote diagnosis not required")
		return DiagnosisHandoff{}, ErrRemoteDiagnosisNotNeeded
	}

	handoff := DiagnosisHandoff{
		ID:          s.idGenerator("handoff"),
		AgentID:     agentID,
		Consent:     consent,
		ArtifactRef: state.LastDiagnoseFile,
		CreatedAt:   s.now(),
	}
	if consent {
		handoff.Status = HandoffStatusPending
	} else {
		handoff.Status = HandoffStatusDeclined
	}

	s.mu.Lock()
	s.handoffs[handoff.ID] = handoff
	s.mu.Unlock()

	result := AuditResultSuccess
	if !consent {
		result = AuditResultNeutral
	}
	s.recordAudit(requestID, actor, "remote_diagnosis_consent", agentID, result, "", fmt.Sprintf("consent=%t handoff_id=%s", consent, handoff.ID))
	return handoff, nil
}

func (s *Service) DiagnosisHandoffs() []DiagnosisHandoff {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]DiagnosisHandoff, 0, len(s.handoffs))
	for _, h := range s.handoffs {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Cleanup stops all managed processes for graceful shutdown.
func (s *Service) Cleanup() {
	if s.processManager != nil {
		s.processManager.Cleanup()
	}
}

func (s *Service) CleanupExpiredDiagnosisHandoffs() int {
	cutoff := s.now().Add(-s.handoffTTL)

	s.mu.Lock()
	removed := 0
	for id, h := range s.handoffs {
		if h.CreatedAt.Before(cutoff) {
			delete(s.handoffs, id)
			removed++
		}
	}
	s.mu.Unlock()

	if removed > 0 {
		s.recordAudit("", "system", "handoff_cleanup", "diagnosis_handoffs", AuditResultSuccess, "", fmt.Sprintf("removed=%d", removed))
	}
	return removed
}
