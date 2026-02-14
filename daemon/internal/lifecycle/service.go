package lifecycle

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/runtimecheck"
)

var (
	ErrAgentNotFound            = errors.New("agent not found")
	ErrNotInstalled             = errors.New("agent is not installed")
	ErrAlreadyRunning           = errors.New("agent already running")
	ErrAlreadyStopped           = errors.New("agent already stopped")
	ErrMissingRequiredEnv       = errors.New("missing required environment variables")
	ErrPortConflict             = errors.New("port conflict detected")
	ErrRuntimePrerequisites     = errors.New("runtime prerequisites failed")
	ErrRemoteDiagnosisNotNeeded = errors.New("remote diagnosis is not required for this agent")
)

type Option func(*Service)

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

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
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

type Service struct {
	mu          sync.RWMutex
	states      map[string]AgentState
	manifests   map[string]manifest.Manifest
	logs        map[string][]string
	handoffs    map[string]DiagnosisHandoff
	auditLogs   []AuditLog
	triager     baseagent.Triager
	checker     runtimecheck.Checker
	runner      commandexec.Runner
	diagnoseDir string
	logLimit    int
	handoffTTL  time.Duration
	now         func() time.Time
	idCounter   uint64
	idGenerator func(prefix string) string
}

func NewService(triager baseagent.Triager, opts ...Option) *Service {
	if triager == nil {
		triager = baseagent.NoopTriager{}
	}

	svc := &Service{
		states:      make(map[string]AgentState),
		manifests:   make(map[string]manifest.Manifest),
		logs:        make(map[string][]string),
		handoffs:    make(map[string]DiagnosisHandoff),
		auditLogs:   make([]AuditLog, 0, 128),
		triager:     triager,
		checker:     runtimecheck.NewHostChecker(),
		runner:      commandexec.NewShellRunner(),
		diagnoseDir: filepath.Join(os.TempDir(), "agentd-diagnose"),
		logLimit:    1000,
		handoffTTL:  24 * time.Hour,
		now:         time.Now,
	}
	svc.idGenerator = func(prefix string) string {
		next := atomic.AddUint64(&svc.idCounter, 1)
		return fmt.Sprintf("%s-%d", prefix, next)
	}
	for _, opt := range opts {
		opt(svc)
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
	s.logs[m.ID] = nil

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

func (s *Service) Install(agentID string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}

	if err := s.checkRuntimePrerequisites(m); err != nil {
		s.updateStateOnInstallError(agentID, err)
		s.recordAudit("", "system", "install", agentID, AuditResultFailure, "E_RUNTIME_PREREQUISITES", err.Error())
		return err
	}

	result, runErr := s.runner.Run(context.Background(), m.Runtime.Install.Command)
	s.appendCommandLog(agentID, "install", m.Runtime.Install.Command, result, runErr)
	if runErr != nil {
		s.updateStateOnInstallError(agentID, runErr)
		s.recordAudit("", "system", "install", agentID, AuditResultFailure, "E_INSTALL_FAILED", runErr.Error())
		return runErr
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "install", agentID, AuditResultSuccess, "", "install completed")

	return nil
}

func (s *Service) Start(agentID string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	if state.Install != InstallStateInstalled {
		return ErrNotInstalled
	}
	if state.Runtime == RuntimeStateRunning {
		return ErrAlreadyRunning
	}

	if err := s.checkRuntimePrerequisites(m); err != nil {
		s.updateStateOnStartError(agentID, err)
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_RUNTIME_PREREQUISITES", err.Error())
		return err
	}
	if err := s.validateRequiredEnv(m); err != nil {
		s.updateStateOnStartError(agentID, err)
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_ENV_MISSING", err.Error())
		return err
	}
	if err := s.ensurePortsAvailable(m.Network.Ports); err != nil {
		s.updateStateOnStartError(agentID, err)
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_PORT_CONFLICT", err.Error())
		return err
	}

	result, runErr := s.runner.Run(context.Background(), m.Runtime.Start.Command)
	s.appendCommandLog(agentID, "start", m.Runtime.Start.Command, result, runErr)
	if runErr != nil {
		triage, triageErr := s.HandleFailure(context.Background(), agentID, runErr.Error())
		if triageErr == nil {
			s.appendLog(agentID, fmt.Sprintf("triage summary: %s", triage.Summary))
		}
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_START_FAILED", runErr.Error())
		return runErr
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Runtime = RuntimeStateRunning
	state.Health = HealthStateHealthy
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "start", agentID, AuditResultSuccess, "", "start completed")

	return nil
}

func (s *Service) Stop(agentID string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	if state.Runtime == RuntimeStateStopped {
		return ErrAlreadyStopped
	}

	result, runErr := s.runner.Run(context.Background(), m.Runtime.Stop.Command)
	s.appendCommandLog(agentID, "stop", m.Runtime.Stop.Command, result, runErr)
	if runErr != nil {
		s.mu.Lock()
		state = s.states[agentID]
		state.LastError = runErr.Error()
		state.UpdatedAt = s.now()
		s.states[agentID] = state
		s.mu.Unlock()
		s.recordAudit("", "system", "stop", agentID, AuditResultFailure, "E_STOP_FAILED", runErr.Error())
		return runErr
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "stop", agentID, AuditResultSuccess, "", "stop completed")

	return nil
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

func (s *Service) AuditLogs() []AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]AuditLog(nil), s.auditLogs...)
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
	if err := s.writeDiagnoseZip(filePath, m, state, logs); err != nil {
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
		result = AuditResultFailure
	}
	s.recordAudit(requestID, actor, "remote_diagnosis_consent", agentID, result, "", fmt.Sprintf("consent=%t handoff_id=%s", consent, handoff.ID))
	return handoff, nil
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
	s.mu.Unlock()

	triage, err := s.triager.Analyze(ctx, baseagent.Evidence{AgentID: agentID, LastError: lastError})
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

	return triage, nil
}

func (s *Service) updateStateOnInstallError(agentID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentID]
	if !ok {
		return
	}
	state.Install = InstallStateBroken
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	state.UpdatedAt = s.now()
	s.states[agentID] = state
}

func (s *Service) updateStateOnStartError(agentID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentID]
	if !ok {
		return
	}
	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	state.UpdatedAt = s.now()
	s.states[agentID] = state
}

func (s *Service) checkRuntimePrerequisites(m manifest.Manifest) error {
	if err := s.checker.Check(m); err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimePrerequisites, err)
	}
	return nil
}

func (s *Service) validateRequiredEnv(m manifest.Manifest) error {
	missing := make([]string, 0)
	for _, envVar := range m.Env.Required {
		if strings.TrimSpace(os.Getenv(envVar.Name)) == "" {
			missing = append(missing, envVar.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrMissingRequiredEnv, strings.Join(missing, ","))
}

func (s *Service) ensurePortsAvailable(ports []manifest.PortSpec) error {
	for _, port := range ports {
		if port.Port <= 0 {
			continue
		}
		addr := fmt.Sprintf("127.0.0.1:%d", port.Port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("%w: %s (%d)", ErrPortConflict, port.Name, port.Port)
		}
		_ = ln.Close()
	}
	return nil
}

func (s *Service) getManifestAndState(agentID string) (manifest.Manifest, AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.manifests[agentID]
	if !ok {
		return manifest.Manifest{}, AgentState{}, ErrAgentNotFound
	}
	state := s.states[agentID]
	return m, state, nil
}

func (s *Service) appendCommandLog(agentID, action, command string, result commandexec.Result, runErr error) {
	line := fmt.Sprintf("[%s] command=%q exit=%d", action, command, result.ExitCode)
	s.appendLog(agentID, line)
	if result.CombinedOutput != "" {
		s.appendLog(agentID, fmt.Sprintf("[%s] output=%s", action, result.CombinedOutput))
	}
	if runErr != nil {
		s.appendLog(agentID, fmt.Sprintf("[%s] error=%v", action, runErr))
	}
}

func (s *Service) appendLog(agentID, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.logs[agentID]
	if !ok {
		return
	}
	entry := fmt.Sprintf("%s %s", s.now().UTC().Format(time.RFC3339), line)
	entries = append(entries, entry)
	if len(entries) > s.logLimit {
		entries = entries[len(entries)-s.logLimit:]
	}
	s.logs[agentID] = entries
}

func (s *Service) recordAudit(requestID, actor, action, target string, result AuditResult, errorCode, message string) {
	if actor == "" {
		actor = "system"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.auditLogs = append(s.auditLogs, AuditLog{
		RequestID: requestID,
		Actor:     actor,
		Action:    action,
		Target:    target,
		Result:    result,
		ErrorCode: errorCode,
		Message:   message,
		Timestamp: s.now(),
	})
}

func (s *Service) writeDiagnoseZip(path string, m manifest.Manifest, state AgentState, logs []string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create diagnose zip: %w", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := addZipFile(zipWriter, "state.json", stateJSON); err != nil {
		return err
	}
	if err := addZipFile(zipWriter, "manifest.json", manifestJSON); err != nil {
		return err
	}
	if err := addZipFile(zipWriter, "logs.txt", []byte(strings.Join(logs, "\n"))); err != nil {
		return err
	}

	return nil
}

func addZipFile(zw *zip.Writer, name string, content []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("write zip entry %s: %w", name, err)
	}
	return nil
}
