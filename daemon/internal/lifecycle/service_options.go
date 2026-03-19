package lifecycle

import (
	"time"

	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/memory"
	"carrier/daemon/internal/runtimecheck"
)

type Option func(*Service)

type ProcessController interface {
	Start(agentID string, command string, args []string) (int, error)
	Stop(agentID string) error
	IsRunning(agentID string) bool
	Wait(agentID string) error
	Cleanup()
}

type ProcessControllerWithEnv interface {
	ProcessController
	StartWithEnv(agentID string, command string, args []string, env map[string]string) (int, error)
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

func WithAuditLogDir(dir string) Option {
	return func(s *Service) {
		if dir != "" {
			s.auditLogDir = dir
		}
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

func WithCommandTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.commandTimeout = timeout
		}
	}
}

func WithAlertManager(manager *AlertManager) Option {
	return func(s *Service) {
		if manager != nil {
			s.alertManager = manager
		}
	}
}

func WithWebhookManager(manager *WebhookManager) Option {
	return func(s *Service) {
		if manager != nil {
			s.webhookManager = manager
		}
	}
}

func loadCommandTimeoutFromEnv(raw string) time.Duration {
	if raw == "" {
		return defaultCommandTimeout
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return defaultCommandTimeout
	}
	return timeout
}
