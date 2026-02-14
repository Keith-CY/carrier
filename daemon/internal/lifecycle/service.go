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
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/runtimecheck"
)

var (
	ErrAgentNotFound        = errors.New("agent not found")
	ErrNotInstalled         = errors.New("agent is not installed")
	ErrAlreadyRunning       = errors.New("agent already running")
	ErrAlreadyStopped       = errors.New("agent already stopped")
	ErrMissingRequiredEnv   = errors.New("missing required environment variables")
	ErrPortConflict         = errors.New("port conflict detected")
	ErrRuntimePrerequisites = errors.New("runtime prerequisites failed")
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

type Service struct {
	mu          sync.RWMutex
	states      map[string]AgentState
	manifests   map[string]manifest.Manifest
	logs        map[string][]string
	triager     baseagent.Triager
	checker     runtimecheck.Checker
	runner      commandexec.Runner
	diagnoseDir string
	logLimit    int
	now         func() time.Time
}

func NewService(triager baseagent.Triager, opts ...Option) *Service {
	if triager == nil {
		triager = baseagent.NoopTriager{}
	}

	svc := &Service{
		states:      make(map[string]AgentState),
		manifests:   make(map[string]manifest.Manifest),
		logs:        make(map[string][]string),
		triager:     triager,
		checker:     runtimecheck.NewHostChecker(),
		runner:      commandexec.NewShellRunner(),
		diagnoseDir: filepath.Join(os.TempDir(), "agentd-diagnose"),
		logLimit:    1000,
		now:         time.Now,
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
		s.updateStateOnInstallError(state, err)
		return err
	}

	result, runErr := s.runner.Run(context.Background(), m.Runtime.Install.Command)
	s.appendCommandLog(agentID, "install", m.Runtime.Install.Command, result, runErr)
	if runErr != nil {
		s.updateStateOnInstallError(state, runErr)
		return runErr
	}

	s.mu.Lock()
	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()

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
		s.updateStateOnStartError(state, err)
		return err
	}
	if err := s.validateRequiredEnv(m); err != nil {
		s.updateStateOnStartError(state, err)
		return err
	}
	if err := s.ensurePortsAvailable(m.Network.Ports); err != nil {
		s.updateStateOnStartError(state, err)
		return err
	}

	result, runErr := s.runner.Run(context.Background(), m.Runtime.Start.Command)
	s.appendCommandLog(agentID, "start", m.Runtime.Start.Command, result, runErr)
	if runErr != nil {
		triage, triageErr := s.HandleFailure(context.Background(), agentID, runErr.Error())
		if triageErr == nil {
			s.appendLog(agentID, fmt.Sprintf("triage summary: %s", triage.Summary))
		}
		return runErr
	}

	s.mu.Lock()
	state.Runtime = RuntimeStateRunning
	state.Health = HealthStateHealthy
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()

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
		state.LastError = runErr.Error()
		state.UpdatedAt = s.now()
		s.states[agentID] = state
		s.mu.Unlock()
		return runErr
	}

	s.mu.Lock()
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()

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

func (s *Service) Diagnose(agentID string) (string, error) {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return "", err
	}

	s.mu.RLock()
	logs := append([]string(nil), s.logs[agentID]...)
	s.mu.RUnlock()

	if err := os.MkdirAll(s.diagnoseDir, 0o755); err != nil {
		return "", fmt.Errorf("create diagnose dir: %w", err)
	}

	fileName := fmt.Sprintf("%s-diagnose-%s.zip", agentID, s.now().UTC().Format("2006-01-02T15-04-05Z"))
	filePath := filepath.Join(s.diagnoseDir, fileName)
	if err := s.writeDiagnoseZip(filePath, m, state, logs); err != nil {
		return "", err
	}

	s.mu.Lock()
	state.LastDiagnoseFile = filePath
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()

	return filePath, nil
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

	return triage, nil
}

func (s *Service) updateStateOnInstallError(state AgentState, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.Install = InstallStateBroken
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	state.UpdatedAt = s.now()
	s.states[state.ID] = state
}

func (s *Service) updateStateOnStartError(state AgentState, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	state.UpdatedAt = s.now()
	s.states[state.ID] = state
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
