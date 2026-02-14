package lifecycle

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/redact"
	"carrier/daemon/internal/runtimecheck"
)

var (
	ErrAgentNotFound            = errors.New("agent not found")
	ErrNotInstalled             = errors.New("agent is not installed")
	ErrAlreadyRunning           = errors.New("agent already running")
	ErrAlreadyStopped           = errors.New("agent already stopped")
	ErrCrashLoop                = errors.New("agent is in crash loop cooldown")
	ErrAgentRunning             = errors.New("agent is running; stop it before upgrading")
	ErrUpgradeNotSupported      = errors.New("agent manifest does not define an upgrade command")
	ErrMissingRequiredEnv       = errors.New("missing required environment variables")
	ErrPortConflict             = errors.New("port conflict detected")
	ErrRuntimePrerequisites     = errors.New("runtime prerequisites failed")
	ErrRemoteDiagnosisNotNeeded = errors.New("remote diagnosis is not required for this agent")
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

type Service struct {
	mu            sync.RWMutex
	states        map[string]AgentState
	manifests     map[string]manifest.Manifest
	memoryLinks   map[string][]string
	logs          map[string][]string
	handoffs      map[string]DiagnosisHandoff
	auditLogs     []AuditLog
	auditLogLimit int
	triager       baseagent.Triager
	checker       runtimecheck.Checker
	runner        commandexec.Runner
	diagnoseDir   string
	logLimit      int
	handoffTTL    time.Duration
	now           func() time.Time
	idCounter     uint64
	idGenerator   func(prefix string) string
	restarts           map[string][]time.Time
	cooldowns          map[string]time.Time
	crashLoopThreshold int
	crashLoopWindow    time.Duration
	crashLoopCooldown  time.Duration
}

func NewService(triager baseagent.Triager, opts ...Option) *Service {
	if triager == nil {
		triager = baseagent.NoopTriager{}
	}

	svc := &Service{
		states:        make(map[string]AgentState),
		manifests:     make(map[string]manifest.Manifest),
		memoryLinks:   make(map[string][]string),
		logs:          make(map[string][]string),
		handoffs:      make(map[string]DiagnosisHandoff),
		auditLogs:     make([]AuditLog, 0, 128),
		auditLogLimit: 1000,
		triager:       triager,
		checker:       runtimecheck.NewHostChecker(),
		runner:        commandexec.NewShellRunner(),
		diagnoseDir:   filepath.Join(os.TempDir(), "agentd-diagnose"),
		logLimit:      1000,
		handoffTTL:    24 * time.Hour,
		now:           time.Now,
		restarts:           make(map[string][]time.Time),
		cooldowns:          make(map[string]time.Time),
		crashLoopThreshold: defaultCrashLoopThreshold,
		crashLoopWindow:    defaultCrashLoopWindow,
		crashLoopCooldown:  defaultCrashLoopCooldown,
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
	if _, ok := s.memoryLinks[m.ID]; !ok {
		s.memoryLinks[m.ID] = nil
	}
	s.logs[m.ID] = nil
	s.restarts[m.ID] = nil
	s.cooldowns[m.ID] = time.Time{}

	return nil
}

type upgradeBackup struct {
	AgentID           string            `json:"agent_id"`
	CreatedAt         time.Time         `json:"created_at"`
	CurrentVersion    string            `json:"current_version"`
	EnvVarKeys        []string          `json:"env_var_keys"`
	MemoryAttachments []string          `json:"memory_attachments"`
	RuntimeState      AgentState        `json:"runtime_state"`
	Config            manifest.Manifest `json:"config"`
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
	if err := s.blockIfCrashLoopCoolingDown(agentID, state); err != nil {
		return err
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
		s.updateStateOnStartError(agentID, runErr)
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
	delete(s.restarts, agentID)
	delete(s.cooldowns, agentID)
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

func (s *Service) Upgrade(agentID string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	if state.Install != InstallStateInstalled {
		return ErrNotInstalled
	}
	if state.Runtime == RuntimeStateRunning {
		return ErrAgentRunning
	}
	if strings.TrimSpace(m.Runtime.Upgrade.Command) == "" {
		return ErrUpgradeNotSupported
	}

	attachments := s.getMemoryAttachments(agentID)
	backupPath, err := s.createUpgradeBackup(agentID, m, state, attachments)
	if err != nil {
		return err
	}

	s.recordAudit("", "system", "upgrade", agentID, AuditResultSuccess, "", fmt.Sprintf("upgrade_start backup=%q command=%q", backupPath, m.Runtime.Upgrade.Command))

	result, runErr := s.runner.Run(context.Background(), m.Runtime.Upgrade.Command)
	s.appendCommandLog(agentID, "upgrade", m.Runtime.Upgrade.Command, result, runErr)
	if runErr != nil {
		s.recordAudit("", "system", "upgrade", agentID, AuditResultFailure, "E_UPGRADE_FAILED", fmt.Sprintf("upgrade_failure backup=%q error=%q", backupPath, runErr.Error()))
		return fmt.Errorf("upgrade failed; use backup at %s for manual rollback guidance: %w", backupPath, runErr)
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.restarts[agentID] = nil
	delete(s.cooldowns, agentID)
	s.mu.Unlock()

	s.recordAudit("", "system", "upgrade", agentID, AuditResultSuccess, "", fmt.Sprintf("upgrade_success backup=%q", backupPath))
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
	now := s.now()
	restarts := append(s.restarts[agentID], now)
	restarts = trimRestartHistory(restarts, now.Add(-s.crashLoopWindow))
	s.restarts[agentID] = restarts
	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	if len(restarts) >= s.crashLoopThreshold {
		cooldownUntil := now.Add(s.crashLoopCooldown)
		s.cooldowns[agentID] = cooldownUntil
		state.LastError = fmt.Sprintf(
			"crash-loop detected: %d restarts within %s; cooldown until %s; last error: %v",
			len(restarts),
			s.crashLoopWindow.String(),
			cooldownUntil.UTC().Format(time.RFC3339),
			err,
		)
	}
	state.UpdatedAt = now
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
		ln, err := listenTCP("tcp", addr)
		if err != nil {
			return fmt.Errorf("%w: %s (%d) is in use by %s", ErrPortConflict, port.Name, port.Port, portOccupantFor(port.Port))
		}
		_ = ln.Close()
	}
	return nil
}

func (s *Service) blockIfCrashLoopCoolingDown(agentID string, state AgentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cooldownUntil, ok := s.cooldowns[agentID]
	if !ok || cooldownUntil.IsZero() {
		return nil
	}

	now := s.now()
	if !now.Before(cooldownUntil) {
		delete(s.cooldowns, agentID)
		delete(s.restarts, agentID)
		return nil
	}

	restartCount := len(trimRestartHistory(s.restarts[agentID], now.Add(-s.crashLoopWindow)))
	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = fmt.Sprintf(
		"crash-loop detected: %d restarts within %s; cooldown until %s",
		restartCount,
		s.crashLoopWindow.String(),
		cooldownUntil.UTC().Format(time.RFC3339),
	)
	state.UpdatedAt = now
	s.states[agentID] = state

	return fmt.Errorf("%w: %s", ErrCrashLoop, state.LastError)
}

func trimRestartHistory(history []time.Time, windowStart time.Time) []time.Time {
	if len(history) == 0 {
		return history
	}
	firstKept := 0
	for firstKept < len(history) && history[firstKept].Before(windowStart) {
		firstKept++
	}
	if firstKept == 0 {
		return history
	}
	if firstKept >= len(history) {
		return nil
	}
	return append([]time.Time(nil), history[firstKept:]...)
}

func describePortOccupant(port int) string {
	if runtime.GOOS != "linux" {
		return "an unknown process"
	}

	inode, err := findListeningSocketInode(port)
	if err != nil || inode == "" {
		return "an unknown process"
	}

	pid, processName, err := findProcessBySocketInode(inode)
	if err != nil || pid <= 0 {
		return fmt.Sprintf("socket inode %s (pid unknown)", inode)
	}
	if processName == "" {
		return fmt.Sprintf("pid %d", pid)
	}
	return fmt.Sprintf("pid %d (%s)", pid, processName)
}

func findListeningSocketInode(port int) (string, error) {
	for _, procFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		inode, err := findListeningSocketInodeInFile(procFile, port)
		if err == nil && inode != "" {
			return inode, nil
		}
	}
	return "", errors.New("listening socket not found")
}

func findListeningSocketInodeInFile(path string, port int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "sl") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		localAddress := fields[1]
		state := fields[3]
		if state != "0A" {
			continue
		}
		parts := strings.Split(localAddress, ":")
		if len(parts) != 2 {
			continue
		}
		p, err := strconv.ParseInt(parts[1], 16, 32)
		if err != nil || int(p) != port {
			continue
		}
		return fields[9], nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("socket not found")
}

func findProcessBySocketInode(inode string) (int, string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", err
	}
	target := "socket:[" + inode + "]"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			linkPath := filepath.Join(fdDir, fd.Name())
			link, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}
			if link != target {
				continue
			}
			nameBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
			if err != nil {
				return pid, "", nil
			}
			return pid, strings.TrimSpace(string(nameBytes)), nil
		}
	}
	return 0, "", errors.New("process not found")
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

func (s *Service) getMemoryAttachments(agentID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attachments := s.memoryLinks[agentID]
	return append([]string(nil), attachments...)
}

func (s *Service) setMemoryAttachments(agentID string, attachments []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memoryLinks[agentID] = append([]string(nil), attachments...)
}

func (s *Service) envVarKeys(m manifest.Manifest) []string {
	seen := make(map[string]struct{}, len(m.Env.Required)+len(m.Env.Optional))
	keys := make([]string, 0, len(m.Env.Required)+len(m.Env.Optional))
	for _, envVar := range m.Env.Required {
		if _, ok := seen[envVar.Name]; !ok {
			seen[envVar.Name] = struct{}{}
			keys = append(keys, envVar.Name)
		}
	}
	for _, envVar := range m.Env.Optional {
		if _, ok := seen[envVar.Name]; !ok {
			seen[envVar.Name] = struct{}{}
			keys = append(keys, envVar.Name)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) createUpgradeBackup(agentID string, m manifest.Manifest, state AgentState, attachments []string) (string, error) {
	if err := os.MkdirAll(s.diagnoseDir, 0o755); err != nil {
		return "", fmt.Errorf("create diagnose dir: %w", err)
	}

	fileName := fmt.Sprintf("%s-pre-upgrade-%s.json", agentID, s.now().UTC().Format("2006-01-02T15-04-05Z"))
	filePath := filepath.Join(s.diagnoseDir, fileName)
	backup := upgradeBackup{
		AgentID:           agentID,
		CreatedAt:         s.now().UTC(),
		CurrentVersion:    state.Version,
		EnvVarKeys:        s.envVarKeys(m),
		MemoryAttachments: append([]string(nil), attachments...),
		RuntimeState:      state,
		Config:            m,
	}

	content, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal upgrade backup: %w", err)
	}
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return "", fmt.Errorf("write upgrade backup: %w", err)
	}

	return filePath, nil
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
	if len(s.auditLogs) > s.auditLogLimit {
		s.auditLogs = s.auditLogs[len(s.auditLogs)-s.auditLogLimit:]
	}
}

func (s *Service) writeDiagnoseZip(path string, m manifest.Manifest, state AgentState, logs []string, createdAt time.Time) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create diagnose zip: %w", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)

	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	redactedEnvJSON, err := json.MarshalIndent(redact.RedactEnviron(os.Environ()), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}

	artifacts := map[string][]byte{
		"state.json":    []byte(redact.RedactText(string(stateJSON))),
		"manifest.json": []byte(redact.RedactText(string(manifestJSON))),
		"logs.txt":      []byte(redact.RedactText(strings.Join(logs, "\n"))),
		"env.json":      redactedEnvJSON,
	}

	metadataJSON, err := redact.MetadataJSON(createdAt, 24*time.Hour, artifacts)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	artifacts["metadata.json"] = metadataJSON

	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := addZipFile(zipWriter, name, artifacts[name]); err != nil {
			return err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
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
