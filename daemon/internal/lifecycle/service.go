package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"carrier/daemon/internal/baseagent"
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
	ErrRemoteDiagnosisNotNeeded   = errors.New("remote diagnosis is not required for this agent")
	ErrUpgradeFailed              = errors.New("agent upgrade failed")
	ErrUpgradeStrategyUnsupported = errors.New("upgrade strategy is not supported")
)

const (
	defaultCrashLoopThreshold = 3
	defaultCrashLoopWindow    = 5 * time.Minute
	defaultCrashLoopCooldown  = 5 * time.Minute
)

var (
	listenTCP       = net.Listen
	portOccupantFor = describePortOccupant
)

type Option func(*Service)

type ProcessController interface {
	Start(agentID string, command string, args []string) (int, error)
	Stop(agentID string) error
	IsRunning(agentID string) bool
	Wait(agentID string) error
	Cleanup()
}

func WithRunner(r commandexec.Runner) Option {
	return func(s *Service) { s.runner = r }
}

func WithRuntimeChecker(c runtimecheck.Checker) Option {
	return func(s *Service) { s.checker = c }
}

func WithDiagnoseDir(dir string) Option {
	return func(s *Service) { s.diagnoseDir = dir }
}

func WithLogLimit(limit int) Option {
	return func(s *Service) {
		if limit > 0 {
			s.logLimit = limit
		}
	}
}

// WithCrashLoopConfig overrides the default crash-loop detection parameters.
//   - threshold: number of restarts within window that triggers cooldown (default: 3)
//   - window:    sliding time window for counting restarts (default: 5m)
//   - cooldown:  duration to block restarts after a crash loop is detected (default: 5m)
func WithCrashLoopConfig(threshold int, window, cooldown time.Duration) Option {
	return func(s *Service) {
		if threshold > 0 {
			s.crashLoopThreshold = threshold
		}
		if window > 0 {
			s.crashLoopWindow = window
		}
		if cooldown > 0 {
			s.crashLoopCooldown = cooldown
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithAuditLogLimit(limit int) Option {
	return func(s *Service) {
		if limit > 0 {
			s.auditLogLimit = limit
		}
	}
}

func WithIDGenerator(gen func(prefix string) string) Option {
	return func(s *Service) {
		if gen != nil {
			s.idGenerator = gen
		}
	}
}

func WithHandoffRetention(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.handoffTTL = ttl
		}
	}
}

func WithMemoryStore(ms *memory.Store) Option {
	return func(s *Service) {
		if ms != nil {
			s.memoryStore = ms
		}
	}
}

func WithStateFile(path string) Option {
	return func(s *Service) {
		s.stateFile = NewStateFile(path)
	}
}

func WithProcessLogDir(dir string) Option {
	return func(s *Service) {
		if dir != "" {
			s.processLogDir = dir
		}
	}
}

func WithProcessManager(pm ProcessController) Option {
	return func(s *Service) {
		if pm != nil {
			s.processManager = pm
		}
	}
}

func WithBackoffPolicy(policy BackoffPolicy) Option {
	return func(s *Service) {
		s.backoffPolicy = policy
	}
}

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
	logLimit           int
	handoffTTL         time.Duration
	now                func() time.Time
	idCounter          uint64
	idGenerator        func(prefix string) string
	restarts           map[string][]time.Time
	cooldowns          map[string]time.Time
	crashLoopThreshold int
	crashLoopWindow    time.Duration
	crashLoopCooldown  time.Duration
	memoryStore        *memory.Store
	stateFile          *StateFile
	processManager     ProcessController
	processLogDir      string
	backoffPolicy      BackoffPolicy
	backoffStates      map[string]BackoffState
	exitCodes          map[string]*int
	evidenceCollector  *EvidenceCollector
}

func NewService(triager baseagent.Triager, opts ...Option) *Service {
	if triager == nil {
		triager = baseagent.NoopTriager{}
	}

	processLogDir := filepath.Join(os.TempDir(), "agentd-process-logs")
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
		diagnoseDir:        filepath.Join(os.TempDir(), "agentd-diagnose"),
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
	}
	svc.processManager = NewProcessManager(processLogDir)
	svc.evidenceCollector = NewEvidenceCollector(logs, exitCodes, 1000)
	svc.idGenerator = func(prefix string) string {
		next := atomic.AddUint64(&svc.idCounter, 1)
		return fmt.Sprintf("%s-%d", prefix, next)
	}
	for _, opt := range opts {
		opt(svc)
	}

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

	s.manifests[m.ID] = m
	s.states[m.ID] = AgentState{
		ID:        m.ID,
		Version:   m.Version,
		Install:   InstallStateNotInstalled,
		Runtime:   RuntimeStateStopped,
		Health:    HealthStateUnknown,
		UpdatedAt: s.now(),
	}
	if _, ok := s.memoryLinks[m.ID]; !ok {
		s.memoryLinks[m.ID] = nil
	}
	s.logs[m.ID] = nil
	s.restarts[m.ID] = nil
	s.cooldowns[m.ID] = time.Time{}
	s.backoffStates[m.ID] = BackoffState{}

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
	if tail <= 0 || tail >= len(logs) {
		return append([]string(nil), logs...), nil
	}

	start := len(logs) - tail
	return append([]string(nil), logs[start:]...), nil
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

	if err := os.MkdirAll(s.diagnoseDir, 0o755); err != nil {
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

// loadPersistedState restores agent state from the state file.
// Only restores Install and Runtime state for registered agents.
// Verifies that processes marked as "running" are actually alive.
func (s *Service) loadPersistedState() error {
	if s.stateFile == nil {
		return nil
	}

	persisted, err := s.stateFile.Load()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, pState := range persisted {
		// Only restore state for agents that have been registered
		if state, ok := s.states[id]; ok {
			if pState.Installed {
				state.Install = InstallStateInstalled
			} else {
				state.Install = InstallStateNotInstalled
			}

			// Restore runtime state, but verify process liveness
			restoredState := RuntimeState(pState.RuntimeState)
			if restoredState == RuntimeStateRunning {
				// Verify the process is actually alive
				if !s.processManager.IsRunning(id) {
					// Process is not actually running, mark as stopped
					restoredState = RuntimeStateStopped
				}
			}
			state.Runtime = restoredState
			state.UpdatedAt = pState.LastTransition
			s.states[id] = state
		}
	}

	return nil
}

// saveState persists the current agent states to disk.
func (s *Service) saveState() {
	if s.stateFile == nil {
		return
	}

	s.mu.RLock()
	// Create a map of pointers for Save
	agents := make(map[string]*AgentState, len(s.states))
	for id := range s.states {
		state := s.states[id]
		agents[id] = &state
	}
	s.mu.RUnlock()

	// Save asynchronously to avoid blocking lifecycle operations
	// In production, you might want to handle errors more carefully
	_ = s.stateFile.Save(agents)
}
