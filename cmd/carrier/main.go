// Carrier — unified binary for the Carrier agent platform.
//
// Usage:
//
//	carrier                 Bootstrap Carrier (onboard if needed, keep daemon+gateway running, then exit)
//	carrier daemon          Start daemon HTTP API server (foreground)
//	carrier gateway         Start gateway HTTP server
//	carrier stop            Stop background daemon and gateway started by Carrier
//	carrier stop <id|name>  Stop a managed agent instance
//	carrier start <id|name> Start a managed agent instance
//	carrier status <id|name> Show status for a managed agent instance
//	carrier upgrade <id|name> Upgrade a managed agent instance
//	carrier uninstall <id|name> Uninstall and remove a managed agent instance
//	carrier list            List managed agent instances
//	carrier onboard [--tui|--cli]
//	                       Interactive terminal onboarding (channel/provider -> keep gateway running in background)
//	carrier onboard --webui Launch WebUI onboarding (start/reuse daemon+gateway)
//	carrier add <agent_id> [--isolation|--no-isolation] [--tui|--cli]
//	                       Add/install an agent via terminal flow
//	carrier add <agent_id> [--isolation|--no-isolation] --webui
//	                       Add/install an agent via WebUI flow
//	carrier install <agent_id> [--isolation|--no-isolation] [--tui|--cli|--webui]
//	                       Alias for `carrier add <agent_id>`
//	carrier --help          Show usage
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"carrier/configv2"
	"carrier/daemon/credentialstore"
	"carrier/daemon/server"

	gatewayruntime "carrier/gateway"
	"carrier/shared/catalog"
	"carrier/shared/openclawcfg"
	sharedorchestration "carrier/shared/orchestration"
	gossh "golang.org/x/crypto/ssh"
)

type choiceOption struct {
	ID      string
	Name    string
	Setup   string
	Aliases []string

	AuthMode     providerAuthMode
	ProviderEnv  string
	ExampleModel string

	TokenEnv     string
	RequireToken bool
	SecretEnv    string
}

type providerAuthMode = catalog.AuthMode

const (
	authModeAPIKey          providerAuthMode = catalog.AuthModeAPIKey
	authModeOAuthDeviceCode providerAuthMode = catalog.AuthModeOAuthDeviceCode
	authModeNone            providerAuthMode = catalog.AuthModeNone
)

type addCommandOptions struct {
	AgentID   string
	Isolation bool
	WebUI     bool
	CLI       bool
	TUI       bool
	Quiet     bool
}

type onboardCommandOptions struct {
	WebUI bool
	CLI   bool
	TUI   bool
}

type statusCommandOptions struct {
	Target  string
	Metrics bool
}

type updateCommandOptions struct {
	Check     bool
	Yes       bool
	DryRun    bool
	Force     bool
	Channel   string
	Tag       string
	Timeout   time.Duration
	JSON      bool
	NoRestart bool
}

type versionCommandOptions struct {
	JSON bool
}

type doctorCommandOptions struct {
	JSON bool
}

type logsCommandOptions struct {
	AgentID string
	Since   time.Time
	Level   string
	Grep    string
	Export  string
}

type serviceCommandOptions struct {
	Action string
}

type catalogCommandOptions struct {
	Action       string
	ManifestPath string
	ID           string
}

type webhooksCommandOptions struct {
	Action string
}

type configCommandOptions struct {
	Action     string
	Agent      string
	Key        string
	Value      string
	Domain     string
	OutputPath string
	FromPath   string
}

type keysCommandOptions struct {
	Action string
	Name   string
}

type remoteCommandOptions struct {
	Action             string
	AgentID            string
	InstallAgentID     string
	TargetAgentID      string
	All                bool
	Concurrency        int
	Isolation          bool
	HostID             string
	HostName           string
	HostAddr           string
	Port               int
	User               string
	AuthMode           string
	KeyPath            string
	KeyRef             string
	SSHConfigHost      string
	RuntimeMode        string
	CheckRetries       int
	CheckRetryDelaySec int
	SkipReconnectCheck bool
	AutoRollback       bool
	Tail               int
	Commit             string
	KeyType            string
	KeyImportPath      string
	KeyOutputPath      string
	SyncChannels       []string
	SyncProviders      []string
	TelegramAllowFrom  []string
	DiscordAllowFrom   []string
}

type remoteStoreCommandOptions struct {
	Action     string
	OutputPath string
	FromPath   string
}

type orchestrateCommandOptions struct {
	Action         string
	Goal           string
	ExecutionID    string
	TemplateID     string
	HostIDs        []string
	HostLabels     []string
	Provider       string
	Format         string
	MaxConcurrency int
	PolicyApprove  bool
	IdempotencyKey string
	Limit          int
	OutputPath     string
	Timeout        time.Duration
	Async          bool
	JSON           bool
	Inputs         map[string]string
}

type templatesCommandOptions struct {
	Action         string
	TemplateID     string
	HostIDs        []string
	HostLabels     []string
	Provider       string
	MaxConcurrency int
	PolicyApprove  bool
	JSON           bool
	Inputs         map[string]string
}

type triggersCommandOptions struct {
	Action           string
	TriggerID        string
	Type             string
	TemplateID       string
	Name             string
	CreatedBy        string
	HostIDs          []string
	HostLabels       []string
	Provider         string
	MaxConcurrency   int
	PolicyApprove    bool
	WebhookSecret    string
	GitHubCommand    string
	GitHubLabel      string
	GitHubRepository string
	Cron             string
	Timezone         string
	Enable           bool
	Disable          bool
	JSON             bool
	Inputs           map[string]string
}

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

type picoclawChannel struct {
	ID         string
	Name       string
	TokenLabel string
}

type managedAgentAddResult struct {
	InstanceID    string
	WorkspacePath string
	ConfigPath    string
	RecordPath    string
	ChannelID     string
	ProviderID    string
	PairedChatID  string
	Port          int
}

type managedAgentConfig struct {
	ID                  string
	Name                string
	ConfigDir           string
	RequiredEnvKey      string
	OptionalPopulateKey string
}

type managedAgentInstance struct {
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type"`
	AgentID      string `json:"agent_id"`
	Isolation    bool   `json:"isolation,omitempty"`
	GatewayURL   string `json:"gateway_url"`
	Workspace    string `json:"workspace_path,omitempty"`
	ConfigPath   string `json:"config_path,omitempty"`
	RecordPath   string `json:"record_path,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Provider     string `json:"provider,omitempty"`
	PairRequired bool   `json:"pair_required,omitempty"`
	PairCode     string `json:"pair_code,omitempty"`
	PairedChatID string `json:"paired_chat_id,omitempty"`
	RuntimeState string `json:"runtime_state,omitempty"`
	Port         int    `json:"port,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type managedAgentInstanceFile struct {
	Instances []managedAgentInstance `json:"instances"`
}

var picoclawPairCodePattern = regexp.MustCompile(`\bpair-[a-f0-9]{32}\b`)

var picoclawChannels = []picoclawChannel{
	{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for PicoClaw",
	},
}

var openclawChannels = []picoclawChannel{
	{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for OpenClaw",
	},
}

var (
	carrierVersion   = "dev"
	carrierCommit    = "unknown"
	carrierBuildDate = ""

	carrierUserHomeDirFunc = os.UserHomeDir
	carrierCurrentUserFunc = user.Current
)

var zeroclawChannels = []picoclawChannel{
	{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for ZeroClaw",
	},
}

var managedAgents = map[string]managedAgentConfig{
	"picoclaw": {
		ID:        "picoclaw",
		Name:      "PicoClaw",
		ConfigDir: ".picoclaw",
	},
	"openclaw": {
		ID:             "openclaw",
		Name:           "OpenClaw",
		ConfigDir:      ".openclaw",
		RequiredEnvKey: "OPENAI_API_KEY",
	},
	"zeroclaw": {
		ID:                  "zeroclaw",
		Name:                "ZeroClaw",
		ConfigDir:           ".zeroclaw",
		OptionalPopulateKey: "ZEROCLAW_API_KEY",
	},
}

var channelOptions = buildChannelOptionsFromCatalog()

var onboardChannelOptions = channelOptions

var providerOptions = buildProviderOptionsFromCatalog()

func buildChannelOptionsFromCatalog() []choiceOption {
	specs := catalog.ListChannels()
	out := make([]choiceOption, 0, len(specs))
	for _, spec := range specs {
		out = append(out, choiceOption{
			ID:           spec.ID,
			Name:         spec.Name,
			Setup:        spec.Setup,
			TokenEnv:     spec.TokenEnv,
			RequireToken: spec.RequireToken,
			SecretEnv:    spec.SecretEnv,
		})
	}
	return out
}

func buildProviderOptionsFromCatalog() []choiceOption {
	specs := catalog.ListProviders()
	out := make([]choiceOption, 0, len(specs))
	for _, spec := range specs {
		out = append(out, choiceOption{
			ID:           spec.ID,
			Name:         spec.Name,
			Setup:        spec.Setup,
			AuthMode:     spec.AuthMode,
			ProviderEnv:  spec.EnvVar,
			ExampleModel: spec.ExampleModel,
			Aliases:      catalog.ProviderAliasesFor(spec.ID),
		})
	}
	return out
}

const (
	openAICodexAuthBaseURL        = "https://auth.openai.com"
	openAICodexClientID           = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexVerificationURL    = openAICodexAuthBaseURL + "/codex/device"
	openAICodexDeviceAuthTimeout  = 15 * time.Minute
	openAICodexDefaultPollSeconds = 5
	daemonBootTimeout             = 45 * time.Second
	gatewayBootTimeout            = 10 * time.Second
	daemonBootPollInterval        = 250 * time.Millisecond
	defaultPairCodeTTLSeconds     = 300
	defaultUpdateTimeout          = 120 * time.Second
	defaultOrchestrateWaitTimeout = 30 * time.Minute
)

var runOpenAICodexDeviceCodeFlow = performOpenAICodexDeviceCodeFlow
var gatewayHealthProbe = checkGatewayHealth
var daemonHealthProbe = checkDaemonHealth
var doctorAgentStatusesFetcher = fetchDoctorAgentStatuses
var portAvailableProbe = isPortAvailable
var remoteHostsLister = listRemoteHosts
var remoteInstallStreamer = runRemoteInstallStream
var remoteHostChecker = runRemoteHostCheck
var daemonBackgroundStarter = startDaemonInBackground
var gatewayBackgroundStarter = startGatewayInBackground
var runStopFlow = runStop
var carrierGOOS = runtime.GOOS
var windowsWSLListDistros = listWindowsWSLDistros
var writeBootstrapPIDFileFunc = writeBootstrapPIDFile
var terminateBackgroundProcessFunc = terminateBackgroundProcess
var daemonPairCodeFetcher = fetchDaemonPairCode
var openBrowserFunc = openBrowserURL
var runOnboardFlow = runOnboard
var ensureWebUIServicesFlow = ensureWebUIServices
var execGitCommand = func(ctx context.Context, workingDir string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workingDir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(raw)))
	}
	return strings.TrimSpace(string(raw)), nil
}
var procFSRoot = "/proc"
var daemonActionLogPollInterval = 2 * time.Second
var daemonActionHeartbeatInterval = 15 * time.Second
var orchestratePollInterval = 2 * time.Second

const usage = `Carrier — unified agent platform binary

Usage:
  carrier                Bootstrap Carrier (onboard if needed, keep daemon+gateway running, then exit)
  carrier version        Print version metadata
  carrier --version      Print version metadata
  carrier -v             Print version metadata
  carrier -V             Print version metadata
  carrier daemon         Start daemon HTTP API server (foreground)
  carrier update         Update to a newer git ref
  carrier update --check  Show current and target without applying changes
  Common update options:
    --check, --yes, --dry-run, --force, --channel <stable|beta|dev>, --tag <dist-tag|version>, --timeout <seconds>, --json, --no-restart
  carrier gateway        Start gateway HTTP server
  carrier stop           Stop background daemon and gateway
  carrier reset          Stop Carrier services and remove local Carrier-generated data
  carrier stop <id|name> Stop a managed agent instance
  carrier start <id|name> Start a managed agent instance
  carrier status <id|name> Show status for a managed agent instance
  carrier upgrade <id|name> Upgrade a managed agent instance
  carrier uninstall <id|name> Uninstall and remove a managed agent instance
  carrier list           List managed agent instances
  carrier onboard [--tui|--cli]
                        Interactive terminal onboarding (channel/provider -> keep gateway running in background)
  carrier onboard --webui
                        Launch WebUI onboarding (start/reuse daemon+gateway)
  carrier add <agent_id> [--isolation|--no-isolation] [--tui|--cli|--webui] [-q|--quiet]
                        Add/install an agent (default: terminal flow; use -q for quiet mode)
                        managed agents default to isolation unless --no-isolation is set
  carrier install <agent_id> [--isolation|--no-isolation] [--tui|--cli|--webui] [-q|--quiet]
                        Alias for carrier add <agent_id>
  carrier logs <id|name> [--since <rfc3339|unix>] [--level <level>] [--grep <text>] [--export <path>]
                        Query and optionally export agent logs
  carrier service <install|start|stop|status|uninstall>
                        Manage Carrier Windows service registration
  carrier catalog add --manifest <path>
  carrier catalog list
  carrier catalog remove <id>
                        Manage custom agent catalog manifests
  carrier webhooks test
                        Send a test webhook callback using CARRIER_WEBHOOK_URL
  carrier config set <agent_id> key=value
                        Update agent config and apply hot-reload/restart
  carrier doctor [--json]
                        Run local environment and daemon health checks
  carrier keys generate [--name <alias>]
                        Generate managed SSH key (default alias: default)
  carrier keys list     List managed SSH keys with fingerprints
  carrier keys delete <alias>
                        Delete a managed SSH key
  carrier remote add <agent_id> --host-id <id> --host <ip-or-domain> --port <port> --user <ssh-user> --key-path <private-key-path> [options]
                        Deterministic remote install workflow via gateway API
                        agent_id: openclaw | picoclaw | zeroclaw
                        options:
                          [--name <display-name>]
                          [--auth-mode <private_key|ssh_config>]
                          [--ssh-config-host <alias>]
                          [--key-ref <uploaded-key-ref>]
                          [--runtime-mode <on_demand|managed_gateway>]
                          [--isolation|--no-isolation]
                          (default: isolation enabled)
                          [--sync-channel <telegram|discord|feishu>]...
                          [--sync-provider <provider-id>]...
                          [--telegram-allow-from <id>]...
                          [--discord-allow-from <id>]...
                          [--check-retries <n>] [--check-retry-delay <seconds>]
                          [--no-auto-rollback]
                          [--skip-reconnect-check]
  carrier remote status <host_id> <agent_id>
                        Show remote instance status via gateway API
  carrier remote logs <host_id> <agent_id> [--tail <n>]
                        Fetch remote instance logs via gateway API
  carrier remote rollback <host_id> <agent_id> [--commit <sha>]
                        Roll back remote instance config sync state
  carrier remote uninstall <host_id> <agent_id>
                        Uninstall remote instance artifacts (best-effort)
  carrier remote key import --file <pem-path>
                        Upload SSH private key to Carrier key store
  carrier remote key generate [--type <ed25519|rsa>] [--output <private-key-path>]
                        Generate SSH keypair locally, then upload private key
  carrier config backup [--output <path>]
                        Backup local Carrier config.v2.json
  carrier config restore --from <path>
                        Restore local Carrier config.v2.json
  carrier remote-store backup [--output <path>]
                        Backup remote-control.json store
  carrier remote-store restore --from <path>
                        Restore remote-control.json store
  carrier orchestrate <goal...> [--host-id <id>]... [--provider <provider-id>]
                        [--max-concurrency <n>] [--idempotency-key <key>]
                        [--timeout <duration>] [--async] [--dry-run] [--json]
                        Decompose goal with base agent, then run orchestration
  carrier orchestrate status <execution_id> [--json]
                        Show orchestration execution status/results
  carrier orchestrate cancel <execution_id> [--json]
                        Cancel orchestration execution
  carrier executions [list] [--limit <n>] [--json]
                        List orchestration executions
  carrier executions show <execution_id> [--json]
                        Show orchestration execution status/results
  carrier executions cancel <execution_id> [--json]
                        Cancel orchestration execution
  carrier templates [list] [--json]
                        List built-in execution templates
  carrier templates show <template_id> [--json]
                        Show one execution template
  carrier templates run <template_id> --input key=value [--input key=value]...
                        [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>]
                        [--max-concurrency <n>] [--policy-approve] [--json]
                        Launch a built-in execution template through the gateway
  carrier triggers [list] [--json]
                        List execution triggers
  carrier triggers show <trigger_id> [--json]
                        Show one execution trigger
  carrier triggers create --type <webhook|github|schedule> --template-id <template_id>
                        [--name <name>] [--host-id <id>]... [--host-label <label>]...
                        [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve]
                        [--webhook-secret <secret>] [--github-command <cmd>] [--github-label <label>]
                        [--github-repository <owner/repo>] [--cron <expr>] [--timezone UTC]
                        [--input key=value]... [--json]
                        Create one execution trigger
  carrier triggers update <trigger_id> [--name <name>] [--template-id <template_id>]
                        [--enable|--disable] [--host-id <id>]... [--host-label <label>]...
                        [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve]
                        [--webhook-secret <secret>] [--github-command <cmd>] [--github-label <label>]
                        [--github-repository <owner/repo>] [--cron <expr>] [--timezone UTC]
                        [--input key=value]... [--json]
                        Update one execution trigger
  carrier triggers delete <trigger_id> [--json]
                        Delete one execution trigger
  carrier --help         Show this help message

Notes:
  - daemon API defaults to http://127.0.0.1:9090
  - gateway API defaults to http://127.0.0.1:8787
  - onboarding asks channel credentials and provider auth setup
  - onboarding config path:
      $CARRIER_CONFIG or ~/.carrier/config.v2.json
`

func main() {
	if len(os.Args) > 1 {
		command, commandArgs, parseErr := parseCarrierCommand(os.Args)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n\n", parseErr)
			fmt.Fprint(os.Stderr, usage)
			os.Exit(1)
		}
		switch command {
		case "daemon", "--daemon":
			applyConfigV2Env(os.Stderr)
			server.Run()
			return
		case "gateway", "--gateway":
			applyConfigV2Env(os.Stderr)
			if err := gatewayruntime.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "gateway error: %v\n", err)
				os.Exit(1)
			}
			return
		case "stop":
			if len(commandArgs) >= 1 {
				if err := runStopInstance(os.Stdout, commandArgs[0]); err != nil {
					fmt.Fprintf(os.Stderr, "stop failed: %v\n", err)
					os.Exit(1)
				}
				return
			}
			if err := runStop(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "stop failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "reset":
			if err := runReset(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "reset failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "start":
			if len(commandArgs) < 1 {
				fmt.Fprintln(os.Stderr, "start failed: instance id or name is required")
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runStartInstance(os.Stdout, commandArgs[0]); err != nil {
				fmt.Fprintf(os.Stderr, "start failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "status":
			opts, err := parseStatusCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "status failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runStatusInstanceWithOptions(os.Stdout, opts.Target, opts.Metrics); err != nil {
				fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "upgrade":
			if len(commandArgs) < 1 {
				fmt.Fprintln(os.Stderr, "upgrade failed: instance id or name is required")
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runUpgradeInstance(os.Stdout, commandArgs[0]); err != nil {
				fmt.Fprintf(os.Stderr, "upgrade failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "uninstall":
			if len(commandArgs) < 1 {
				fmt.Fprintln(os.Stderr, "uninstall failed: instance id or name is required")
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runUninstallInstance(os.Stdout, commandArgs[0]); err != nil {
				fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "list":
			if err := runListInstances(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "list failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "doctor":
			opts, err := parseDoctorCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "doctor failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runDoctor(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "doctor failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "logs":
			opts, err := parseLogsCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "logs failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runLogsCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "logs failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "service":
			opts, err := parseServiceCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "service failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runServiceCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "service failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "catalog":
			opts, err := parseCatalogCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "catalog failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runCatalogCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "catalog failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "webhooks":
			opts, err := parseWebhooksCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "webhooks failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runWebhooksCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "webhooks failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "config":
			opts, err := parseConfigCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "config failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runConfigCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "config failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "keys":
			opts, err := parseKeysCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "keys failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			switch opts.Action {
			case "generate":
				if err := runKeysGenerate(os.Stdout, opts.Name); err != nil {
					fmt.Fprintf(os.Stderr, "keys generate failed: %v\n", err)
					os.Exit(1)
				}
			case "list":
				if err := runKeysList(os.Stdout); err != nil {
					fmt.Fprintf(os.Stderr, "keys list failed: %v\n", err)
					os.Exit(1)
				}
			case "delete":
				if err := runKeysDelete(os.Stdout, opts.Name); err != nil {
					fmt.Fprintf(os.Stderr, "keys delete failed: %v\n", err)
					os.Exit(1)
				}
			default:
				fmt.Fprintf(os.Stderr, "keys failed: unsupported action %q\n", opts.Action)
				os.Exit(1)
			}
			return
		case "remote":
			opts, err := parseRemoteCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "remote failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runRemoteCommand(os.Stdin, os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "remote failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "remote-store":
			opts, err := parseRemoteStoreCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "remote-store failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runRemoteStoreCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "remote-store failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "orchestrate":
			opts, err := parseOrchestrateCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "orchestrate failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runOrchestrateCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "orchestrate failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "executions":
			opts, err := parseExecutionsCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "executions failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runOrchestrateCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "executions failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "templates":
			opts, err := parseTemplatesCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "templates failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runTemplatesCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "templates failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "triggers":
			opts, err := parseTriggersCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "triggers failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runTriggersCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "triggers failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "onboard":
			opts, err := parseOnboardCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "onboard failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if opts.WebUI {
				if err := runOnboardWebUI(os.Stdout); err != nil {
					fmt.Fprintf(os.Stderr, "onboard failed: %v\n", err)
					os.Exit(1)
				}
				return
			}
			if err := runOnboardFlow(os.Stdin, os.Stdout, startGatewayInBackgroundAndWait); err != nil {
				fmt.Fprintf(os.Stderr, "onboard failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "add", "install":
			opts, err := parseAddCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s failed: %v\n\n", command, err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if opts.WebUI {
				if err := runAddWebUI(os.Stdout, opts.AgentID, opts.Isolation); err != nil {
					fmt.Fprintf(os.Stderr, "%s failed: %v\n", command, err)
					os.Exit(1)
				}
				return
			}
			if err := runAddTUI(os.Stdin, os.Stdout, opts.AgentID, opts.Quiet, opts.Isolation); err != nil {
				fmt.Fprintf(os.Stderr, "%s failed: %v\n", command, err)
				os.Exit(1)
			}
			return
		case "version":
			opts, err := parseVersionCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "version failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runVersionCommand(os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "version failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "update":
			opts, err := parseUpdateCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "update failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runUpdate(os.Stdin, os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
				os.Exit(1)
			}
			return
		case "--help", "-h", "help":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", command)
			fmt.Fprint(os.Stderr, usage)
			os.Exit(1)
		}
	}

	if err := runBootstrap(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "carrier bootstrap failed: %v\n", err)
		os.Exit(1)
	}
}

func parseCarrierCommand(args []string) (string, []string, error) {
	if len(args) <= 1 {
		return "bootstrap", nil, nil
	}
	command := strings.ToLower(strings.TrimSpace(args[1]))
	if command == "" {
		return "", nil, errors.New("empty command")
	}
	switch command {
	case "daemon", "--daemon":
		return "daemon", args[2:], nil
	case "gateway", "--gateway":
		return "gateway", args[2:], nil
	case "stop":
		return "stop", args[2:], nil
	case "reset":
		return "reset", args[2:], nil
	case "start":
		return "start", args[2:], nil
	case "status":
		return "status", args[2:], nil
	case "upgrade":
		return "upgrade", args[2:], nil
	case "uninstall":
		return "uninstall", args[2:], nil
	case "list":
		return "list", nil, nil
	case "doctor":
		return "doctor", args[2:], nil
	case "logs":
		return "logs", args[2:], nil
	case "service":
		return "service", args[2:], nil
	case "catalog":
		return "catalog", args[2:], nil
	case "webhooks":
		return "webhooks", args[2:], nil
	case "config":
		return "config", args[2:], nil
	case "keys":
		return "keys", args[2:], nil
	case "onboard":
		return "onboard", args[2:], nil
	case "add":
		return "add", args[2:], nil
	case "install":
		return "install", args[2:], nil
	case "remote":
		return "remote", args[2:], nil
	case "remote-store":
		return "remote-store", args[2:], nil
	case "orchestrate":
		return "orchestrate", args[2:], nil
	case "executions":
		return "executions", args[2:], nil
	case "templates":
		return "templates", args[2:], nil
	case "triggers":
		return "triggers", args[2:], nil
	case "--help", "-h", "help":
		return "help", nil, nil
	case "version", "--version", "-v", "-V":
		return "version", args[2:], nil
	case "update":
		return "update", args[2:], nil
	default:
		return "", nil, fmt.Errorf("unknown command: %s", command)
	}
}

func parseAddCommandArgs(args []string) (addCommandOptions, error) {
	if len(args) == 0 {
		return addCommandOptions{}, errors.New("usage: carrier add <agent_id> [--isolation|--no-isolation] [--tui|--cli|--webui] [-q|--quiet] (alias: carrier install <agent_id>)")
	}

	opts := addCommandOptions{}
	isolationExplicit := false
	for _, raw := range args {
		arg := strings.ToLower(strings.TrimSpace(raw))
		switch arg {
		case "--webui", "--web", "webui":
			opts.WebUI = true
		case "--isolation":
			opts.Isolation = true
			isolationExplicit = true
		case "--no-isolation":
			opts.Isolation = false
			isolationExplicit = true
		case "--cli", "cli":
			opts.CLI = true
		case "--tui", "tui":
			opts.TUI = true
		case "-q", "--quiet", "--quite": // "--quite" is an intentional typo alias for common misspelling
			opts.Quiet = true
		case "":
			continue
		default:
			if strings.HasPrefix(arg, "-") {
				return addCommandOptions{}, fmt.Errorf("unknown add option: %s", raw)
			}
			if opts.AgentID != "" {
				return addCommandOptions{}, fmt.Errorf("multiple agent ids provided: %s and %s", opts.AgentID, raw)
			}
			opts.AgentID = arg
		}
	}
	if opts.AgentID == "" {
		return addCommandOptions{}, errors.New("agent_id is required")
	}
	terminalModeRequested := opts.CLI || opts.TUI
	if opts.WebUI && terminalModeRequested {
		return addCommandOptions{}, errors.New("cannot combine --webui with --cli/--tui")
	}
	if opts.CLI {
		// CLI mode is a terminal onboarding/install path implemented by TUI prompts.
		opts.TUI = true
	}
	if !opts.WebUI && !terminalModeRequested {
		opts.TUI = true
	}
	if !isolationExplicit && isManagedAgent(opts.AgentID) {
		opts.Isolation = true
	}
	return opts, nil
}

func parseOnboardCommandArgs(args []string) (onboardCommandOptions, error) {
	opts := onboardCommandOptions{}
	for _, raw := range args {
		arg := strings.ToLower(strings.TrimSpace(raw))
		switch arg {
		case "--webui", "--web", "webui":
			opts.WebUI = true
		case "--cli", "cli":
			opts.CLI = true
		case "--tui", "tui":
			opts.TUI = true
		case "":
			continue
		default:
			return onboardCommandOptions{}, fmt.Errorf("unknown onboard option: %s", raw)
		}
	}

	terminalModeRequested := opts.CLI || opts.TUI
	if opts.WebUI && terminalModeRequested {
		return onboardCommandOptions{}, errors.New("cannot combine --webui with --cli/--tui")
	}
	if opts.CLI {
		// CLI mode is a terminal onboarding path implemented by TUI prompts.
		opts.TUI = true
	}
	if !opts.WebUI && !terminalModeRequested {
		opts.TUI = true
	}
	return opts, nil
}

func parseStatusCommandArgs(args []string) (statusCommandOptions, error) {
	opts := statusCommandOptions{}
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		switch strings.ToLower(arg) {
		case "":
		case "--metrics":
			opts.Metrics = true
		default:
			if strings.HasPrefix(arg, "-") {
				return statusCommandOptions{}, fmt.Errorf("unknown status option: %s", raw)
			}
			if opts.Target != "" {
				return statusCommandOptions{}, errors.New("multiple instance targets provided")
			}
			opts.Target = arg
		}
	}
	if strings.TrimSpace(opts.Target) == "" {
		return statusCommandOptions{}, errors.New("instance id or name is required")
	}
	return opts, nil
}

func parseVersionCommandArgs(args []string) (versionCommandOptions, error) {
	opts := versionCommandOptions{}
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		switch arg {
		case "":
		case "--json":
			opts.JSON = true
		default:
			return versionCommandOptions{}, fmt.Errorf("unknown version option: %s", raw)
		}
	}
	return opts, nil
}

func parseDoctorCommandArgs(args []string) (doctorCommandOptions, error) {
	opts := doctorCommandOptions{}
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		switch arg {
		case "":
		case "--json":
			opts.JSON = true
		default:
			return doctorCommandOptions{}, fmt.Errorf("unknown doctor option: %s", raw)
		}
	}
	return opts, nil
}

func parseLogsCommandArgs(args []string) (logsCommandOptions, error) {
	if len(args) == 0 {
		return logsCommandOptions{}, errors.New("usage: carrier logs <id|name> [--since <rfc3339|unix>] [--level <level>] [--grep <text>] [--export <path>]")
	}
	opts := logsCommandOptions{}
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		switch strings.ToLower(raw) {
		case "":
		case "--since":
			value, next, err := parseRequiredFlagValue(args, i, "--since")
			if err != nil {
				return logsCommandOptions{}, err
			}
			if since, ok := parseSinceValue(value); ok {
				opts.Since = since
			} else {
				return logsCommandOptions{}, fmt.Errorf("invalid --since value: %s", value)
			}
			i = next
		case "--level":
			value, next, err := parseRequiredFlagValue(args, i, "--level")
			if err != nil {
				return logsCommandOptions{}, err
			}
			opts.Level = strings.ToUpper(strings.TrimSpace(value))
			i = next
		case "--grep":
			value, next, err := parseRequiredFlagValue(args, i, "--grep")
			if err != nil {
				return logsCommandOptions{}, err
			}
			opts.Grep = strings.TrimSpace(value)
			i = next
		case "--export":
			value, next, err := parseRequiredFlagValue(args, i, "--export")
			if err != nil {
				return logsCommandOptions{}, err
			}
			opts.Export = strings.TrimSpace(value)
			i = next
		default:
			if strings.HasPrefix(raw, "-") {
				return logsCommandOptions{}, fmt.Errorf("unknown logs option: %s", raw)
			}
			if opts.AgentID != "" {
				return logsCommandOptions{}, errors.New("multiple log targets provided")
			}
			opts.AgentID = raw
		}
	}
	if strings.TrimSpace(opts.AgentID) == "" {
		return logsCommandOptions{}, errors.New("agent id or instance name is required")
	}
	return opts, nil
}

func parseServiceCommandArgs(args []string) (serviceCommandOptions, error) {
	if len(args) != 1 {
		return serviceCommandOptions{}, errors.New("usage: carrier service <install|start|stop|status|uninstall>")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "install", "start", "stop", "status", "uninstall":
		return serviceCommandOptions{Action: action}, nil
	default:
		return serviceCommandOptions{}, fmt.Errorf("unsupported service action: %s", args[0])
	}
}

func parseCatalogCommandArgs(args []string) (catalogCommandOptions, error) {
	if len(args) == 0 {
		return catalogCommandOptions{}, errors.New("usage: carrier catalog <add|list|remove>")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "list":
		return catalogCommandOptions{Action: action}, nil
	case "remove":
		if len(args) != 2 {
			return catalogCommandOptions{}, errors.New("usage: carrier catalog remove <id>")
		}
		return catalogCommandOptions{Action: action, ID: strings.TrimSpace(args[1])}, nil
	case "add":
		opts := catalogCommandOptions{Action: action}
		for i := 1; i < len(args); i++ {
			switch strings.ToLower(strings.TrimSpace(args[i])) {
			case "--manifest":
				value, next, err := parseRequiredFlagValue(args, i, "--manifest")
				if err != nil {
					return catalogCommandOptions{}, err
				}
				opts.ManifestPath = strings.TrimSpace(value)
				i = next
			default:
				return catalogCommandOptions{}, fmt.Errorf("unknown catalog add option: %s", args[i])
			}
		}
		if opts.ManifestPath == "" {
			return catalogCommandOptions{}, errors.New("--manifest is required")
		}
		return opts, nil
	default:
		return catalogCommandOptions{}, fmt.Errorf("unsupported catalog action: %s", args[0])
	}
}

func parseWebhooksCommandArgs(args []string) (webhooksCommandOptions, error) {
	if len(args) != 1 || strings.ToLower(strings.TrimSpace(args[0])) != "test" {
		return webhooksCommandOptions{}, errors.New("usage: carrier webhooks test")
	}
	return webhooksCommandOptions{Action: "test"}, nil
}

func parseConfigCommandArgs(args []string) (configCommandOptions, error) {
	if len(args) == 0 {
		return configCommandOptions{}, errors.New("usage: carrier config <set|backup|restore> ...")
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "backup":
		opts := configCommandOptions{Action: "backup"}
		for i := 1; i < len(args); i++ {
			switch strings.ToLower(strings.TrimSpace(args[i])) {
			case "--output":
				value, next, err := parseRequiredFlagValue(args, i, "--output")
				if err != nil {
					return configCommandOptions{}, err
				}
				opts.OutputPath = strings.TrimSpace(value)
				i = next
			default:
				return configCommandOptions{}, fmt.Errorf("unknown config option: %s", args[i])
			}
		}
		return opts, nil
	case "restore":
		opts := configCommandOptions{Action: "restore"}
		for i := 1; i < len(args); i++ {
			switch strings.ToLower(strings.TrimSpace(args[i])) {
			case "--from":
				value, next, err := parseRequiredFlagValue(args, i, "--from")
				if err != nil {
					return configCommandOptions{}, err
				}
				opts.FromPath = strings.TrimSpace(value)
				i = next
			default:
				return configCommandOptions{}, fmt.Errorf("unknown config option: %s", args[i])
			}
		}
		if opts.FromPath == "" {
			return configCommandOptions{}, errors.New("--from is required for config restore")
		}
		return opts, nil
	case "set":
		if len(args) < 3 {
			return configCommandOptions{}, errors.New("usage: carrier config set <agent_id> key=value OR carrier config set tls.enabled true --domain <example.com>")
		}
		if strings.EqualFold(strings.TrimSpace(args[1]), "tls.enabled") {
			opts := configCommandOptions{Action: "set-tls", Key: "tls.enabled", Value: strings.TrimSpace(args[2])}
			for i := 3; i < len(args); i++ {
				flag := strings.ToLower(strings.TrimSpace(args[i]))
				switch flag {
				case "--domain":
					value, next, err := parseRequiredFlagValue(args, i, "--domain")
					if err != nil {
						return configCommandOptions{}, err
					}
					opts.Domain = strings.TrimSpace(value)
					i = next
				default:
					return configCommandOptions{}, fmt.Errorf("unknown config option: %s", args[i])
				}
			}
			return opts, nil
		}
		if len(args) != 3 {
			return configCommandOptions{}, errors.New("usage: carrier config set <agent_id> key=value")
		}
		agent := strings.TrimSpace(args[1])
		kv := strings.TrimSpace(args[2])
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return configCommandOptions{}, errors.New("key=value is required")
		}
		return configCommandOptions{
			Action: "set",
			Agent:  agent,
			Key:    strings.TrimSpace(parts[0]),
			Value:  strings.TrimSpace(parts[1]),
		}, nil
	default:
		return configCommandOptions{}, fmt.Errorf("unsupported config action: %s", args[0])
	}
}

func parseSinceValue(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	if sec, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.Unix(sec, 0).UTC(), true
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func runServiceCommand(out io.Writer, opts serviceCommandOptions) error {
	result, err := runPlatformServiceAction(opts.Action)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result) != "" {
		_, _ = fmt.Fprintln(out, result)
	}
	return nil
}

func runCatalogCommand(out io.Writer, opts catalogCommandOptions) error {
	switch opts.Action {
	case "add":
		return runCatalogAdd(out, opts.ManifestPath)
	case "list":
		return runCatalogList(out)
	case "remove":
		return runCatalogRemove(out, opts.ID)
	default:
		return fmt.Errorf("unsupported catalog action: %s", opts.Action)
	}
}

func runWebhooksCommand(out io.Writer, opts webhooksCommandOptions) error {
	if opts.Action != "test" {
		return fmt.Errorf("unsupported webhooks action: %s", opts.Action)
	}
	webhookURL := strings.TrimSpace(os.Getenv("CARRIER_WEBHOOK_URL"))
	if webhookURL == "" {
		return errors.New("CARRIER_WEBHOOK_URL is not set")
	}
	payload := map[string]interface{}{
		"type":      "agent.started",
		"agentId":   "test-agent",
		"details":   "carrier webhooks test",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	_, _ = fmt.Fprintln(out, "webhook test sent successfully")
	return nil
}

func runConfigCommand(out io.Writer, opts configCommandOptions) error {
	switch opts.Action {
	case "backup":
		configPath, err := configv2.DefaultPath()
		if err != nil {
			return err
		}
		output := strings.TrimSpace(opts.OutputPath)
		if output == "" {
			output, err = defaultBackupOutputPath("config.v2")
			if err != nil {
				return err
			}
		}
		if err := copyFileSecure(configPath, output, 0o600); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Config backup saved to %s\n", output)
		return nil
	case "restore":
		configPath, err := configv2.DefaultPath()
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(strings.TrimSpace(opts.FromPath))
		if err != nil {
			return fmt.Errorf("read backup file: %w", err)
		}
		if err := validateConfigBackup(raw); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}
		if err := backupIfExists(configPath); err != nil {
			return fmt.Errorf("backup existing config before restore: %w", err)
		}
		if err := os.WriteFile(configPath, raw, 0o600); err != nil {
			return fmt.Errorf("write restored config: %w", err)
		}
		_, _ = fmt.Fprintf(out, "Config restored from %s\n", strings.TrimSpace(opts.FromPath))
		return nil
	}
	if opts.Action == "set-tls" {
		return runConfigTLSCommand(out, opts)
	}
	if opts.Action != "set" {
		return fmt.Errorf("unsupported config action: %s", opts.Action)
	}
	changes := map[string]string{opts.Key: opts.Value}
	path := fmt.Sprintf("/api/v1/agents/%s/config", neturl.PathEscape(strings.TrimSpace(opts.Agent)))
	_, _, err := daemonRequestWithTimeout(http.MethodPost, path, map[string]interface{}{"changes": changes}, 30*time.Second)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "updated %s %s=%s\n", opts.Agent, opts.Key, opts.Value)
	return nil
}

func runConfigTLSCommand(out io.Writer, opts configCommandOptions) error {
	enabled := strings.EqualFold(strings.TrimSpace(opts.Value), "true") || strings.TrimSpace(opts.Value) == "1"
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".carrier", "tls")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "config.json")
	payload := map[string]interface{}{
		"enabled":  enabled,
		"domain":   strings.TrimSpace(opts.Domain),
		"autocert": enabled,
		"cert_dir": dir,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "updated gateway TLS config: enabled=%t domain=%s cert_dir=%s\n", enabled, strings.TrimSpace(opts.Domain), dir)
	return nil
}

func runCatalogAdd(out io.Writer, manifestPath string) error {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	id := parseManifestID(raw)
	if strings.TrimSpace(id) == "" {
		return errors.New("manifest id is required")
	}
	dir, err := carrierCustomCatalogDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	ext := filepath.Ext(manifestPath)
	if ext == "" {
		ext = ".toml"
	}
	dst := filepath.Join(dir, id+ext)
	if err := os.WriteFile(dst, raw, 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "registered custom manifest %s\n", id)
	return nil
}

func runCatalogList(out io.Writer) error {
	builtins := []string{"openclaw", "picoclaw", "zeroclaw"}
	seen := map[string]bool{}
	for _, id := range builtins {
		seen[id] = true
		_, _ = fmt.Fprintf(out, "%s\tbuiltin\n", id)
	}
	dir, err := carrierCustomCatalogDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		id := strings.TrimSpace(parseManifestID(raw))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		_, _ = fmt.Fprintf(out, "%s\tcustom\n", id)
	}
	return nil
}

func runCatalogRemove(out io.Writer, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("manifest id is required")
	}
	dir, err := carrierCustomCatalogDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == id || strings.HasPrefix(name, id+".") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	_, _ = fmt.Fprintf(out, "removed custom manifest %s\n", id)
	return nil
}

func parseManifestID(raw []byte) string {
	var asMap map[string]interface{}
	if json.Unmarshal(raw, &asMap) == nil {
		return strings.TrimSpace(anyToString(asMap["id"]))
	}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != "id" {
			continue
		}
		return strings.Trim(strings.TrimSpace(parts[1]), "\"'")
	}
	return ""
}

func carrierCustomCatalogDir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_CUSTOM_CATALOG_DIR")); custom != "" {
		return custom, nil
	}
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "catalog", "custom"), nil
}

func parseKeysCommandArgs(args []string) (keysCommandOptions, error) {
	if len(args) == 0 {
		return keysCommandOptions{}, errors.New("usage: carrier keys <generate|list|delete>")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "generate":
		opts := keysCommandOptions{Action: action, Name: "default"}
		for i := 1; i < len(args); i++ {
			flag := strings.ToLower(strings.TrimSpace(args[i]))
			switch flag {
			case "--name":
				value, next, err := parseRequiredFlagValue(args, i, "--name")
				if err != nil {
					return keysCommandOptions{}, err
				}
				opts.Name = strings.TrimSpace(value)
				i = next
			default:
				return keysCommandOptions{}, fmt.Errorf("unknown keys generate option: %s", args[i])
			}
		}
		if err := validateCarrierKeyAlias(opts.Name); err != nil {
			return keysCommandOptions{}, err
		}
		return opts, nil
	case "list":
		if len(args) > 1 {
			return keysCommandOptions{}, fmt.Errorf("usage: carrier keys list")
		}
		return keysCommandOptions{Action: action}, nil
	case "delete":
		if len(args) != 2 {
			return keysCommandOptions{}, errors.New("usage: carrier keys delete <alias>")
		}
		name := strings.TrimSpace(args[1])
		if err := validateCarrierKeyAlias(name); err != nil {
			return keysCommandOptions{}, err
		}
		return keysCommandOptions{Action: action, Name: name}, nil
	default:
		return keysCommandOptions{}, fmt.Errorf("unknown keys action: %s", args[0])
	}
}

func runLogsCommand(out io.Writer, opts logsCommandOptions) error {
	instances, _, err := loadManagedInstances()
	if err != nil {
		return err
	}
	inst, _, err := resolveManagedInstanceTarget(instances, opts.AgentID)
	if err != nil {
		return err
	}
	lines, err := daemonFetchAgentLogs(inst.AgentID, 1000)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if !opts.Since.IsZero() {
			ts, ok := extractLogTimestamp(line)
			if ok && ts.Before(opts.Since) {
				continue
			}
		}
		if lvl := strings.TrimSpace(opts.Level); lvl != "" {
			if !strings.Contains(strings.ToUpper(line), strings.ToUpper("["+lvl+"]")) && !strings.Contains(strings.ToUpper(line), " "+lvl+" ") {
				continue
			}
		}
		if grep := strings.TrimSpace(opts.Grep); grep != "" {
			if !strings.Contains(strings.ToLower(line), strings.ToLower(grep)) {
				continue
			}
		}
		filtered = append(filtered, line)
	}
	output := strings.Join(filtered, "\n")
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	if export := strings.TrimSpace(opts.Export); export != "" {
		if err := os.WriteFile(export, []byte(output), 0o600); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "exported %d log lines to %s\n", len(filtered), export)
		return nil
	}
	_, _ = io.WriteString(out, output)
	return nil
}

func extractLogTimestamp(line string) (time.Time, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return time.Time{}, false
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, parts[0]); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

type doctorCheckResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Details    string `json:"details,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

func runDoctor(out io.Writer, opts doctorCommandOptions) error {
	results := []doctorCheckResult{
		doctorCheckDaemonReachable(),
		doctorCheckConfig(),
		doctorCheckDiskSpace(),
		doctorCheckRunningAgents(),
	}
	if opts.JSON {
		raw, err := json.MarshalIndent(map[string]interface{}{
			"checks": results,
		}, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(raw))
		return nil
	}
	for _, check := range results {
		symbol := "✅"
		switch strings.ToLower(strings.TrimSpace(check.Status)) {
		case "warn":
			symbol = "⚠️"
		case "fail":
			symbol = "❌"
		}
		_, _ = fmt.Fprintf(out, "%s %s: %s\n", symbol, check.Name, check.Details)
		if suggestion := strings.TrimSpace(check.Suggestion); suggestion != "" {
			_, _ = fmt.Fprintf(out, "   suggestion: %s\n", suggestion)
		}
	}
	return nil
}

func doctorCheckDaemonReachable() doctorCheckResult {
	base := daemonProbeBaseURL()
	if daemonHealthProbe(base) {
		return doctorCheckResult{Name: "Daemon reachable", Status: "ok", Details: "daemon is healthy"}
	}
	return doctorCheckResult{
		Name:       "Daemon reachable",
		Status:     "fail",
		Details:    "daemon health check failed",
		Suggestion: "start daemon with `carrier daemon` or run `carrier` bootstrap",
	}
}

func doctorCheckConfig() doctorCheckResult {
	cfg, path, err := configv2.Load()
	if err != nil || cfg == nil {
		return doctorCheckResult{
			Name:       "Config valid",
			Status:     "fail",
			Details:    "config is missing or invalid",
			Suggestion: "run `carrier onboard` to generate a valid config",
		}
	}
	return doctorCheckResult{Name: "Config valid", Status: "ok", Details: fmt.Sprintf("loaded %s", path)}
}

func doctorCheckDiskSpace() doctorCheckResult {
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return doctorCheckResult{Name: "Disk space", Status: "warn", Details: "unable to resolve home directory", Suggestion: "set HOME and retry"}
	}
	free, err := statfsFreeBytes(home)
	if err != nil {
		return doctorCheckResult{Name: "Disk space", Status: "warn", Details: "unable to determine free disk space", Suggestion: "check free space manually"}
	}
	const minFreeBytes = 500 * 1024 * 1024
	if free < minFreeBytes {
		return doctorCheckResult{Name: "Disk space", Status: "fail", Details: fmt.Sprintf("free disk space is %d bytes", free), Suggestion: "free at least 500MB and retry"}
	}
	return doctorCheckResult{Name: "Disk space", Status: "ok", Details: fmt.Sprintf("free disk space is %d bytes", free)}
}

func doctorCheckRunningAgents() doctorCheckResult {
	statuses, err := doctorAgentStatusesFetcher()
	if err != nil {
		return doctorCheckResult{
			Name:       "Running agents healthy",
			Status:     "warn",
			Details:    "unable to query agent statuses",
			Suggestion: "ensure daemon is running and authenticated",
		}
	}
	unhealthy := make([]string, 0)
	for _, st := range statuses {
		if !strings.EqualFold(strings.TrimSpace(st.Runtime), "running") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(st.Health), "healthy") {
			continue
		}
		unhealthy = append(unhealthy, strings.TrimSpace(st.ID))
	}
	if len(unhealthy) > 0 {
		return doctorCheckResult{
			Name:       "Running agents healthy",
			Status:     "fail",
			Details:    fmt.Sprintf("unhealthy agents: %s", strings.Join(unhealthy, ", ")),
			Suggestion: "run `carrier status <id>` and inspect logs",
		}
	}
	return doctorCheckResult{Name: "Running agents healthy", Status: "ok", Details: "all running agents are healthy"}
}

type doctorAgentStatus struct {
	ID      string
	Runtime string
	Health  string
}

func fetchDoctorAgentStatuses() ([]doctorAgentStatus, error) {
	raw, status, err := daemonRequestWithTimeout(http.MethodGet, "/api/v1/agents/status", nil, 3*time.Second)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("status request failed with status %d", status)
	}
	var payload struct {
		Statuses []struct {
			ID      string `json:"id"`
			Runtime string `json:"runtimeState"`
			Health  string `json:"health"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	out := make([]doctorAgentStatus, 0, len(payload.Statuses))
	for _, st := range payload.Statuses {
		out = append(out, doctorAgentStatus{ID: st.ID, Runtime: st.Runtime, Health: st.Health})
	}
	return out, nil
}

func parseRemoteCommandArgs(args []string) (remoteCommandOptions, error) {
	if len(args) < 1 {
		return remoteCommandOptions{}, errors.New("usage: carrier remote <add|install|status|logs|rollback|uninstall|key> ...")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	opts := remoteCommandOptions{
		Action:             action,
		Port:               22,
		AuthMode:           "private_key",
		RuntimeMode:        "on_demand",
		CheckRetries:       10,
		CheckRetryDelaySec: 2,
		AutoRollback:       true,
		Tail:               200,
		SyncChannels:       []string{},
		SyncProviders:      []string{},
		TelegramAllowFrom:  []string{},
		DiscordAllowFrom:   []string{},
		Concurrency:        5,
	}
	isolationExplicit := false
	switch action {
	case "add":
		if len(args) < 2 {
			return remoteCommandOptions{}, errors.New("usage: carrier remote add <agent_id> --host-id <id> --host <ip-or-domain> --port <port> --user <ssh-user> --key-path <private-key-path> [options]")
		}
		opts.AgentID = strings.ToLower(strings.TrimSpace(args[1]))
	case "install":
		if len(args) < 2 {
			return remoteCommandOptions{}, errors.New("usage: carrier remote install --all <agent_id> [--concurrency <n>]")
		}
		opts.AgentID = strings.ToLower(strings.TrimSpace(args[len(args)-1]))
		opts.All = true
	case "status":
		// Status supports both "carrier remote status --all" and
		// "carrier remote status <host_id> <agent_id>".
		if len(args) == 1 || strings.HasPrefix(strings.TrimSpace(args[1]), "-") {
			opts.All = true
		}
	case "logs", "rollback", "uninstall":
		if len(args) < 3 {
			return remoteCommandOptions{}, fmt.Errorf("usage: carrier remote %s <host_id> <agent_id>", opts.Action)
		}
		opts.HostID = strings.TrimSpace(args[1])
		opts.TargetAgentID = strings.ToLower(strings.TrimSpace(args[2]))
		opts.AgentID = opts.TargetAgentID
		opts.InstallAgentID = opts.TargetAgentID
	case "key":
		if len(args) < 2 {
			return remoteCommandOptions{}, errors.New("usage: carrier remote key <import|generate> [options]")
		}
		subAction := strings.ToLower(strings.TrimSpace(args[1]))
		switch subAction {
		case "import":
			opts.Action = "key-import"
		case "generate":
			opts.Action = "key-generate"
			opts.KeyType = "ed25519"
		default:
			return remoteCommandOptions{}, fmt.Errorf("unsupported remote key action: %s", subAction)
		}
	default:
		return remoteCommandOptions{}, fmt.Errorf("unsupported remote action: %s", opts.Action)
	}
	switch opts.AgentID {
	case "openclaw", "picoclaw", "zeroclaw":
	default:
		if opts.Action == "status" ||
			opts.Action == "logs" ||
			opts.Action == "rollback" ||
			opts.Action == "uninstall" ||
			opts.Action == "key-import" ||
			opts.Action == "key-generate" {
			// status --all does not require agent ID
		} else {
			return remoteCommandOptions{}, fmt.Errorf("unsupported remote agent_id: %s (expected one of openclaw, picoclaw, zeroclaw)", opts.AgentID)
		}
	}
	// OpenClaw runtime install endpoint uses the default agent slot "main".
	if opts.AgentID == "openclaw" {
		opts.InstallAgentID = "main"
	} else {
		opts.InstallAgentID = opts.AgentID
	}

	startIndex := 1
	switch opts.Action {
	case "add":
		startIndex = 2
	case "status", "logs", "rollback", "uninstall":
		if !opts.All {
			startIndex = 3
		}
	case "key-import", "key-generate":
		startIndex = 2
	}
	for i := startIndex; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		if raw == "" {
			continue
		}
		switch strings.ToLower(raw) {
		case "--all":
			opts.All = true
		case "--concurrency":
			value, next, err := parseRequiredFlagValue(args, i, "--concurrency")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			n, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || n <= 0 {
				return remoteCommandOptions{}, fmt.Errorf("invalid --concurrency value: %s", value)
			}
			opts.Concurrency = n
			i = next
		case "--host-id":
			value, next, err := parseRequiredFlagValue(args, i, "--host-id")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.HostID = strings.TrimSpace(value)
			i = next
		case "--name":
			value, next, err := parseRequiredFlagValue(args, i, "--name")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.HostName = strings.TrimSpace(value)
			i = next
		case "--host":
			value, next, err := parseRequiredFlagValue(args, i, "--host")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.HostAddr = strings.TrimSpace(value)
			i = next
		case "--port":
			value, next, err := parseRequiredFlagValue(args, i, "--port")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			port, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || port < 1 || port > 65535 {
				return remoteCommandOptions{}, fmt.Errorf("invalid --port value: %s", value)
			}
			opts.Port = port
			i = next
		case "--user":
			value, next, err := parseRequiredFlagValue(args, i, "--user")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.User = strings.TrimSpace(value)
			i = next
		case "--key-path":
			value, next, err := parseRequiredFlagValue(args, i, "--key-path")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.KeyPath = strings.TrimSpace(value)
			i = next
		case "--key-ref":
			value, next, err := parseRequiredFlagValue(args, i, "--key-ref")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.KeyRef = strings.TrimSpace(value)
			i = next
		case "--ssh-config-host":
			value, next, err := parseRequiredFlagValue(args, i, "--ssh-config-host")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.SSHConfigHost = strings.TrimSpace(value)
			i = next
		case "--auth-mode":
			value, next, err := parseRequiredFlagValue(args, i, "--auth-mode")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.AuthMode = strings.ToLower(strings.TrimSpace(value))
			i = next
		case "--runtime-mode":
			value, next, err := parseRequiredFlagValue(args, i, "--runtime-mode")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.RuntimeMode = strings.ToLower(strings.TrimSpace(value))
			i = next
		case "--isolation":
			opts.Isolation = true
			isolationExplicit = true
		case "--no-isolation":
			opts.Isolation = false
			isolationExplicit = true
		case "--sync-channel":
			value, next, err := parseRequiredFlagValue(args, i, "--sync-channel")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.SyncChannels = append(opts.SyncChannels, strings.ToLower(strings.TrimSpace(value)))
			i = next
		case "--sync-provider":
			value, next, err := parseRequiredFlagValue(args, i, "--sync-provider")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.SyncProviders = append(opts.SyncProviders, strings.ToLower(strings.TrimSpace(value)))
			i = next
		case "--telegram-allow-from":
			value, next, err := parseRequiredFlagValue(args, i, "--telegram-allow-from")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.TelegramAllowFrom = append(opts.TelegramAllowFrom, strings.TrimSpace(value))
			i = next
		case "--discord-allow-from":
			value, next, err := parseRequiredFlagValue(args, i, "--discord-allow-from")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.DiscordAllowFrom = append(opts.DiscordAllowFrom, strings.TrimSpace(value))
			i = next
		case "--check-retries":
			value, next, err := parseRequiredFlagValue(args, i, "--check-retries")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			retries, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || retries < 0 {
				return remoteCommandOptions{}, fmt.Errorf("invalid --check-retries value: %s", value)
			}
			opts.CheckRetries = retries
			i = next
		case "--check-retry-delay":
			value, next, err := parseRequiredFlagValue(args, i, "--check-retry-delay")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			delay, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || delay < 0 {
				return remoteCommandOptions{}, fmt.Errorf("invalid --check-retry-delay value: %s", value)
			}
			opts.CheckRetryDelaySec = delay
			i = next
		case "--skip-reconnect-check":
			opts.SkipReconnectCheck = true
		case "--no-auto-rollback":
			opts.AutoRollback = false
		case "--tail":
			if opts.Action != "logs" {
				return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
			}
			value, next, err := parseRequiredFlagValue(args, i, "--tail")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			tail, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || tail <= 0 {
				return remoteCommandOptions{}, fmt.Errorf("invalid --tail value: %s", value)
			}
			opts.Tail = tail
			i = next
		case "--commit":
			if opts.Action != "rollback" {
				return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
			}
			value, next, err := parseRequiredFlagValue(args, i, "--commit")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.Commit = strings.TrimSpace(value)
			i = next
		case "--file":
			if opts.Action != "key-import" {
				return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
			}
			value, next, err := parseRequiredFlagValue(args, i, "--file")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.KeyImportPath = strings.TrimSpace(value)
			i = next
		case "--type":
			if opts.Action != "key-generate" {
				return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
			}
			value, next, err := parseRequiredFlagValue(args, i, "--type")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.KeyType = strings.ToLower(strings.TrimSpace(value))
			i = next
		case "--output":
			if opts.Action != "key-generate" {
				return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
			}
			value, next, err := parseRequiredFlagValue(args, i, "--output")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.KeyOutputPath = strings.TrimSpace(value)
			i = next
		default:
			if opts.Action == "install" && strings.EqualFold(raw, opts.AgentID) {
				continue
			}
			if (opts.Action == "status" || opts.Action == "logs" || opts.Action == "rollback" || opts.Action == "uninstall") && !opts.All && (i == 1 || i == 2) {
				continue
			}
			if opts.Action == "status" {
				return remoteCommandOptions{}, fmt.Errorf("unknown remote status option: %s", raw)
			}
			return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
		}
	}
	if !isolationExplicit && (opts.Action == "add" || opts.Action == "install") {
		switch opts.AgentID {
		case "openclaw", "picoclaw", "zeroclaw":
			opts.Isolation = true
		}
	}
	if opts.Action == "install" {
		if !opts.All {
			return remoteCommandOptions{}, errors.New("remote install currently requires --all")
		}
		return opts, nil
	}
	if opts.Action == "status" {
		if opts.All {
			return opts, nil
		}
	}

	if opts.Action == "logs" || opts.Action == "rollback" || opts.Action == "uninstall" {
		if opts.HostID == "" {
			return remoteCommandOptions{}, errors.New("host_id is required")
		}
		if opts.TargetAgentID == "" {
			return remoteCommandOptions{}, errors.New("agent_id is required")
		}
		return opts, nil
	}

	if opts.Action == "key-import" {
		if opts.KeyImportPath == "" {
			return remoteCommandOptions{}, errors.New("--file is required for remote key import")
		}
		return opts, nil
	}
	if opts.Action == "key-generate" {
		switch opts.KeyType {
		case "ed25519", "rsa":
		default:
			return remoteCommandOptions{}, fmt.Errorf("invalid --type value: %s", opts.KeyType)
		}
		return opts, nil
	}

	if opts.HostID == "" {
		return remoteCommandOptions{}, errors.New("--host-id is required")
	}
	if opts.HostName == "" {
		opts.HostName = opts.HostID
	}
	switch opts.AuthMode {
	case "private_key":
		if opts.HostAddr == "" {
			return remoteCommandOptions{}, errors.New("--host is required")
		}
		if opts.User == "" {
			return remoteCommandOptions{}, errors.New("--user is required")
		}
		if opts.KeyPath == "" && opts.KeyRef == "" {
			return remoteCommandOptions{}, errors.New("--key-path or --key-ref is required")
		}
	case "ssh_config":
		if opts.SSHConfigHost == "" && opts.HostAddr == "" {
			return remoteCommandOptions{}, errors.New("--ssh-config-host or --host is required when --auth-mode ssh_config")
		}
	default:
		return remoteCommandOptions{}, fmt.Errorf("invalid --auth-mode value: %s", opts.AuthMode)
	}
	switch opts.RuntimeMode {
	case "on_demand", "managed_gateway":
	default:
		return remoteCommandOptions{}, fmt.Errorf("invalid --runtime-mode value: %s", opts.RuntimeMode)
	}
	for _, channel := range opts.SyncChannels {
		if !catalog.IsSupportedChannel(channel) {
			return remoteCommandOptions{}, fmt.Errorf("invalid --sync-channel value: %s", channel)
		}
	}
	for _, provider := range opts.SyncProviders {
		trimmed := strings.TrimSpace(provider)
		if trimmed == "" {
			return remoteCommandOptions{}, errors.New("--sync-provider cannot be empty")
		}
		if !catalog.IsSupportedProvider(trimmed) {
			return remoteCommandOptions{}, fmt.Errorf("invalid --sync-provider value: %s", trimmed)
		}
	}
	return opts, nil
}

func parseRemoteStoreCommandArgs(args []string) (remoteStoreCommandOptions, error) {
	if len(args) == 0 {
		return remoteStoreCommandOptions{}, errors.New("usage: carrier remote-store <backup|restore> [--output <path>] [--from <path>]")
	}
	opts := remoteStoreCommandOptions{Action: strings.ToLower(strings.TrimSpace(args[0]))}
	switch opts.Action {
	case "backup":
		for i := 1; i < len(args); i++ {
			switch strings.ToLower(strings.TrimSpace(args[i])) {
			case "--output":
				value, next, err := parseRequiredFlagValue(args, i, "--output")
				if err != nil {
					return remoteStoreCommandOptions{}, err
				}
				opts.OutputPath = strings.TrimSpace(value)
				i = next
			default:
				return remoteStoreCommandOptions{}, fmt.Errorf("unknown remote-store option: %s", args[i])
			}
		}
	case "restore":
		for i := 1; i < len(args); i++ {
			switch strings.ToLower(strings.TrimSpace(args[i])) {
			case "--from":
				value, next, err := parseRequiredFlagValue(args, i, "--from")
				if err != nil {
					return remoteStoreCommandOptions{}, err
				}
				opts.FromPath = strings.TrimSpace(value)
				i = next
			default:
				return remoteStoreCommandOptions{}, fmt.Errorf("unknown remote-store option: %s", args[i])
			}
		}
		if opts.FromPath == "" {
			return remoteStoreCommandOptions{}, errors.New("--from is required for remote-store restore")
		}
	default:
		return remoteStoreCommandOptions{}, fmt.Errorf("unsupported remote-store action: %s", opts.Action)
	}
	return opts, nil
}

func parseOrchestrateCommandArgs(args []string) (orchestrateCommandOptions, error) {
	if len(args) == 0 {
		return orchestrateCommandOptions{}, errors.New("usage: carrier orchestrate <goal...> [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve] [--idempotency-key <key>] [--timeout <duration>] [--async] [--dry-run] [--json] OR carrier orchestrate <status|cancel|authorize> <execution_id> [--policy-approve] [--json]")
	}

	opts := orchestrateCommandOptions{
		Action:  "run",
		Timeout: defaultOrchestrateWaitTimeout,
	}

	firstArg := strings.ToLower(strings.TrimSpace(args[0]))
	if firstArg == "status" || firstArg == "cancel" || firstArg == "authorize" {
		opts.Action = firstArg
		for i := 1; i < len(args); i++ {
			raw := strings.TrimSpace(args[i])
			lower := strings.ToLower(raw)
			switch lower {
			case "":
			case "--json":
				opts.JSON = true
			case "--policy-approve":
				opts.PolicyApprove = true
			default:
				if strings.HasPrefix(raw, "-") {
					return orchestrateCommandOptions{}, fmt.Errorf("unknown orchestrate %s option: %s", opts.Action, raw)
				}
				if opts.ExecutionID != "" {
					return orchestrateCommandOptions{}, errors.New("multiple execution ids provided")
				}
				opts.ExecutionID = raw
			}
		}
		if strings.TrimSpace(opts.ExecutionID) == "" {
			return orchestrateCommandOptions{}, fmt.Errorf("usage: carrier orchestrate %s <execution_id> [--policy-approve] [--json]", opts.Action)
		}
		return opts, nil
	}

	goalParts := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch lower {
		case "":
		case "--host-id":
			value, next, err := parseRequiredFlagValue(args, i, "--host-id")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			hostID := strings.TrimSpace(value)
			if hostID == "" {
				return orchestrateCommandOptions{}, errors.New("--host-id cannot be empty")
			}
			opts.HostIDs = append(opts.HostIDs, hostID)
			i = next
		case "--host-label":
			value, next, err := parseRequiredFlagValue(args, i, "--host-label")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			hostLabel := strings.TrimSpace(value)
			if hostLabel == "" {
				return orchestrateCommandOptions{}, errors.New("--host-label cannot be empty")
			}
			opts.HostLabels = append(opts.HostLabels, hostLabel)
			i = next
		case "--provider":
			value, next, err := parseRequiredFlagValue(args, i, "--provider")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			opts.Provider = strings.TrimSpace(value)
			i = next
		case "--max-concurrency":
			value, next, err := parseRequiredFlagValue(args, i, "--max-concurrency")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			parsed, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || parsed <= 0 {
				return orchestrateCommandOptions{}, fmt.Errorf("invalid --max-concurrency value: %s", value)
			}
			opts.MaxConcurrency = parsed
			i = next
		case "--idempotency-key":
			value, next, err := parseRequiredFlagValue(args, i, "--idempotency-key")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			opts.IdempotencyKey = strings.TrimSpace(value)
			i = next
		case "--policy-approve":
			opts.PolicyApprove = true
		case "--timeout":
			value, next, err := parseRequiredFlagValue(args, i, "--timeout")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			timeout, convErr := time.ParseDuration(strings.TrimSpace(value))
			if convErr != nil || timeout <= 0 {
				return orchestrateCommandOptions{}, fmt.Errorf("invalid --timeout value: %s", value)
			}
			opts.Timeout = timeout
			i = next
		case "--async":
			opts.Async = true
		case "--dry-run":
			opts.Action = "plan"
		case "--json":
			opts.JSON = true
		default:
			if strings.HasPrefix(raw, "-") {
				return orchestrateCommandOptions{}, fmt.Errorf("unknown orchestrate option: %s", raw)
			}
			goalParts = append(goalParts, raw)
		}
	}

	opts.Provider = strings.ToLower(strings.TrimSpace(opts.Provider))
	opts.IdempotencyKey = strings.TrimSpace(opts.IdempotencyKey)
	opts.HostIDs = dedupeStringSlice(opts.HostIDs)
	opts.HostLabels = normalizeStringSelectorSlice(opts.HostLabels)
	if opts.MaxConcurrency > 64 {
		opts.MaxConcurrency = 64
	}
	if len(goalParts) == 0 {
		return orchestrateCommandOptions{}, errors.New("goal is required")
	}
	opts.Goal = strings.TrimSpace(strings.Join(goalParts, " "))
	if opts.Goal == "" {
		return orchestrateCommandOptions{}, errors.New("goal is required")
	}
	return opts, nil
}

func parseExecutionsCommandArgs(args []string) (orchestrateCommandOptions, error) {
	opts := orchestrateCommandOptions{
		Action: "list",
	}
	if len(args) == 0 {
		return opts, nil
	}

	mode := strings.ToLower(strings.TrimSpace(args[0]))
	startIdx := 0
	switch mode {
	case "", "list":
		opts.Action = "list"
		startIdx = 1
	case "show", "status":
		opts.Action = "status"
		startIdx = 1
	case "cancel":
		opts.Action = "cancel"
		startIdx = 1
	case "retry":
		opts.Action = "retry"
		startIdx = 1
	case "rerun":
		opts.Action = "rerun"
		startIdx = 1
	case "clone":
		opts.Action = "clone"
		startIdx = 1
	case "artifacts":
		opts.Action = "artifacts"
		startIdx = 1
	case "evidence":
		opts.Action = "evidence"
		startIdx = 1
	case "authorize":
		opts.Action = "authorize"
		startIdx = 1
	default:
		if strings.HasPrefix(mode, "-") {
			opts.Action = "list"
			startIdx = 0
		} else {
			opts.Action = "status"
			opts.ExecutionID = strings.TrimSpace(args[0])
			startIdx = 1
		}
	}

	for i := startIdx; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch lower {
		case "":
		case "--json":
			opts.JSON = true
		case "--policy-approve":
			opts.PolicyApprove = true
		case "--output":
			value, next, err := parseRequiredFlagValue(args, i, "--output")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			opts.OutputPath = strings.TrimSpace(value)
			i = next
		case "--format":
			value, next, err := parseRequiredFlagValue(args, i, "--format")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			normalized := strings.ToLower(strings.TrimSpace(value))
			if normalized != "json" && normalized != "zip" {
				return orchestrateCommandOptions{}, fmt.Errorf("invalid --format value: %s", value)
			}
			opts.Format = normalized
			i = next
		case "--limit":
			value, next, err := parseRequiredFlagValue(args, i, "--limit")
			if err != nil {
				return orchestrateCommandOptions{}, err
			}
			parsed, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || parsed <= 0 {
				return orchestrateCommandOptions{}, fmt.Errorf("invalid --limit value: %s", value)
			}
			opts.Limit = parsed
			i = next
		default:
			if strings.HasPrefix(raw, "-") {
				return orchestrateCommandOptions{}, fmt.Errorf("unknown executions option: %s", raw)
			}
			if opts.Action != "status" && opts.Action != "cancel" && opts.Action != "authorize" && opts.Action != "retry" && opts.Action != "rerun" && opts.Action != "clone" && opts.Action != "artifacts" && opts.Action != "evidence" {
				return orchestrateCommandOptions{}, fmt.Errorf("unexpected executions argument: %s", raw)
			}
			if opts.ExecutionID != "" {
				return orchestrateCommandOptions{}, errors.New("multiple execution ids provided")
			}
			opts.ExecutionID = raw
		}
	}

	if (opts.Action == "status" || opts.Action == "cancel" || opts.Action == "authorize" || opts.Action == "retry" || opts.Action == "rerun" || opts.Action == "clone" || opts.Action == "artifacts" || opts.Action == "evidence") && strings.TrimSpace(opts.ExecutionID) == "" {
		if opts.Action == "cancel" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions cancel <execution_id> [--json]")
		}
		if opts.Action == "retry" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions retry <execution_id> [--json]")
		}
		if opts.Action == "rerun" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions rerun <execution_id> [--json]")
		}
		if opts.Action == "clone" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions clone <execution_id> [--json]")
		}
		if opts.Action == "artifacts" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions artifacts <execution_id> [--json]")
		}
		if opts.Action == "evidence" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions evidence <execution_id> [--format json|zip] [--output <path>] [--json]")
		}
		if opts.Action == "authorize" {
			return orchestrateCommandOptions{}, errors.New("usage: carrier executions authorize <execution_id> [--policy-approve] [--json]")
		}
		return orchestrateCommandOptions{}, errors.New("usage: carrier executions show <execution_id> [--json]")
	}
	if opts.Action == "evidence" {
		if opts.Format == "" {
			opts.Format = "json"
		}
		if opts.Format == "zip" && opts.JSON {
			return orchestrateCommandOptions{}, errors.New("--json cannot be used with --format zip")
		}
	}
	return opts, nil
}

func parseTemplatesCommandArgs(args []string) (templatesCommandOptions, error) {
	opts := templatesCommandOptions{
		Action: "list",
		Inputs: map[string]string{},
	}
	if len(args) == 0 {
		return opts, nil
	}
	mode := strings.ToLower(strings.TrimSpace(args[0]))
	startIdx := 0
	switch mode {
	case "", "list":
		opts.Action = "list"
		startIdx = 1
	case "show":
		opts.Action = "show"
		startIdx = 1
	case "run":
		opts.Action = "run"
		startIdx = 1
	default:
		if strings.HasPrefix(mode, "-") {
			opts.Action = "list"
			startIdx = 0
		} else {
			opts.Action = "show"
			opts.TemplateID = strings.TrimSpace(args[0])
			startIdx = 1
		}
	}

	for i := startIdx; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch lower {
		case "":
		case "--json":
			opts.JSON = true
		case "--policy-approve":
			opts.PolicyApprove = true
		case "--host-id":
			value, next, err := parseRequiredFlagValue(args, i, "--host-id")
			if err != nil {
				return templatesCommandOptions{}, err
			}
			if value = strings.TrimSpace(value); value == "" {
				return templatesCommandOptions{}, errors.New("--host-id cannot be empty")
			}
			opts.HostIDs = append(opts.HostIDs, value)
			i = next
		case "--host-label":
			value, next, err := parseRequiredFlagValue(args, i, "--host-label")
			if err != nil {
				return templatesCommandOptions{}, err
			}
			if value = strings.TrimSpace(value); value == "" {
				return templatesCommandOptions{}, errors.New("--host-label cannot be empty")
			}
			opts.HostLabels = append(opts.HostLabels, value)
			i = next
		case "--provider":
			value, next, err := parseRequiredFlagValue(args, i, "--provider")
			if err != nil {
				return templatesCommandOptions{}, err
			}
			opts.Provider = strings.TrimSpace(value)
			i = next
		case "--max-concurrency":
			value, next, err := parseRequiredFlagValue(args, i, "--max-concurrency")
			if err != nil {
				return templatesCommandOptions{}, err
			}
			parsed, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || parsed <= 0 {
				return templatesCommandOptions{}, fmt.Errorf("invalid --max-concurrency value: %s", value)
			}
			opts.MaxConcurrency = parsed
			i = next
		case "--input":
			value, next, err := parseRequiredFlagValue(args, i, "--input")
			if err != nil {
				return templatesCommandOptions{}, err
			}
			key, inputValue, ok := strings.Cut(strings.TrimSpace(value), "=")
			key = strings.TrimSpace(key)
			inputValue = strings.TrimSpace(inputValue)
			if !ok || key == "" {
				return templatesCommandOptions{}, fmt.Errorf("invalid --input value: %s", value)
			}
			opts.Inputs[key] = inputValue
			i = next
		default:
			if strings.HasPrefix(raw, "-") {
				return templatesCommandOptions{}, fmt.Errorf("unknown templates option: %s", raw)
			}
			if opts.TemplateID != "" {
				return templatesCommandOptions{}, errors.New("multiple template ids provided")
			}
			opts.TemplateID = raw
		}
	}

	opts.Provider = strings.ToLower(strings.TrimSpace(opts.Provider))
	opts.HostIDs = dedupeStringSlice(opts.HostIDs)
	opts.HostLabels = normalizeStringSelectorSlice(opts.HostLabels)
	if opts.MaxConcurrency > 64 {
		opts.MaxConcurrency = 64
	}

	if (opts.Action == "show" || opts.Action == "run") && strings.TrimSpace(opts.TemplateID) == "" {
		if opts.Action == "run" {
			return templatesCommandOptions{}, errors.New("usage: carrier templates run <template_id> --input key=value [--input key=value]... [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve] [--json]")
		}
		return templatesCommandOptions{}, errors.New("usage: carrier templates show <template_id> [--json]")
	}
	return opts, nil
}

func parseTriggersCommandArgs(args []string) (triggersCommandOptions, error) {
	opts := triggersCommandOptions{
		Action: "list",
		Inputs: map[string]string{},
	}
	if len(args) == 0 {
		return opts, nil
	}

	mode := strings.ToLower(strings.TrimSpace(args[0]))
	startIdx := 0
	switch mode {
	case "", "list":
		opts.Action = "list"
		startIdx = 1
	case "show":
		opts.Action = "show"
		startIdx = 1
	case "create":
		opts.Action = "create"
		startIdx = 1
	case "update":
		opts.Action = "update"
		startIdx = 1
	case "delete":
		opts.Action = "delete"
		startIdx = 1
	default:
		if strings.HasPrefix(mode, "-") {
			opts.Action = "list"
			startIdx = 0
		} else {
			opts.Action = "show"
			opts.TriggerID = strings.TrimSpace(args[0])
			startIdx = 1
		}
	}

	for i := startIdx; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch lower {
		case "":
		case "--json":
			opts.JSON = true
		case "--policy-approve":
			opts.PolicyApprove = true
		case "--enable":
			opts.Enable = true
		case "--disable":
			opts.Disable = true
		case "--type":
			value, next, err := parseRequiredFlagValue(args, i, "--type")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.Type = strings.ToLower(strings.TrimSpace(value))
			i = next
		case "--template-id":
			value, next, err := parseRequiredFlagValue(args, i, "--template-id")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.TemplateID = strings.TrimSpace(value)
			i = next
		case "--name":
			value, next, err := parseRequiredFlagValue(args, i, "--name")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.Name = strings.TrimSpace(value)
			i = next
		case "--created-by":
			value, next, err := parseRequiredFlagValue(args, i, "--created-by")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.CreatedBy = strings.TrimSpace(value)
			i = next
		case "--host-id":
			value, next, err := parseRequiredFlagValue(args, i, "--host-id")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			if value = strings.TrimSpace(value); value == "" {
				return triggersCommandOptions{}, errors.New("--host-id cannot be empty")
			}
			opts.HostIDs = append(opts.HostIDs, value)
			i = next
		case "--host-label":
			value, next, err := parseRequiredFlagValue(args, i, "--host-label")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			if value = strings.TrimSpace(value); value == "" {
				return triggersCommandOptions{}, errors.New("--host-label cannot be empty")
			}
			opts.HostLabels = append(opts.HostLabels, value)
			i = next
		case "--provider":
			value, next, err := parseRequiredFlagValue(args, i, "--provider")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.Provider = strings.ToLower(strings.TrimSpace(value))
			i = next
		case "--max-concurrency":
			value, next, err := parseRequiredFlagValue(args, i, "--max-concurrency")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			parsed, convErr := strconv.Atoi(strings.TrimSpace(value))
			if convErr != nil || parsed <= 0 {
				return triggersCommandOptions{}, fmt.Errorf("invalid --max-concurrency value: %s", value)
			}
			opts.MaxConcurrency = parsed
			i = next
		case "--webhook-secret":
			value, next, err := parseRequiredFlagValue(args, i, "--webhook-secret")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.WebhookSecret = strings.TrimSpace(value)
			i = next
		case "--github-command":
			value, next, err := parseRequiredFlagValue(args, i, "--github-command")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.GitHubCommand = strings.TrimSpace(value)
			i = next
		case "--github-label":
			value, next, err := parseRequiredFlagValue(args, i, "--github-label")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.GitHubLabel = strings.TrimSpace(value)
			i = next
		case "--github-repository":
			value, next, err := parseRequiredFlagValue(args, i, "--github-repository")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.GitHubRepository = strings.TrimSpace(value)
			i = next
		case "--cron":
			value, next, err := parseRequiredFlagValue(args, i, "--cron")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.Cron = strings.TrimSpace(value)
			i = next
		case "--timezone":
			value, next, err := parseRequiredFlagValue(args, i, "--timezone")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			opts.Timezone = strings.ToUpper(strings.TrimSpace(value))
			i = next
		case "--input":
			value, next, err := parseRequiredFlagValue(args, i, "--input")
			if err != nil {
				return triggersCommandOptions{}, err
			}
			key, inputValue, ok := strings.Cut(strings.TrimSpace(value), "=")
			key = strings.TrimSpace(key)
			inputValue = strings.TrimSpace(inputValue)
			if !ok || key == "" {
				return triggersCommandOptions{}, fmt.Errorf("invalid --input value: %s", value)
			}
			opts.Inputs[key] = inputValue
			i = next
		default:
			if strings.HasPrefix(raw, "-") {
				return triggersCommandOptions{}, fmt.Errorf("unknown triggers option: %s", raw)
			}
			if (opts.Action == "show" || opts.Action == "update" || opts.Action == "delete") && opts.TriggerID == "" {
				opts.TriggerID = raw
				continue
			}
			return triggersCommandOptions{}, fmt.Errorf("unexpected triggers argument: %s", raw)
		}
	}

	if opts.Enable && opts.Disable {
		return triggersCommandOptions{}, errors.New("cannot combine --enable and --disable")
	}
	opts.HostIDs = dedupeStringSlice(opts.HostIDs)
	opts.HostLabels = normalizeStringSelectorSlice(opts.HostLabels)
	if opts.MaxConcurrency > 64 {
		opts.MaxConcurrency = 64
	}

	switch opts.Action {
	case "show":
		if strings.TrimSpace(opts.TriggerID) == "" {
			return triggersCommandOptions{}, errors.New("usage: carrier triggers show <trigger_id> [--json]")
		}
	case "create":
		if strings.TrimSpace(opts.Type) == "" || strings.TrimSpace(opts.TemplateID) == "" {
			return triggersCommandOptions{}, errors.New("usage: carrier triggers create --type <webhook|github|schedule> --template-id <template_id> [--name <name>] [--created-by <actor>] [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve] [--webhook-secret <secret>] [--github-command <command>] [--github-label <label>] [--github-repository <owner/repo>] [--cron <expr>] [--timezone UTC] [--input key=value]... [--json]")
		}
	case "update":
		if strings.TrimSpace(opts.TriggerID) == "" {
			return triggersCommandOptions{}, errors.New("usage: carrier triggers update <trigger_id> [--name <name>] [--template-id <template_id>] [--enable|--disable] [--created-by <actor>] [--host-id <id>]... [--host-label <label>]... [--provider <provider-id>] [--max-concurrency <n>] [--policy-approve] [--webhook-secret <secret>] [--github-command <command>] [--github-label <label>] [--github-repository <owner/repo>] [--cron <expr>] [--timezone UTC] [--input key=value]... [--json]")
		}
	case "delete":
		if strings.TrimSpace(opts.TriggerID) == "" {
			return triggersCommandOptions{}, errors.New("usage: carrier triggers delete <trigger_id> [--json]")
		}
	}
	return opts, nil
}

func dedupeStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func runKeysGenerate(out io.Writer, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "default"
	}
	privatePath, pubPath, fingerprint, created, err := ensureCarrierManagedKey(alias)
	if err != nil {
		return err
	}
	if created {
		_, _ = fmt.Fprintf(out, "generated key %s\n", alias)
	} else {
		_, _ = fmt.Fprintf(out, "key %s already exists\n", alias)
	}
	_, _ = fmt.Fprintf(out, "private: %s\n", privatePath)
	_, _ = fmt.Fprintf(out, "public: %s\n", pubPath)
	_, _ = fmt.Fprintf(out, "fingerprint: %s\n", fingerprint)
	return nil
}

func runKeysList(out io.Writer) error {
	keysDir, err := carrierKeysDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(keysDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, _ = fmt.Fprintln(out, "no managed keys found")
			return nil
		}
		return fmt.Errorf("read keys dir: %w", err)
	}
	type keyEntry struct {
		alias       string
		privatePath string
		fingerprint string
	}
	keys := make([]keyEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasSuffix(name, ".pub") {
			continue
		}
		privatePath := filepath.Join(keysDir, name)
		if _, err := os.Stat(privatePath + ".pub"); err != nil {
			continue
		}
		fp, err := carrierKeyFingerprintByAlias(name)
		if err != nil {
			continue
		}
		keys = append(keys, keyEntry{alias: name, privatePath: privatePath, fingerprint: fp})
	}
	if len(keys) == 0 {
		_, _ = fmt.Fprintln(out, "no managed keys found")
		return nil
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].alias < keys[j].alias })
	for _, item := range keys {
		_, _ = fmt.Fprintf(out, "%s %s %s\n", item.alias, item.fingerprint, item.privatePath)
	}
	return nil
}

func runKeysDelete(out io.Writer, alias string) error {
	if err := validateCarrierKeyAlias(alias); err != nil {
		return err
	}
	privatePath, err := carrierManagedKeyPrivatePath(alias)
	if err != nil {
		return err
	}
	pubPath := privatePath + ".pub"
	_ = os.Remove(pubPath)
	if err := os.Remove(privatePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("managed key %q does not exist", alias)
		}
		return fmt.Errorf("delete key %q: %w", alias, err)
	}
	_, _ = fmt.Fprintf(out, "deleted key %s\n", alias)
	return nil
}

func carrierKeysDir() (string, error) {
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for keys: %w", err)
	}
	return filepath.Join(home, ".carrier", "keys"), nil
}

func validateCarrierKeyAlias(alias string) error {
	trimmed := strings.TrimSpace(alias)
	if trimmed == "" {
		return errors.New("key alias is required")
	}
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("invalid key alias %q: use letters, numbers, '-' or '_'", alias)
	}
	return nil
}

func carrierManagedKeyPrivatePath(alias string) (string, error) {
	if err := validateCarrierKeyAlias(alias); err != nil {
		return "", err
	}
	dir, err := carrierKeysDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, strings.TrimSpace(alias)), nil
}

func ensureCarrierManagedKey(alias string) (privatePath string, pubPath string, fingerprint string, created bool, err error) {
	privatePath, err = carrierManagedKeyPrivatePath(alias)
	if err != nil {
		return "", "", "", false, err
	}
	pubPath = privatePath + ".pub"
	if _, err = os.Stat(privatePath); err == nil {
		fp, fpErr := carrierKeyFingerprintByAlias(alias)
		if fpErr != nil {
			return "", "", "", false, fpErr
		}
		return privatePath, pubPath, fp, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", "", false, fmt.Errorf("stat key: %w", err)
	}
	keysDir, err := carrierKeysDir()
	if err != nil {
		return "", "", "", false, err
	}
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return "", "", "", false, fmt.Errorf("create keys dir: %w", err)
	}
	_, privateKey, genErr := ed25519.GenerateKey(rand.Reader)
	if genErr != nil {
		return "", "", "", false, fmt.Errorf("generate ed25519 key: %w", genErr)
	}
	privDER, marshalErr := x509.MarshalPKCS8PrivateKey(privateKey)
	if marshalErr != nil {
		return "", "", "", false, fmt.Errorf("marshal private key: %w", marshalErr)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	if writeErr := os.WriteFile(privatePath, privPEM, 0o600); writeErr != nil {
		return "", "", "", false, fmt.Errorf("write private key: %w", writeErr)
	}
	pubRaw, sshPub, pubErr := marshalAuthorizedEd25519(privateKey.Public().(ed25519.PublicKey))
	if pubErr != nil {
		return "", "", "", false, fmt.Errorf("build public key: %w", pubErr)
	}
	if writeErr := os.WriteFile(pubPath, pubRaw, 0o644); writeErr != nil {
		return "", "", "", false, fmt.Errorf("write public key: %w", writeErr)
	}
	return privatePath, pubPath, sshFingerprint(sshPub), true, nil
}

func carrierKeyFingerprintByAlias(alias string) (string, error) {
	privatePath, err := carrierManagedKeyPrivatePath(alias)
	if err != nil {
		return "", err
	}
	pubRaw, err := os.ReadFile(privatePath + ".pub")
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	sshPub, err := parseAuthorizedKeyToSSH(pubRaw)
	if err != nil {
		return "", fmt.Errorf("parse public key: %w", err)
	}
	return sshFingerprint(sshPub), nil
}

func marshalAuthorizedEd25519(pub ed25519.PublicKey) ([]byte, gossh.PublicKey, error) {
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("create SSH public key: %w", err)
	}
	line := gossh.MarshalAuthorizedKey(sshPub)
	return line, sshPub, nil
}

func parseAuthorizedKeyToSSH(raw []byte) (gossh.PublicKey, error) {
	pubKey, _, _, _, err := gossh.ParseAuthorizedKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse authorized key: %w", err)
	}
	return pubKey, nil
}

func sshFingerprint(pub gossh.PublicKey) string {
	return gossh.FingerprintSHA256(pub)
}

func resolveRemoteKeyPath(out io.Writer, explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit, nil
	}
	privatePath, _, fingerprint, created, err := ensureCarrierManagedKey("default")
	if err != nil {
		return "", err
	}
	if created {
		_, _ = fmt.Fprintf(out, "Generated default managed SSH key: %s (%s)\n", privatePath, fingerprint)
	} else {
		_, _ = fmt.Fprintf(out, "Using default managed SSH key: %s (%s)\n", privatePath, fingerprint)
	}
	return privatePath, nil
}

func parseRequiredFlagValue(args []string, index int, flag string) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("missing value for %s", flag)
	}
	value := strings.TrimSpace(args[index+1])
	if value == "" {
		return "", index, fmt.Errorf("missing value for %s", flag)
	}
	if strings.HasPrefix(value, "--") {
		return "", index, fmt.Errorf("missing value for %s", flag)
	}
	return value, index + 1, nil
}

func parseUpdateCommandArgs(args []string) (updateCommandOptions, error) {
	opts := updateCommandOptions{
		Channel: "stable",
		Timeout: defaultUpdateTimeout,
	}
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(strings.TrimSpace(args[i]))
		switch arg {
		case "":
		case "--check":
			opts.Check = true
		case "--yes":
			opts.Yes = true
		case "--dry-run":
			opts.DryRun = true
		case "--force":
			opts.Force = true
		case "--no-restart":
			opts.NoRestart = true
		case "--json":
			opts.JSON = true
		case "--channel":
			if i+1 >= len(args) {
				return updateCommandOptions{}, errors.New("missing value for --channel")
			}
			opts.Channel = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
			switch opts.Channel {
			case "stable", "beta", "dev":
			default:
				return updateCommandOptions{}, fmt.Errorf("invalid --channel value: %s", args[i])
			}
		case "--tag":
			if i+1 >= len(args) {
				return updateCommandOptions{}, errors.New("missing value for --tag")
			}
			opts.Tag = strings.TrimSpace(args[i+1])
			i++
			if opts.Tag == "" {
				return updateCommandOptions{}, errors.New("missing value for --tag")
			}
		case "--timeout":
			if i+1 >= len(args) {
				return updateCommandOptions{}, errors.New("missing value for --timeout")
			}
			arg = strings.TrimSpace(args[i+1])
			i++
			seconds, err := strconv.Atoi(arg)
			if err != nil || seconds <= 0 {
				return updateCommandOptions{}, fmt.Errorf("invalid --timeout value: %s", arg)
			}
			opts.Timeout = time.Duration(seconds) * time.Second
		default:
			return updateCommandOptions{}, fmt.Errorf("unknown update option: %s", arg)
		}
	}
	opts.Channel = strings.ToLower(strings.TrimSpace(opts.Channel))
	if opts.Channel == "" {
		opts.Channel = "stable"
	}
	return opts, nil
}

func runVersionCommand(out io.Writer, opts versionCommandOptions) error {
	info := versionInfo{
		Version:   carrierVersion,
		Commit:    carrierCommit,
		BuildDate: carrierBuildDate,
		GoVersion: runtime.Version(),
	}
	if opts.JSON {
		raw, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(raw))
		return nil
	}
	_, _ = fmt.Fprintf(out, "carrier %s\n", info.Version)
	_, _ = fmt.Fprintf(out, "commit: %s\n", info.Commit)
	_, _ = fmt.Fprintf(out, "build date: %s\n", info.BuildDate)
	_, _ = fmt.Fprintf(out, "go version: %s\n", info.GoVersion)
	return nil
}

func runUpdate(in io.Reader, out io.Writer, opts updateCommandOptions) error {
	if opts.DryRun {
		opts.Check = true
	}

	repoRoot, err := resolveRepoRootForUpdate(opts.Timeout)
	if err != nil {
		return err
	}

	if err := fetchGitRefs(opts.Timeout, repoRoot); err != nil {
		return err
	}

	target, targetSource, err := resolveUpdateTarget(opts.Timeout, repoRoot, opts)
	if err != nil {
		return err
	}
	current, err := resolveCurrentRef(opts.Timeout, repoRoot)
	if err != nil {
		return err
	}
	if opts.JSON {
		type payload struct {
			Current    string `json:"current"`
			Target     string `json:"target"`
			Source     string `json:"source"`
			Timeout    int    `json:"timeoutSeconds"`
			Channel    string `json:"channel"`
			Tag        string `json:"tag"`
			Check      bool   `json:"check"`
			DryRun     bool   `json:"dryRun"`
			Force      bool   `json:"force"`
			JSON       bool   `json:"json"`
			NoRestart  bool   `json:"noRestart"`
			WouldApply bool   `json:"wouldApply"`
		}
		raw, err := json.MarshalIndent(payload{
			Current:    current,
			Target:     target,
			Source:     targetSource,
			Timeout:    int(opts.Timeout.Seconds()),
			Channel:    opts.Channel,
			Tag:        opts.Tag,
			Check:      opts.Check,
			DryRun:     opts.DryRun,
			Force:      opts.Force,
			JSON:       opts.JSON,
			NoRestart:  opts.NoRestart,
			WouldApply: !opts.Check,
		}, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, string(raw))
	}

	if opts.Check {
		if !opts.JSON {
			_, _ = fmt.Fprintf(out, "Current: %s\n", current)
			_, _ = fmt.Fprintf(out, "Target: %s (%s)\n", target, targetSource)
			if opts.NoRestart {
				_, _ = fmt.Fprintln(out, "no-restart: enabled (no service restart action)")
			}
			_, _ = fmt.Fprintln(out, "check mode: no changes applied")
		}
		return nil
	}

	if !opts.Force {
		clean, err := isWorkingTreeClean(opts.Timeout, repoRoot)
		if err != nil {
			return err
		}
		if !clean {
			return errors.New("working tree is not clean; pass --force to continue")
		}
	}

	if !opts.Yes {
		if opts.JSON {
			return errors.New("--json in apply mode requires --yes to avoid interactive prompts in machine output")
		}
		ok, err := promptYesNo(bufio.NewReader(in), out, fmt.Sprintf("Apply update from %s to %s?", current, target), false)
		if err != nil {
			return err
		}
		if !ok {
			_, _ = fmt.Fprintln(out, "Update canceled.")
			return nil
		}
	}

	if !opts.DryRun {
		if err := applyGitUpdate(opts.Timeout, repoRoot, target); err != nil {
			return err
		}
	}
	if opts.JSON {
		return nil
	}
	if opts.NoRestart {
		_, _ = fmt.Fprintln(out, "no-restart: enabled, service restart skipped")
	}
	_, _ = fmt.Fprintf(out, "Updated to %s.\n", target)
	return nil
}

func resolveRepoRootForUpdate(timeout time.Duration) (string, error) {
	return runGitCommand(timeout, "", "rev-parse", "--show-toplevel")
}

func resolveCurrentRef(timeout time.Duration, repoRoot string) (string, error) {
	ref, err := runGitCommand(timeout, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		ref = strings.TrimSpace(ref)
		if ref != "" && ref != "HEAD" {
			return ref, nil
		}
	}
	ref, err = runGitCommand(timeout, repoRoot, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(ref), nil
}

func isWorkingTreeClean(timeout time.Duration, repoRoot string) (bool, error) {
	out, err := runGitCommand(timeout, repoRoot, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

func resolveUpdateTarget(timeout time.Duration, repoRoot string, opts updateCommandOptions) (string, string, error) {
	if opts.Tag != "" {
		exact := strings.TrimSpace(opts.Tag)
		refs, err := runGitCommand(timeout, repoRoot, "tag", "--list", exact)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(refs) == "" {
			return "", "", fmt.Errorf("tag %q not found", exact)
		}
		return exact, "tag", nil
	}

	channel := strings.ToLower(strings.TrimSpace(opts.Channel))
	if channel == "" {
		channel = "stable"
	}
	switch channel {
	case "dev":
		return "origin/main", "channel dev", nil
	case "beta":
		tag, err := latestTagMatching(timeout, repoRoot, true)
		if err != nil {
			return "", "", err
		}
		return tag, "channel beta", nil
	case "stable":
		tag, err := latestTagMatching(timeout, repoRoot, false)
		if err != nil {
			return "", "", err
		}
		return tag, "channel stable", nil
	default:
		return "", "", fmt.Errorf("invalid channel %q", channel)
	}
}

func fetchGitRefs(timeout time.Duration, repoRoot string) error {
	_, err := runGitCommand(timeout, repoRoot, "fetch", "--all", "--tags", "--prune", "--force")
	return err
}

func latestTagMatching(timeout time.Duration, repoRoot string, requireBeta bool) (string, error) {
	raw, err := runGitCommand(timeout, repoRoot, "tag", "--list", "--sort=-creatordate")
	if err != nil {
		return "", fmt.Errorf("list tags: %w", err)
	}
	for _, line := range strings.Split(raw, "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		isBeta := strings.Contains(strings.ToLower(tag), "beta")
		if isBeta == requireBeta {
			return tag, nil
		}
	}
	if requireBeta {
		return "", errors.New("no matching beta tags found")
	}
	return "", errors.New("no matching stable tags found")
}

func applyGitUpdate(timeout time.Duration, repoRoot, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("empty update target")
	}
	if strings.HasPrefix(target, "origin/") {
		branch := strings.TrimPrefix(target, "origin/")
		_, err := runGitCommand(timeout, repoRoot, "checkout", "-B", branch, target)
		if err != nil {
			return fmt.Errorf("checkout %s: %w", target, err)
		}
		_, err = runGitCommand(timeout, repoRoot, "pull", "--rebase", "--ff-only", "origin", branch)
		if err != nil {
			return fmt.Errorf("pull remote %s: %w", target, err)
		}
		return nil
	}
	_, err := runGitCommand(timeout, repoRoot, "checkout", target)
	if err != nil {
		return fmt.Errorf("checkout %s: %w", target, err)
	}
	return nil
}

func runGitCommand(timeout time.Duration, repoRoot string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return execGitCommand(ctx, repoRoot, args...)
}

func needsInitialOnboard(loadFn func() (*configv2.Config, string, error)) bool {
	cfg, _, err := loadFn()
	if err != nil || cfg == nil {
		return true
	}
	if len(cfg.ModelList) == 0 {
		return true
	}
	if strings.TrimSpace(cfg.ConfiguredAt) == "" {
		return true
	}
	return false
}

func runBootstrap(in io.Reader, out io.Writer) error {
	_, _ = fmt.Fprintln(out, "Carrier Bootstrap")
	_, _ = fmt.Fprintln(out, "-----------------")

	if needsInitialOnboard(configv2.Load) {
		_, _ = fmt.Fprintln(out, "Carrier is not onboarded. Starting onboarding...")
		if err := runOnboardFlow(in, out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(out, "Carrier is already onboarded. Ensuring runtime services...")
		applyConfigV2Env(out)
		if _, err := ensureWebUIServicesFlow(out); err != nil {
			return err
		}
	}

	printRuntimeSummary(out)

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Terminal is ready for the next command.")
	_, _ = fmt.Fprintln(out, "Examples:")
	_, _ = fmt.Fprintln(out, "  - carrier add picoclaw")
	_, _ = fmt.Fprintln(out, "  - carrier install picoclaw")
	_, _ = fmt.Fprintln(out, "  - carrier add picoclaw --webui")
	_, _ = fmt.Fprintln(out, "  - carrier list")
	return nil
}

func runStop(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "Carrier Stop")
	_, _ = fmt.Fprintln(out, "------------")

	daemonStopped, daemonMsg := stopBackgroundService(
		"daemon",
		daemonProbeBaseURL(),
		daemonHealthProbe,
	)
	gatewayStopped, gatewayMsg := stopBackgroundService(
		"gateway",
		gatewayProbeBaseURL(),
		gatewayHealthProbe,
	)

	_, _ = fmt.Fprintf(out, "- daemon: %s\n", daemonMsg)
	_, _ = fmt.Fprintf(out, "- gateway: %s\n", gatewayMsg)

	if daemonStopped || gatewayStopped {
		_, _ = fmt.Fprintln(out, "✅ stop complete")
		return nil
	}
	_, _ = fmt.Fprintln(out, "No background daemon/gateway process was stopped.")
	return nil
}

func runReset(out io.Writer) error {
	if runStopFlow != nil {
		if err := runStopFlow(out); err != nil {
			_, _ = fmt.Fprintf(out, "warning: stop flow before reset failed: %v\n", err)
		}
	}

	paths := make([]string, 0, 16)
	home, err := resolveCarrierHomeDir()
	if err == nil {
		paths = append(paths,
			filepath.Join(home, ".carrier"),
			filepath.Join(home, ".picoclaw"),
			filepath.Join(home, ".openclaw"),
			filepath.Join(home, ".zeroclaw"),
		)
	}
	for _, key := range []string{
		"CARRIER_CONFIG",
		"CARRIER_ONBOARD_CONFIG",
		"CARRIER_INSTANCE_STORE",
		"CARRIER_CREDENTIAL_STORE",
		"CARRIER_REMOTE_KEY_DIR",
		"CARRIER_BOOTSTRAP_RUN_DIR",
		"CARRIER_BOOTSTRAP_LOG_DIR",
	} {
		if path := strings.TrimSpace(os.Getenv(key)); path != "" {
			paths = append(paths, path)
		}
	}

	seen := map[string]bool{}
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}

	_, _ = fmt.Fprintln(out, "reset complete")
	return nil
}

func runStartInstance(out io.Writer, target string) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	inst, idx, err := resolveManagedInstanceTarget(instances, target)
	if err != nil {
		return err
	}
	if _, err := ensureDaemonRunning(out); err != nil {
		return err
	}
	startPayload := map[string]interface{}{}
	if inst.Isolation {
		startPayload["isolation"] = true
	}
	if err := daemonAgentActionWithPayloadWithProgress(out, inst.AgentID, "start", startPayload, false); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	instances[idx].RuntimeState = "running"
	instances[idx].UpdatedAt = now
	if err := saveManagedInstances(path, instances); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ started instance %s (%s)\n", inst.ID, managedInstanceDisplayName(inst))
	return nil
}

func runStopInstance(out io.Writer, target string) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	inst, idx, err := resolveManagedInstanceTarget(instances, target)
	if err != nil {
		return err
	}
	if _, err := ensureDaemonRunning(out); err != nil {
		return err
	}
	if err := daemonAgentActionWithProgress(out, inst.AgentID, "stop", false); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	instances[idx].RuntimeState = "stopped"
	instances[idx].UpdatedAt = now
	if err := saveManagedInstances(path, instances); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ stopped instance %s (%s)\n", inst.ID, managedInstanceDisplayName(inst))
	return nil
}

func runUpgradeInstance(out io.Writer, target string) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	inst, idx, err := resolveManagedInstanceTarget(instances, target)
	if err != nil {
		return err
	}
	if _, err := ensureDaemonRunning(out); err != nil {
		return err
	}
	if err := daemonAgentActionWithProgress(out, inst.AgentID, "upgrade", false); err != nil {
		return err
	}
	instances[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(path, instances); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ upgraded instance %s (%s)\n", inst.ID, managedInstanceDisplayName(inst))
	return nil
}

func runStatusInstance(out io.Writer, target string) error {
	return runStatusInstanceWithOptions(out, target, false)
}

func runStatusInstanceWithOptions(out io.Writer, target string, includeMetrics bool) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	inst, idx, err := resolveManagedInstanceTarget(instances, target)
	if err != nil {
		return err
	}
	if _, err := ensureDaemonRunning(out); err != nil {
		return err
	}
	status, err := daemonFetchAgentStatus(inst.AgentID)
	if err != nil {
		return err
	}
	if runtimeState := strings.TrimSpace(status.RuntimeState); runtimeState != "" {
		instances[idx].RuntimeState = runtimeState
	}
	instances[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(path, instances); err != nil {
		return err
	}
	installState := strings.TrimSpace(status.InstallState)
	if installState == "" {
		installState = "unknown"
	}
	runtimeState := strings.TrimSpace(status.RuntimeState)
	if runtimeState == "" {
		runtimeState = "unknown"
	}
	_, _ = fmt.Fprintf(out, "Instance %s (%s)\n", inst.ID, managedInstanceDisplayName(inst))
	_, _ = fmt.Fprintf(out, "  install=%s runtime=%s\n", installState, runtimeState)
	if lastErr := strings.TrimSpace(status.LastError); lastErr != "" {
		_, _ = fmt.Fprintf(out, "  lastError=%s\n", lastErr)
	}
	if includeMetrics {
		metrics, metricsErr := daemonFetchAgentMetrics(inst.AgentID)
		if metricsErr != nil {
			_, _ = fmt.Fprintf(out, "  metrics_error=%v\n", metricsErr)
		} else {
			_, _ = fmt.Fprintf(out, "  metrics cpu=%.2f%% rss=%d uptime=%ds restarts=%d\n",
				metrics.CPUPercent,
				metrics.MemoryRSS,
				metrics.Uptime,
				metrics.RestartCount,
			)
			if strings.TrimSpace(metrics.LastErrorAt) != "" {
				_, _ = fmt.Fprintf(out, "  metrics lastErrorAt=%s\n", metrics.LastErrorAt)
			}
		}
	}
	return nil
}

func runUninstallInstance(out io.Writer, instanceID string) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	inst, idx, err := resolveManagedInstanceTarget(instances, instanceID)
	if err != nil {
		return err
	}
	if _, err := ensureDaemonRunning(out); err != nil {
		return err
	}
	_ = daemonAgentAction(inst.AgentID, "stop")
	if err := daemonAgentAction(inst.AgentID, "uninstall"); err != nil {
		return err
	}
	if err := cleanupManagedInstanceFiles(inst); err != nil {
		_, _ = fmt.Fprintf(out, "Warning: instance file cleanup failed: %v\n", err)
	}
	instances = append(instances[:idx], instances[idx+1:]...)
	if err := saveManagedInstances(path, instances); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ uninstalled instance %s (%s)\n", inst.ID, managedInstanceDisplayName(inst))
	return nil
}

func runListInstances(out io.Writer) error {
	instances, _, err := loadManagedInstances()
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		_, _ = fmt.Fprintln(out, "No managed agent instances found.")
		return nil
	}
	_, _ = fmt.Fprintln(out, "Managed agent instances:")
	for _, inst := range instances {
		runtimeState := strings.TrimSpace(inst.RuntimeState)
		if runtimeState == "" {
			runtimeState = "unknown"
		}
		_, _ = fmt.Fprintf(out, "- id=%s name=%s type=%s state=%s port=%d gateway=%s\n",
			strings.TrimSpace(inst.ID),
			managedInstanceDisplayName(inst),
			strings.TrimSpace(inst.Type),
			runtimeState,
			inst.Port,
			strings.TrimSpace(inst.GatewayURL),
		)
	}
	return nil
}

const orchestratorLocalHostID = "local"

type orchestrateDecomposeTask = sharedorchestration.DecomposeTask

type orchestrateRequiredWorker = sharedorchestration.RequiredWorker

type orchestrateTaskUnit = sharedorchestration.TaskUnit

type orchestrateExecutionPayload struct {
	Goal              string                      `json:"goal"`
	TemplateID        string                      `json:"templateId,omitempty"`
	RequestedProvider string                      `json:"requestedProvider,omitempty"`
	IdempotencyKey    string                      `json:"idempotencyKey,omitempty"`
	ApprovalScope     string                      `json:"approvalScope"`
	RequiredWorkers   []orchestrateRequiredWorker `json:"requiredWorkers"`
	TaskUnits         []orchestrateTaskUnit       `json:"taskUnits"`
	MaxConcurrency    int                         `json:"maxConcurrency,omitempty"`
}

type orchestrateTaskResultSnapshot struct {
	TaskID          string `json:"taskId"`
	Status          string `json:"status"`
	HostID          string `json:"hostId,omitempty"`
	AgentID         string `json:"agentId,omitempty"`
	Summary         string `json:"summary,omitempty"`
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
	FailureReason   string `json:"failureReason,omitempty"`
	FailureCategory string `json:"failureCategory,omitempty"`
	LatencyMs       int64  `json:"latencyMs,omitempty"`
}

type orchestrateArtifactSnapshot struct {
	ID          string `json:"id"`
	TaskID      string `json:"taskId,omitempty"`
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}

type orchestrateExecutionOutcomeSnapshot struct {
	Summary         string                        `json:"summary,omitempty"`
	FailureReason   string                        `json:"failureReason,omitempty"`
	FailureCategory string                        `json:"failureCategory,omitempty"`
	Artifacts       []orchestrateArtifactSnapshot `json:"artifacts,omitempty"`
}

type orchestrateToolPolicySnapshot struct {
	Mode         string   `json:"mode,omitempty"`
	AllowedTools []string `json:"allowedTools,omitempty"`
}

type orchestrateExecutionPolicyTargetSnapshot struct {
	HostID     string   `json:"hostId,omitempty"`
	HostLabels []string `json:"hostLabels,omitempty"`
	AgentID    string   `json:"agentId"`
	Count      int      `json:"count,omitempty"`
}

type orchestrateExecutionPolicySnapshot struct {
	Decision                       string                                     `json:"decision"`
	Reason                         string                                     `json:"reason,omitempty"`
	Summary                        string                                     `json:"summary,omitempty"`
	RequiresInfrastructureApproval bool                                       `json:"requiresInfrastructureApproval"`
	ConfiguredMaxConcurrency       int                                        `json:"configuredMaxConcurrency,omitempty"`
	EffectiveMaxConcurrency        int                                        `json:"effectiveMaxConcurrency,omitempty"`
	ToolPolicy                     orchestrateToolPolicySnapshot              `json:"toolPolicy,omitempty"`
	MaxTaskTimeoutMs               int                                        `json:"maxTaskTimeoutMs,omitempty"`
	MaxRetryBudget                 int                                        `json:"maxRetryBudget,omitempty"`
	MatchedRuleID                  string                                     `json:"matchedRuleId,omitempty"`
	MatchedRuleName                string                                     `json:"matchedRuleName,omitempty"`
	ApprovedBy                     string                                     `json:"approvedBy,omitempty"`
	ApprovedAt                     string                                     `json:"approvedAt,omitempty"`
	Targets                        []orchestrateExecutionPolicyTargetSnapshot `json:"targets,omitempty"`
}

type orchestrateProviderGovernanceResolutionSnapshot struct {
	HostID                string  `json:"hostId,omitempty"`
	AgentID               string  `json:"agentId,omitempty"`
	Source                string  `json:"source,omitempty"`
	ProfileID             string  `json:"profileId,omitempty"`
	ProfileName           string  `json:"profileName,omitempty"`
	Provider              string  `json:"provider,omitempty"`
	Model                 string  `json:"model,omitempty"`
	Status                string  `json:"status,omitempty"`
	SyncMode              string  `json:"syncMode,omitempty"`
	EstimatedInputTokens  int     `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int     `json:"estimatedOutputTokens,omitempty"`
	EstimatedTotalTokens  int     `json:"estimatedTotalTokens,omitempty"`
	EstimatedCostUSD      float64 `json:"estimatedCostUsd,omitempty"`
	SuccessfulTasks       int     `json:"successfulTasks,omitempty"`
	FailedTasks           int     `json:"failedTasks,omitempty"`
	AvgLatencyMs          int64   `json:"avgLatencyMs,omitempty"`
	Message               string  `json:"message,omitempty"`
}

type orchestrateExecutionGovernanceSnapshot struct {
	ProviderResolutions []orchestrateProviderGovernanceResolutionSnapshot `json:"providerResolutions,omitempty"`
}

type orchestrateExecutionSnapshot struct {
	ID                   string                                 `json:"id"`
	Goal                 string                                 `json:"goal"`
	TemplateID           string                                 `json:"templateId,omitempty"`
	TriggerSource        string                                 `json:"triggerSource,omitempty"`
	TriggerID            string                                 `json:"triggerId,omitempty"`
	TriggerEvent         string                                 `json:"triggerEvent,omitempty"`
	TriggerPayloadDigest string                                 `json:"triggerPayloadDigest,omitempty"`
	Initiator            string                                 `json:"initiator,omitempty"`
	RequestedProvider    string                                 `json:"requestedProvider,omitempty"`
	ParentExecutionID    string                                 `json:"parentExecutionId,omitempty"`
	SourceExecutionID    string                                 `json:"sourceExecutionId,omitempty"`
	LaunchReason         string                                 `json:"launchReason,omitempty"`
	Status               string                                 `json:"status"`
	Error                string                                 `json:"error,omitempty"`
	MaxConcurrency       int                                    `json:"maxConcurrency,omitempty"`
	Policy               orchestrateExecutionPolicySnapshot     `json:"policy,omitempty"`
	Governance           orchestrateExecutionGovernanceSnapshot `json:"governance,omitempty"`
	Outcome              orchestrateExecutionOutcomeSnapshot    `json:"outcome,omitempty"`
	CreatedAt            string                                 `json:"createdAt,omitempty"`
	UpdatedAt            string                                 `json:"updatedAt,omitempty"`
	TaskUnits            []orchestrateTaskUnit                  `json:"taskUnits,omitempty"`
	Results              []orchestrateTaskResultSnapshot        `json:"results,omitempty"`
}

type orchestrateWorkerLeaseSnapshot struct {
	ID      string `json:"id"`
	HostID  string `json:"hostId"`
	AgentID string `json:"agentId"`
	State   string `json:"state"`
}

type orchestrateExecutionResponse struct {
	Result    string                           `json:"result"`
	ErrorCode string                           `json:"errorCode,omitempty"`
	Message   string                           `json:"message,omitempty"`
	Execution orchestrateExecutionSnapshot     `json:"execution"`
	Workers   []orchestrateWorkerLeaseSnapshot `json:"workers,omitempty"`
}

type orchestrateExecutionListResponse struct {
	Result     string                         `json:"result"`
	ErrorCode  string                         `json:"errorCode,omitempty"`
	Message    string                         `json:"message,omitempty"`
	Executions []orchestrateExecutionSnapshot `json:"executions"`
}

type orchestrateExecutionArtifactsResponse struct {
	Result    string                        `json:"result"`
	ErrorCode string                        `json:"errorCode,omitempty"`
	Message   string                        `json:"message,omitempty"`
	Artifacts []orchestrateArtifactSnapshot `json:"artifacts"`
}

type orchestrateAuditEventSnapshot struct {
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Result string `json:"result,omitempty"`
}

type orchestrateEvidenceBundleSnapshot struct {
	GeneratedAt      string                          `json:"generatedAt,omitempty"`
	Execution        orchestrateExecutionSnapshot    `json:"execution"`
	ArtifactManifest []orchestrateArtifactSnapshot   `json:"artifactManifest,omitempty"`
	Audit            []orchestrateAuditEventSnapshot `json:"audit,omitempty"`
}

type orchestrateEvidenceBundleResponse struct {
	Result    string                            `json:"result"`
	ErrorCode string                            `json:"errorCode,omitempty"`
	Message   string                            `json:"message,omitempty"`
	Evidence  orchestrateEvidenceBundleSnapshot `json:"evidence"`
}

type orchestratePlanSnapshot = sharedorchestration.Plan

type executionTemplateInputFieldSnapshot struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Description  string `json:"description,omitempty"`
	Placeholder  string `json:"placeholder,omitempty"`
	Required     bool   `json:"required,omitempty"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

type executionTemplateTaskSnapshot struct {
	ID            string `json:"id"`
	AgentID       string `json:"agentId,omitempty"`
	InputTemplate string `json:"inputTemplate,omitempty"`
}

type executionTemplateSnapshot struct {
	ID                  string                                `json:"id"`
	Name                string                                `json:"name"`
	Description         string                                `json:"description,omitempty"`
	DefaultGoalTemplate string                                `json:"defaultGoalTemplate,omitempty"`
	InputSchema         []executionTemplateInputFieldSnapshot `json:"inputSchema,omitempty"`
	PlannerTasks        []executionTemplateTaskSnapshot       `json:"plannerTasks,omitempty"`
}

type executionTemplateListResponse struct {
	Result    string                      `json:"result"`
	ErrorCode string                      `json:"errorCode,omitempty"`
	Message   string                      `json:"message,omitempty"`
	Templates []executionTemplateSnapshot `json:"templates"`
}

type executionTemplateResponse struct {
	Result    string                    `json:"result"`
	ErrorCode string                    `json:"errorCode,omitempty"`
	Message   string                    `json:"message,omitempty"`
	Template  executionTemplateSnapshot `json:"template"`
}

type executionTemplateLaunchResponse struct {
	Result    string                       `json:"result"`
	ErrorCode string                       `json:"errorCode,omitempty"`
	Message   string                       `json:"message,omitempty"`
	Template  executionTemplateSnapshot    `json:"template"`
	Execution orchestrateExecutionSnapshot `json:"execution"`
}

type executionTriggerConfigSnapshot struct {
	Inputs                  map[string]string `json:"inputs,omitempty"`
	Provider                string            `json:"provider,omitempty"`
	HostIDs                 []string          `json:"hostIds,omitempty"`
	HostLabels              []string          `json:"hostLabels,omitempty"`
	MaxConcurrency          int               `json:"maxConcurrency,omitempty"`
	PolicyApprove           bool              `json:"policyApprove,omitempty"`
	WebhookSecretConfigured bool              `json:"webhookSecretConfigured,omitempty"`
	GitHubCommand           string            `json:"githubCommand,omitempty"`
	GitHubLabel             string            `json:"githubLabel,omitempty"`
	GitHubRepository        string            `json:"githubRepository,omitempty"`
	Cron                    string            `json:"cron,omitempty"`
	Timezone                string            `json:"timezone,omitempty"`
}

type executionTriggerSnapshot struct {
	ID              string                         `json:"id"`
	Name            string                         `json:"name"`
	Type            string                         `json:"type"`
	TemplateID      string                         `json:"templateId"`
	Enabled         bool                           `json:"enabled"`
	CreatedBy       string                         `json:"createdBy,omitempty"`
	Config          executionTriggerConfigSnapshot `json:"config,omitempty"`
	LastTriggeredAt string                         `json:"lastTriggeredAt,omitempty"`
	LastExecutionID string                         `json:"lastExecutionId,omitempty"`
	LastError       string                         `json:"lastError,omitempty"`
	TriggeredCount  int64                          `json:"triggeredCount,omitempty"`
	NextRunAt       string                         `json:"nextRunAt,omitempty"`
	CreatedAt       string                         `json:"createdAt,omitempty"`
	UpdatedAt       string                         `json:"updatedAt,omitempty"`
}

type executionTriggerListResponse struct {
	Result    string                     `json:"result"`
	ErrorCode string                     `json:"errorCode,omitempty"`
	Message   string                     `json:"message,omitempty"`
	Triggers  []executionTriggerSnapshot `json:"triggers"`
}

type executionTriggerResponse struct {
	Result    string                   `json:"result"`
	ErrorCode string                   `json:"errorCode,omitempty"`
	Message   string                   `json:"message,omitempty"`
	Trigger   executionTriggerSnapshot `json:"trigger"`
}

type executionTriggerDeleteResponse struct {
	Result    string `json:"result"`
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message,omitempty"`
	Deleted   bool   `json:"deleted"`
}

func runTemplatesCommand(out io.Writer, opts templatesCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	switch opts.Action {
	case "list":
		resp, raw, err := fetchExecutionTemplates()
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderExecutionTemplateList(resp.Templates))
		return nil
	case "show":
		resp, raw, err := fetchExecutionTemplate(opts.TemplateID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderExecutionTemplate(resp.Template))
		return nil
	case "run":
		resp, raw, err := launchExecutionTemplate(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		executionID := strings.TrimSpace(resp.Execution.ID)
		_, _ = fmt.Fprintf(out, "template launch accepted: %s\n", executionID)
		_, _ = fmt.Fprintf(out, "template: %s\n", strings.TrimSpace(resp.Template.ID))
		_, _ = fmt.Fprintf(out, "status: %s\n", strings.TrimSpace(resp.Execution.Status))
		if executionID != "" {
			_, _ = fmt.Fprintf(out, "next: carrier executions show %s\n", executionID)
		}
		return nil
	default:
		return fmt.Errorf("unsupported templates action: %s", opts.Action)
	}
}

func runTriggersCommand(out io.Writer, opts triggersCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	switch opts.Action {
	case "list":
		resp, raw, err := fetchExecutionTriggers()
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderExecutionTriggerList(resp.Triggers))
		return nil
	case "show":
		resp, raw, err := fetchExecutionTrigger(opts.TriggerID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderExecutionTrigger(resp.Trigger))
		return nil
	case "create":
		resp, raw, err := createExecutionTrigger(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderExecutionTrigger(resp.Trigger))
		return nil
	case "update":
		resp, raw, err := updateExecutionTrigger(opts)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		_, _ = fmt.Fprintln(out, renderExecutionTrigger(resp.Trigger))
		return nil
	case "delete":
		resp, raw, err := deleteExecutionTrigger(opts.TriggerID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return writePrettyJSON(out, raw)
		}
		if resp.Deleted {
			_, _ = fmt.Fprintf(out, "deleted trigger %s\n", strings.TrimSpace(opts.TriggerID))
			return nil
		}
		return fmt.Errorf("delete trigger %s did not report success", strings.TrimSpace(opts.TriggerID))
	default:
		return fmt.Errorf("unsupported triggers action: %s", opts.Action)
	}
}

func runOrchestrateCommand(out io.Writer, opts orchestrateCommandOptions) error {
	switch opts.Action {
	case "plan":
		if _, err := ensureDaemonRunning(out); err != nil {
			return err
		}
		return runOrchestratePlan(out, opts)
	case "list":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateList(out, opts.Limit, opts.JSON)
	case "status":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateStatus(out, opts.ExecutionID, opts.JSON)
	case "cancel":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateCancel(out, opts.ExecutionID, opts.JSON)
	case "retry", "rerun", "clone":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateDerivedExecutionAction(out, opts.Action, opts.ExecutionID, opts.JSON)
	case "artifacts":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateArtifacts(out, opts.ExecutionID, opts.JSON)
	case "evidence":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateEvidence(out, opts.ExecutionID, opts.Format, opts.OutputPath, opts.JSON)
	case "authorize":
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateAuthorize(out, opts.ExecutionID, opts.PolicyApprove, opts.JSON)
	case "run":
		if _, err := ensureDaemonRunning(out); err != nil {
			return err
		}
		if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
			return err
		}
		return runOrchestrateStart(out, opts)
	default:
		return fmt.Errorf("unsupported orchestrate action: %s", opts.Action)
	}
}

func runOrchestratePlan(out io.Writer, opts orchestrateCommandOptions) error {
	plan, err := buildOrchestratePlan(opts)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writePrettyJSONValue(out, plan)
	}
	_, _ = fmt.Fprintln(out, renderOrchestratePlan(plan))
	return nil
}

func runOrchestrateList(out io.Writer, limit int, outputJSON bool) error {
	resp, _, err := fetchOrchestratorExecutions(limit)
	if err != nil {
		return err
	}
	if outputJSON {
		return writePrettyJSONValue(out, resp)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecutionList(resp.Executions))
	return nil
}

func runOrchestrateStatus(out io.Writer, executionID string, outputJSON bool) error {
	resp, raw, err := fetchOrchestratorExecution(executionID)
	if err != nil {
		return err
	}
	if outputJSON {
		return writePrettyJSON(out, raw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecution(resp))
	return nil
}

func runOrchestrateCancel(out io.Writer, executionID string, outputJSON bool) error {
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return errors.New("execution id is required")
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID) + "/cancel"
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, path, map[string]interface{}{
		"actor": "carrier-cli",
	}, 45*time.Second)
	if err != nil {
		return err
	}
	resp, decodeErr := decodeOrchestrateExecutionResponse(raw)
	if decodeErr != nil {
		return decodeErr
	}
	if outputJSON {
		return writePrettyJSON(out, raw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecution(resp))
	return nil
}

func runOrchestrateDerivedExecutionAction(out io.Writer, action, executionID string, outputJSON bool) error {
	trimmedAction := strings.ToLower(strings.TrimSpace(action))
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return errors.New("execution id is required")
	}
	switch trimmedAction {
	case "retry", "rerun", "clone":
	default:
		return fmt.Errorf("unsupported derived execution action: %s", action)
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID) + "/" + trimmedAction
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, path, map[string]interface{}{}, 45*time.Second)
	if err != nil {
		return err
	}
	resp, decodeErr := decodeOrchestrateExecutionResponse(raw)
	if decodeErr != nil {
		return decodeErr
	}
	if outputJSON {
		return writePrettyJSON(out, raw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecution(resp))
	if nextID := strings.TrimSpace(resp.Execution.ID); nextID != "" {
		_, _ = fmt.Fprintf(out, "next: carrier executions show %s\n", nextID)
	}
	return nil
}

func runOrchestrateArtifacts(out io.Writer, executionID string, outputJSON bool) error {
	resp, raw, err := fetchOrchestratorExecutionArtifacts(executionID)
	if err != nil {
		return err
	}
	if outputJSON {
		return writePrettyJSON(out, raw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecutionArtifacts(strings.TrimSpace(executionID), resp.Artifacts))
	return nil
}

func runOrchestrateEvidence(out io.Writer, executionID, format, outputPath string, outputJSON bool) error {
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	if normalizedFormat == "" {
		normalizedFormat = "json"
	}
	if normalizedFormat == "zip" {
		raw, err := fetchOrchestratorExecutionEvidenceArchive(executionID)
		if err != nil {
			return err
		}
		destination := strings.TrimSpace(outputPath)
		if destination == "" {
			destination = strings.TrimSpace(executionID) + "-evidence.zip"
		}
		if err := os.WriteFile(destination, raw, 0o600); err != nil {
			return fmt.Errorf("write evidence archive: %w", err)
		}
		_, _ = fmt.Fprintf(out, "execution evidence written: %s\n", destination)
		return nil
	}

	resp, raw, err := fetchOrchestratorExecutionEvidence(executionID)
	if err != nil {
		return err
	}
	if destination := strings.TrimSpace(outputPath); destination != "" {
		var pretty bytes.Buffer
		if err := writePrettyJSON(&pretty, raw); err != nil {
			return err
		}
		if err := os.WriteFile(destination, pretty.Bytes(), 0o600); err != nil {
			return fmt.Errorf("write evidence json: %w", err)
		}
		_, _ = fmt.Fprintf(out, "execution evidence written: %s\n", destination)
		return nil
	}
	if outputJSON {
		return writePrettyJSON(out, raw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecutionEvidence(strings.TrimSpace(executionID), resp.Evidence))
	return nil
}

func runOrchestrateAuthorize(out io.Writer, executionID string, policyApprove bool, outputJSON bool) error {
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return errors.New("execution id is required")
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID) + "/authorize"
	body := map[string]interface{}{
		"approved": true,
		"actor":    "carrier-cli",
	}
	if policyApprove {
		body["policyApproved"] = true
	}
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, path, body, 45*time.Second)
	if err != nil {
		return err
	}
	resp, decodeErr := decodeOrchestrateExecutionResponse(raw)
	if decodeErr != nil {
		return decodeErr
	}
	if outputJSON {
		return writePrettyJSON(out, raw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecution(resp))
	return nil
}

func runOrchestrateStart(out io.Writer, opts orchestrateCommandOptions) error {
	plan, err := buildOrchestratePlan(opts)
	if err != nil {
		return err
	}

	payload := orchestrateExecutionPayload{
		Goal:              strings.TrimSpace(plan.Goal),
		TemplateID:        strings.TrimSpace(plan.TemplateID),
		RequestedProvider: strings.TrimSpace(plan.Provider),
		IdempotencyKey:    strings.TrimSpace(opts.IdempotencyKey),
		ApprovalScope:     plan.ApprovalScope,
		RequiredWorkers:   plan.RequiredWorkers,
		TaskUnits:         plan.TaskUnits,
		MaxConcurrency:    plan.MaxConcurrency,
	}

	createRaw, _, err := gatewayRequestWithTimeout(http.MethodPost, "/api/v1/orchestrator/executions", payload, 90*time.Second)
	if err != nil {
		return err
	}
	createResp, err := decodeOrchestrateExecutionResponse(createRaw)
	if err != nil {
		return err
	}
	executionID := strings.TrimSpace(createResp.Execution.ID)
	if executionID == "" {
		return errors.New("orchestrator create response missing execution id")
	}

	authorizeBody := map[string]interface{}{
		"approved": true,
		"actor":    "carrier-cli",
	}
	if opts.MaxConcurrency > 0 {
		authorizeBody["maxConcurrency"] = opts.MaxConcurrency
	}
	if opts.PolicyApprove {
		authorizeBody["policyApproved"] = true
	}
	authorizePath := "/api/v1/orchestrator/executions/" + neturl.PathEscape(executionID) + "/authorize"
	authorizeRaw, _, err := gatewayRequestWithTimeout(http.MethodPost, authorizePath, authorizeBody, 90*time.Second)
	if err != nil {
		return err
	}
	authorizeResp, err := decodeOrchestrateExecutionResponse(authorizeRaw)
	if err != nil {
		return err
	}

	if opts.Async {
		if opts.JSON {
			return writePrettyJSON(out, authorizeRaw)
		}
		_, _ = fmt.Fprintf(out, "orchestrator execution accepted: %s\n", executionID)
		_, _ = fmt.Fprintf(out, "status: %s\n", strings.TrimSpace(authorizeResp.Execution.Status))
		_, _ = fmt.Fprintf(out, "Use `carrier executions show %s` to check progress.\n", executionID)
		return nil
	}

	finalResp, finalRaw, err := waitForOrchestratorExecution(executionID, opts.Timeout)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writePrettyJSON(out, finalRaw)
	}
	_, _ = fmt.Fprintln(out, renderOrchestrateExecution(finalResp))
	return nil
}

func buildOrchestratePlan(opts orchestrateCommandOptions) (orchestratePlanSnapshot, error) {
	tasks, err := decomposeOrchestrateGoal(opts.Goal, opts.Provider)
	if err != nil {
		return orchestratePlanSnapshot{}, err
	}
	return sharedorchestration.BuildPlan(sharedorchestration.BuildPlanInput{
		Goal:           strings.TrimSpace(opts.Goal),
		Provider:       strings.TrimSpace(opts.Provider),
		HostIDs:        opts.HostIDs,
		HostLabels:     opts.HostLabels,
		MaxConcurrency: opts.MaxConcurrency,
		Tasks:          tasks,
	})
}

func decomposeOrchestrateGoal(goal, provider string) ([]orchestrateDecomposeTask, error) {
	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal == "" {
		return nil, errors.New("goal is required")
	}
	body := map[string]interface{}{
		"goal": trimmedGoal,
	}
	if providerID := strings.TrimSpace(provider); providerID != "" {
		body["provider"] = providerID
	}
	raw, _, err := daemonRequestWithTimeout(http.MethodPost, "/api/v1/base-agent/decompose", body, 90*time.Second)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tasks []orchestrateDecomposeTask `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode base-agent decompose response: %w", err)
	}
	out := make([]orchestrateDecomposeTask, 0, len(payload.Tasks))
	seen := map[string]struct{}{}
	for idx, task := range payload.Tasks {
		input := strings.TrimSpace(task.Input)
		if input == "" {
			continue
		}
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = fmt.Sprintf("task-%d", idx+1)
		}
		if _, exists := seen[taskID]; exists {
			taskID = fmt.Sprintf("%s-%d", taskID, idx+1)
		}
		seen[taskID] = struct{}{}
		agentID := sharedorchestration.NormalizeAgentID(task.AgentID)
		out = append(out, orchestrateDecomposeTask{
			ID:      taskID,
			Input:   input,
			AgentID: agentID,
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func decodeOrchestrateExecutionResponse(raw []byte) (orchestrateExecutionResponse, error) {
	var resp orchestrateExecutionResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return orchestrateExecutionResponse{}, fmt.Errorf("decode orchestrator response: %w", err)
	}
	return resp, nil
}

func decodeOrchestrateExecutionListResponse(raw []byte) (orchestrateExecutionListResponse, error) {
	var resp orchestrateExecutionListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return orchestrateExecutionListResponse{}, fmt.Errorf("decode orchestrator execution list response: %w", err)
	}
	return resp, nil
}

func decodeExecutionTemplateListResponse(raw []byte) (executionTemplateListResponse, error) {
	var resp executionTemplateListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return executionTemplateListResponse{}, fmt.Errorf("decode execution template list response: %w", err)
	}
	return resp, nil
}

func decodeExecutionTemplateResponse(raw []byte) (executionTemplateResponse, error) {
	var resp executionTemplateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return executionTemplateResponse{}, fmt.Errorf("decode execution template response: %w", err)
	}
	return resp, nil
}

func decodeExecutionTemplateLaunchResponse(raw []byte) (executionTemplateLaunchResponse, error) {
	var resp executionTemplateLaunchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return executionTemplateLaunchResponse{}, fmt.Errorf("decode execution template launch response: %w", err)
	}
	return resp, nil
}

func decodeExecutionTriggerListResponse(raw []byte) (executionTriggerListResponse, error) {
	var resp executionTriggerListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return executionTriggerListResponse{}, fmt.Errorf("decode execution trigger list response: %w", err)
	}
	return resp, nil
}

func decodeExecutionTriggerResponse(raw []byte) (executionTriggerResponse, error) {
	var resp executionTriggerResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return executionTriggerResponse{}, fmt.Errorf("decode execution trigger response: %w", err)
	}
	return resp, nil
}

func decodeExecutionTriggerDeleteResponse(raw []byte) (executionTriggerDeleteResponse, error) {
	var resp executionTriggerDeleteResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return executionTriggerDeleteResponse{}, fmt.Errorf("decode execution trigger delete response: %w", err)
	}
	return resp, nil
}

func decodeOrchestrateExecutionArtifactsResponse(raw []byte) (orchestrateExecutionArtifactsResponse, error) {
	var resp orchestrateExecutionArtifactsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return orchestrateExecutionArtifactsResponse{}, fmt.Errorf("decode orchestrator execution artifacts response: %w", err)
	}
	return resp, nil
}

func decodeOrchestrateEvidenceBundleResponse(raw []byte) (orchestrateEvidenceBundleResponse, error) {
	var resp orchestrateEvidenceBundleResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return orchestrateEvidenceBundleResponse{}, fmt.Errorf("decode orchestrator evidence response: %w", err)
	}
	return resp, nil
}

func fetchOrchestratorExecution(executionID string) (orchestrateExecutionResponse, []byte, error) {
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return orchestrateExecutionResponse{}, nil, errors.New("execution id is required")
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID)
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return orchestrateExecutionResponse{}, nil, err
	}
	resp, decodeErr := decodeOrchestrateExecutionResponse(raw)
	if decodeErr != nil {
		return orchestrateExecutionResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func fetchOrchestratorExecutions(limit int) (orchestrateExecutionListResponse, []byte, error) {
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, "/api/v1/orchestrator/executions", nil, 45*time.Second)
	if err != nil {
		return orchestrateExecutionListResponse{}, nil, err
	}
	resp, decodeErr := decodeOrchestrateExecutionListResponse(raw)
	if decodeErr != nil {
		return orchestrateExecutionListResponse{}, nil, decodeErr
	}
	sort.Slice(resp.Executions, func(i, j int) bool {
		left, leftOK := parseManagedTimestamp(resp.Executions[i].UpdatedAt)
		right, rightOK := parseManagedTimestamp(resp.Executions[j].UpdatedAt)
		switch {
		case leftOK && rightOK:
			if left.Equal(right) {
				return resp.Executions[i].ID > resp.Executions[j].ID
			}
			return left.After(right)
		case leftOK:
			return true
		case rightOK:
			return false
		default:
			return resp.Executions[i].ID > resp.Executions[j].ID
		}
	})
	if limit > 0 && len(resp.Executions) > limit {
		resp.Executions = resp.Executions[:limit]
	}
	return resp, raw, nil
}

func fetchOrchestratorExecutionArtifacts(executionID string) (orchestrateExecutionArtifactsResponse, []byte, error) {
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return orchestrateExecutionArtifactsResponse{}, nil, errors.New("execution id is required")
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID) + "/artifacts"
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return orchestrateExecutionArtifactsResponse{}, nil, err
	}
	resp, decodeErr := decodeOrchestrateExecutionArtifactsResponse(raw)
	if decodeErr != nil {
		return orchestrateExecutionArtifactsResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func fetchOrchestratorExecutionEvidence(executionID string) (orchestrateEvidenceBundleResponse, []byte, error) {
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return orchestrateEvidenceBundleResponse{}, nil, errors.New("execution id is required")
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID) + "/evidence?format=json"
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return orchestrateEvidenceBundleResponse{}, nil, err
	}
	resp, decodeErr := decodeOrchestrateEvidenceBundleResponse(raw)
	if decodeErr != nil {
		return orchestrateEvidenceBundleResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func fetchOrchestratorExecutionEvidenceArchive(executionID string) ([]byte, error) {
	trimmedID := strings.TrimSpace(executionID)
	if trimmedID == "" {
		return nil, errors.New("execution id is required")
	}
	path := "/api/v1/orchestrator/executions/" + neturl.PathEscape(trimmedID) + "/evidence?format=zip"
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func fetchExecutionTemplates() (executionTemplateListResponse, []byte, error) {
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, "/api/v1/templates", nil, 45*time.Second)
	if err != nil {
		return executionTemplateListResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTemplateListResponse(raw)
	if decodeErr != nil {
		return executionTemplateListResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func fetchExecutionTemplate(templateID string) (executionTemplateResponse, []byte, error) {
	trimmedID := strings.TrimSpace(templateID)
	if trimmedID == "" {
		return executionTemplateResponse{}, nil, errors.New("template id is required")
	}
	path := "/api/v1/templates/" + neturl.PathEscape(trimmedID)
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return executionTemplateResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTemplateResponse(raw)
	if decodeErr != nil {
		return executionTemplateResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func fetchExecutionTriggers() (executionTriggerListResponse, []byte, error) {
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, "/api/v1/triggers", nil, 45*time.Second)
	if err != nil {
		return executionTriggerListResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTriggerListResponse(raw)
	if decodeErr != nil {
		return executionTriggerListResponse{}, nil, decodeErr
	}
	sort.Slice(resp.Triggers, func(i, j int) bool {
		left, leftOK := parseManagedTimestamp(resp.Triggers[i].UpdatedAt)
		right, rightOK := parseManagedTimestamp(resp.Triggers[j].UpdatedAt)
		switch {
		case leftOK && rightOK:
			if left.Equal(right) {
				return resp.Triggers[i].ID > resp.Triggers[j].ID
			}
			return left.After(right)
		case leftOK:
			return true
		case rightOK:
			return false
		default:
			return resp.Triggers[i].ID > resp.Triggers[j].ID
		}
	})
	return resp, raw, nil
}

func fetchExecutionTrigger(triggerID string) (executionTriggerResponse, []byte, error) {
	trimmedID := strings.TrimSpace(triggerID)
	if trimmedID == "" {
		return executionTriggerResponse{}, nil, errors.New("trigger id is required")
	}
	path := "/api/v1/triggers/" + neturl.PathEscape(trimmedID)
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return executionTriggerResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTriggerResponse(raw)
	if decodeErr != nil {
		return executionTriggerResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func createExecutionTrigger(opts triggersCommandOptions) (executionTriggerResponse, []byte, error) {
	body := map[string]interface{}{
		"type":       strings.TrimSpace(opts.Type),
		"templateId": strings.TrimSpace(opts.TemplateID),
		"name":       strings.TrimSpace(opts.Name),
		"createdBy":  strings.TrimSpace(firstNonEmpty(opts.CreatedBy, "carrier-cli")),
		"enabled":    !opts.Disable,
		"config": map[string]interface{}{
			"inputs":           opts.Inputs,
			"provider":         strings.TrimSpace(opts.Provider),
			"hostIds":          opts.HostIDs,
			"hostLabels":       opts.HostLabels,
			"maxConcurrency":   opts.MaxConcurrency,
			"policyApprove":    opts.PolicyApprove,
			"webhookSecret":    strings.TrimSpace(opts.WebhookSecret),
			"githubCommand":    strings.TrimSpace(opts.GitHubCommand),
			"githubLabel":      strings.TrimSpace(opts.GitHubLabel),
			"githubRepository": strings.TrimSpace(opts.GitHubRepository),
			"cron":             strings.TrimSpace(opts.Cron),
			"timezone":         firstNonEmpty(strings.TrimSpace(opts.Timezone), "UTC"),
		},
	}
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, "/api/v1/triggers", body, 90*time.Second)
	if err != nil {
		return executionTriggerResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTriggerResponse(raw)
	if decodeErr != nil {
		return executionTriggerResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func updateExecutionTrigger(opts triggersCommandOptions) (executionTriggerResponse, []byte, error) {
	trimmedID := strings.TrimSpace(opts.TriggerID)
	if trimmedID == "" {
		return executionTriggerResponse{}, nil, errors.New("trigger id is required")
	}
	body := map[string]interface{}{}
	if name := strings.TrimSpace(opts.Name); name != "" {
		body["name"] = name
	}
	if templateID := strings.TrimSpace(opts.TemplateID); templateID != "" {
		body["templateId"] = templateID
	}
	if createdBy := strings.TrimSpace(opts.CreatedBy); createdBy != "" {
		body["createdBy"] = createdBy
	}
	if opts.Enable || opts.Disable {
		body["enabled"] = opts.Enable && !opts.Disable
	}
	config := map[string]interface{}{}
	if len(opts.Inputs) > 0 {
		config["inputs"] = opts.Inputs
	}
	if provider := strings.TrimSpace(opts.Provider); provider != "" {
		config["provider"] = provider
	}
	if len(opts.HostIDs) > 0 {
		config["hostIds"] = opts.HostIDs
	}
	if len(opts.HostLabels) > 0 {
		config["hostLabels"] = opts.HostLabels
	}
	if opts.MaxConcurrency > 0 {
		config["maxConcurrency"] = opts.MaxConcurrency
	}
	if opts.PolicyApprove {
		config["policyApprove"] = true
	}
	if webhookSecret := strings.TrimSpace(opts.WebhookSecret); webhookSecret != "" {
		config["webhookSecret"] = webhookSecret
	}
	if githubCommand := strings.TrimSpace(opts.GitHubCommand); githubCommand != "" {
		config["githubCommand"] = githubCommand
	}
	if githubLabel := strings.TrimSpace(opts.GitHubLabel); githubLabel != "" {
		config["githubLabel"] = githubLabel
	}
	if githubRepo := strings.TrimSpace(opts.GitHubRepository); githubRepo != "" {
		config["githubRepository"] = githubRepo
	}
	if cronExpr := strings.TrimSpace(opts.Cron); cronExpr != "" {
		config["cron"] = cronExpr
	}
	if timezone := strings.TrimSpace(opts.Timezone); timezone != "" {
		config["timezone"] = timezone
	}
	if len(config) > 0 {
		body["config"] = config
	}
	path := "/api/v1/triggers/" + neturl.PathEscape(trimmedID)
	raw, _, err := gatewayRequestWithTimeout(http.MethodPatch, path, body, 90*time.Second)
	if err != nil {
		return executionTriggerResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTriggerResponse(raw)
	if decodeErr != nil {
		return executionTriggerResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func deleteExecutionTrigger(triggerID string) (executionTriggerDeleteResponse, []byte, error) {
	trimmedID := strings.TrimSpace(triggerID)
	if trimmedID == "" {
		return executionTriggerDeleteResponse{}, nil, errors.New("trigger id is required")
	}
	path := "/api/v1/triggers/" + neturl.PathEscape(trimmedID)
	raw, _, err := gatewayRequestWithTimeout(http.MethodDelete, path, nil, 45*time.Second)
	if err != nil {
		return executionTriggerDeleteResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTriggerDeleteResponse(raw)
	if decodeErr != nil {
		return executionTriggerDeleteResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func launchExecutionTemplate(opts templatesCommandOptions) (executionTemplateLaunchResponse, []byte, error) {
	trimmedID := strings.TrimSpace(opts.TemplateID)
	if trimmedID == "" {
		return executionTemplateLaunchResponse{}, nil, errors.New("template id is required")
	}
	body := map[string]interface{}{
		"inputs":         opts.Inputs,
		"provider":       strings.TrimSpace(opts.Provider),
		"hostIds":        opts.HostIDs,
		"hostLabels":     opts.HostLabels,
		"maxConcurrency": opts.MaxConcurrency,
		"policyApprove":  opts.PolicyApprove,
		"actor":          "carrier-cli",
	}
	path := "/api/v1/templates/" + neturl.PathEscape(trimmedID) + "/launch"
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, path, body, 90*time.Second)
	if err != nil {
		return executionTemplateLaunchResponse{}, nil, err
	}
	resp, decodeErr := decodeExecutionTemplateLaunchResponse(raw)
	if decodeErr != nil {
		return executionTemplateLaunchResponse{}, nil, decodeErr
	}
	return resp, raw, nil
}

func waitForOrchestratorExecution(executionID string, timeout time.Duration) (orchestrateExecutionResponse, []byte, error) {
	if timeout <= 0 {
		timeout = defaultOrchestrateWaitTimeout
	}
	deadline := time.Now().Add(timeout)
	var lastStatus string
	for {
		resp, raw, err := fetchOrchestratorExecution(executionID)
		if err != nil {
			return orchestrateExecutionResponse{}, nil, err
		}
		status := strings.ToLower(strings.TrimSpace(resp.Execution.Status))
		lastStatus = status
		if status == "completed" || status == "partial_completed" || status == "failed" || status == "retryable_failed" || status == "declined" || status == "cancelled" {
			return resp, raw, nil
		}
		if time.Now().After(deadline) {
			return orchestrateExecutionResponse{}, nil, fmt.Errorf("wait orchestrator execution timed out after %s (last status: %s)", timeout, firstNonEmpty(lastStatus, "unknown"))
		}
		time.Sleep(orchestratePollInterval)
	}
}

func renderExecutionTemplateList(templates []executionTemplateSnapshot) string {
	if len(templates) == 0 {
		return "No execution templates found."
	}
	lines := []string{"Execution templates:"}
	for _, template := range templates {
		line := fmt.Sprintf("- %s · %s", firstNonEmpty(strings.TrimSpace(template.ID), "unknown"), firstNonEmpty(strings.TrimSpace(template.Name), "(unnamed)"))
		if description := strings.TrimSpace(template.Description); description != "" {
			line += " · " + truncateOrchestrateText(description, 96)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderExecutionTemplate(template executionTemplateSnapshot) string {
	lines := []string{
		fmt.Sprintf("execution template %s", firstNonEmpty(strings.TrimSpace(template.ID), "unknown")),
		fmt.Sprintf("name: %s", firstNonEmpty(strings.TrimSpace(template.Name), "(unnamed)")),
	}
	if description := strings.TrimSpace(template.Description); description != "" {
		lines = append(lines, "description: "+description)
	}
	if goalTemplate := strings.TrimSpace(template.DefaultGoalTemplate); goalTemplate != "" {
		lines = append(lines, "goal template: "+goalTemplate)
	}
	if len(template.InputSchema) > 0 {
		lines = append(lines, "inputs:")
		for _, field := range template.InputSchema {
			parts := []string{firstNonEmpty(strings.TrimSpace(field.ID), "input")}
			if label := strings.TrimSpace(field.Label); label != "" {
				parts = append(parts, label)
			}
			if field.Required {
				parts = append(parts, "required")
			}
			if defaultValue := strings.TrimSpace(field.DefaultValue); defaultValue != "" {
				parts = append(parts, "default="+defaultValue)
			}
			lines = append(lines, "- "+strings.Join(parts, " · "))
		}
	}
	if len(template.PlannerTasks) > 0 {
		lines = append(lines, "planner tasks:")
		for _, task := range template.PlannerTasks {
			lines = append(lines, fmt.Sprintf(
				"- %s · %s · %s",
				firstNonEmpty(strings.TrimSpace(task.ID), "task"),
				firstNonEmpty(strings.TrimSpace(task.AgentID), "zeroclaw"),
				firstNonEmpty(strings.TrimSpace(task.InputTemplate), "(no input template)"),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func renderExecutionTriggerList(triggers []executionTriggerSnapshot) string {
	if len(triggers) == 0 {
		return "No execution triggers found."
	}
	lines := []string{"Execution triggers:"}
	for _, trigger := range triggers {
		parts := []string{
			firstNonEmpty(strings.TrimSpace(trigger.ID), "unknown"),
			firstNonEmpty(strings.TrimSpace(trigger.Name), "(unnamed)"),
			"type=" + firstNonEmpty(strings.TrimSpace(trigger.Type), "unknown"),
			"template=" + firstNonEmpty(strings.TrimSpace(trigger.TemplateID), "unknown"),
			"enabled=" + strconv.FormatBool(trigger.Enabled),
		}
		if nextRunAt := strings.TrimSpace(trigger.NextRunAt); nextRunAt != "" {
			parts = append(parts, "nextRun="+nextRunAt)
		}
		lines = append(lines, "- "+strings.Join(parts, " · "))
	}
	return strings.Join(lines, "\n")
}

func renderExecutionTrigger(trigger executionTriggerSnapshot) string {
	lines := []string{
		fmt.Sprintf("execution trigger %s", firstNonEmpty(strings.TrimSpace(trigger.ID), "unknown")),
		fmt.Sprintf("name: %s", firstNonEmpty(strings.TrimSpace(trigger.Name), "(unnamed)")),
		fmt.Sprintf("type: %s", firstNonEmpty(strings.TrimSpace(trigger.Type), "unknown")),
		fmt.Sprintf("template: %s", firstNonEmpty(strings.TrimSpace(trigger.TemplateID), "unknown")),
		fmt.Sprintf("enabled=%t", trigger.Enabled),
	}
	if createdBy := strings.TrimSpace(trigger.CreatedBy); createdBy != "" {
		lines = append(lines, "created by: "+createdBy)
	}
	if updatedAt := strings.TrimSpace(trigger.UpdatedAt); updatedAt != "" {
		lines = append(lines, "updated at: "+updatedAt)
	}
	if nextRunAt := strings.TrimSpace(trigger.NextRunAt); nextRunAt != "" {
		lines = append(lines, "next run: "+nextRunAt)
	}
	if trigger.TriggeredCount > 0 {
		lines = append(lines, fmt.Sprintf("triggered count: %d", trigger.TriggeredCount))
	}
	if lastExecutionID := strings.TrimSpace(trigger.LastExecutionID); lastExecutionID != "" {
		lines = append(lines, "last execution: "+lastExecutionID)
	}
	if lastTriggeredAt := strings.TrimSpace(trigger.LastTriggeredAt); lastTriggeredAt != "" {
		lines = append(lines, "last triggered at: "+lastTriggeredAt)
	}
	if lastError := strings.TrimSpace(trigger.LastError); lastError != "" {
		lines = append(lines, "last error: "+lastError)
	}
	config := trigger.Config
	if provider := strings.TrimSpace(config.Provider); provider != "" {
		lines = append(lines, "provider: "+provider)
	}
	if len(config.HostIDs) > 0 {
		lines = append(lines, "host ids: "+strings.Join(config.HostIDs, ", "))
	}
	if len(config.HostLabels) > 0 {
		lines = append(lines, "host labels: "+strings.Join(config.HostLabels, ", "))
	}
	if config.MaxConcurrency > 0 {
		lines = append(lines, fmt.Sprintf("max concurrency: %d", config.MaxConcurrency))
	}
	if config.PolicyApprove {
		lines = append(lines, "policy approve: true")
	}
	if config.WebhookSecretConfigured {
		lines = append(lines, "webhook secret: configured")
	}
	if githubCommand := strings.TrimSpace(config.GitHubCommand); githubCommand != "" {
		lines = append(lines, "github command: "+githubCommand)
	}
	if githubLabel := strings.TrimSpace(config.GitHubLabel); githubLabel != "" {
		lines = append(lines, "github label: "+githubLabel)
	}
	if githubRepository := strings.TrimSpace(config.GitHubRepository); githubRepository != "" {
		lines = append(lines, "github repository: "+githubRepository)
	}
	if cronExpr := strings.TrimSpace(config.Cron); cronExpr != "" {
		lines = append(lines, "cron: "+cronExpr)
	}
	if timezone := strings.TrimSpace(config.Timezone); timezone != "" {
		lines = append(lines, "timezone: "+timezone)
	}
	if len(config.Inputs) > 0 {
		inputKeys := make([]string, 0, len(config.Inputs))
		for key := range config.Inputs {
			inputKeys = append(inputKeys, key)
		}
		sort.Strings(inputKeys)
		lines = append(lines, "inputs:")
		for _, key := range inputKeys {
			lines = append(lines, fmt.Sprintf("- %s=%s", key, config.Inputs[key]))
		}
	}
	return strings.Join(lines, "\n")
}

func renderOrchestrateExecution(resp orchestrateExecutionResponse) string {
	execution := resp.Execution
	total, completed, failed := summarizeOrchestrateExecution(execution)
	lines := []string{
		fmt.Sprintf("orchestrator execution %s", strings.TrimSpace(execution.ID)),
		fmt.Sprintf("status: %s", firstNonEmpty(strings.TrimSpace(execution.Status), "unknown")),
		fmt.Sprintf("goal: %s", strings.TrimSpace(execution.Goal)),
		fmt.Sprintf("tasks: total=%d completed=%d failed=%d", total, completed, failed),
	}
	if templateID := strings.TrimSpace(execution.TemplateID); templateID != "" {
		lines = append(lines, "template: "+templateID)
	}
	if triggerSource := strings.TrimSpace(execution.TriggerSource); triggerSource != "" || strings.TrimSpace(execution.TriggerID) != "" || strings.TrimSpace(execution.Initiator) != "" {
		lines = append(lines, fmt.Sprintf(
			"trigger: source=%s id=%s event=%s initiator=%s",
			firstNonEmpty(triggerSource, "n/a"),
			firstNonEmpty(strings.TrimSpace(execution.TriggerID), "n/a"),
			firstNonEmpty(strings.TrimSpace(execution.TriggerEvent), "n/a"),
			firstNonEmpty(strings.TrimSpace(execution.Initiator), "n/a"),
		))
	}
	if requestedProvider := strings.TrimSpace(execution.RequestedProvider); requestedProvider != "" {
		lines = append(lines, "requested provider: "+requestedProvider)
	}
	if len(execution.Governance.ProviderResolutions) > 0 {
		lines = append(lines, "provider trace:")
		for _, resolution := range execution.Governance.ProviderResolutions {
			hostTarget := firstNonEmpty(strings.TrimSpace(resolution.HostID), "local")
			agentTarget := firstNonEmpty(strings.TrimSpace(resolution.AgentID), "unknown")
			providerModel := strings.Join(filterEmptyStrings([]string{
				strings.TrimSpace(resolution.Provider),
				strings.TrimSpace(resolution.Model),
			}), "/")
			if providerModel == "" {
				providerModel = "unbound"
			}
			parts := []string{
				fmt.Sprintf("%s/%s", hostTarget, agentTarget),
				"source=" + firstNonEmpty(strings.TrimSpace(resolution.Source), "none"),
			}
			if profileName := firstNonEmpty(strings.TrimSpace(resolution.ProfileName), strings.TrimSpace(resolution.ProfileID)); profileName != "" {
				parts = append(parts, "profile="+profileName)
			}
			parts = append(parts, providerModel)
			if resolution.SuccessfulTasks > 0 || resolution.FailedTasks > 0 {
				parts = append(parts, fmt.Sprintf("tasks=%d/%d", resolution.SuccessfulTasks, resolution.FailedTasks))
			}
			if resolution.AvgLatencyMs > 0 {
				parts = append(parts, fmt.Sprintf("latency=%dms", resolution.AvgLatencyMs))
			}
			if resolution.EstimatedTotalTokens > 0 {
				parts = append(parts, fmt.Sprintf("tokens=%d", resolution.EstimatedTotalTokens))
			}
			if resolution.EstimatedCostUSD > 0 {
				parts = append(parts, fmt.Sprintf("cost=$%.4f", resolution.EstimatedCostUSD))
			}
			lines = append(lines, "- "+strings.Join(parts, " · "))
		}
	}
	if parentID := strings.TrimSpace(execution.ParentExecutionID); parentID != "" || strings.TrimSpace(execution.SourceExecutionID) != "" || strings.TrimSpace(execution.LaunchReason) != "" {
		lines = append(lines, fmt.Sprintf(
			"lineage: parent=%s source=%s launch=%s",
			firstNonEmpty(parentID, "n/a"),
			firstNonEmpty(strings.TrimSpace(execution.SourceExecutionID), "n/a"),
			firstNonEmpty(strings.TrimSpace(execution.LaunchReason), "n/a"),
		))
	}
	if errText := strings.TrimSpace(execution.Error); errText != "" {
		lines = append(lines, "error: "+errText)
	}
	if summary := strings.TrimSpace(execution.Outcome.Summary); summary != "" {
		lines = append(lines, "outcome: "+summary)
	}
	if failureCategory := strings.TrimSpace(execution.Outcome.FailureCategory); failureCategory != "" || strings.TrimSpace(execution.Outcome.FailureReason) != "" {
		lines = append(lines, fmt.Sprintf(
			"failure: %s (%s)",
			firstNonEmpty(failureCategory, "unknown"),
			firstNonEmpty(strings.TrimSpace(execution.Outcome.FailureReason), "n/a"),
		))
	}
	if policyDecision := strings.TrimSpace(execution.Policy.Decision); policyDecision != "" {
		policyParts := []string{"policy: " + policyDecision}
		if toolMode := strings.TrimSpace(execution.Policy.ToolPolicy.Mode); toolMode != "" {
			policyParts = append(policyParts, "tool mode="+toolMode)
		}
		if execution.Policy.EffectiveMaxConcurrency > 0 {
			policyParts = append(policyParts, fmt.Sprintf("effective concurrency=%d", execution.Policy.EffectiveMaxConcurrency))
		}
		if execution.Policy.MaxTaskTimeoutMs > 0 {
			policyParts = append(policyParts, fmt.Sprintf("max timeout=%dms", execution.Policy.MaxTaskTimeoutMs))
		}
		if execution.Policy.MaxRetryBudget > 0 || len(execution.TaskUnits) > 0 {
			policyParts = append(policyParts, fmt.Sprintf("max retry=%d", execution.Policy.MaxRetryBudget))
		}
		lines = append(lines, strings.Join(policyParts, " · "))
		if ruleName := strings.TrimSpace(execution.Policy.MatchedRuleName); ruleName != "" {
			lines = append(lines, "policy rule: "+ruleName)
		}
		if reason := strings.TrimSpace(execution.Policy.Reason); reason != "" {
			lines = append(lines, "policy reason: "+reason)
		}
		if summary := strings.TrimSpace(execution.Policy.Summary); summary != "" {
			lines = append(lines, "policy summary: "+summary)
		}
		if approvedBy := strings.TrimSpace(execution.Policy.ApprovedBy); approvedBy != "" {
			lines = append(lines, "policy approved by: "+approvedBy)
		}
		if len(execution.Policy.Targets) > 0 {
			lines = append(lines, "policy targets:")
			for _, target := range execution.Policy.Targets {
				hostTarget := firstNonEmpty(strings.TrimSpace(target.HostID), "unknown")
				if hostTarget == "unknown" && len(target.HostLabels) > 0 {
					hostTarget = "labels[" + strings.Join(target.HostLabels, ",") + "]"
				}
				lines = append(lines, fmt.Sprintf(
					"- %s/%s x%d",
					hostTarget,
					firstNonEmpty(strings.TrimSpace(target.AgentID), "unknown"),
					maxInt(target.Count, 1),
				))
			}
		}
		if len(execution.Policy.ToolPolicy.AllowedTools) > 0 {
			lines = append(lines, "allowed tools: "+strings.Join(execution.Policy.ToolPolicy.AllowedTools, ", "))
		}
	}
	if len(execution.Outcome.Artifacts) > 0 {
		lines = append(lines, "artifacts:")
		for _, artifact := range execution.Outcome.Artifacts {
			name := firstNonEmpty(strings.TrimSpace(artifact.Name), strings.TrimSpace(artifact.ID))
			parts := []string{firstNonEmpty(strings.TrimSpace(artifact.ID), "artifact")}
			if taskID := strings.TrimSpace(artifact.TaskID); taskID != "" {
				parts = append(parts, "task="+taskID)
			}
			if kind := strings.TrimSpace(artifact.Kind); kind != "" {
				parts = append(parts, "kind="+kind)
			}
			if artifact.SizeBytes > 0 {
				parts = append(parts, fmt.Sprintf("size=%dB", artifact.SizeBytes))
			}
			lines = append(lines, fmt.Sprintf("- %s · %s", name, strings.Join(parts, " · ")))
		}
	}
	if len(execution.Results) > 0 {
		lines = append(lines, "task results:")
		for _, result := range execution.Results {
			target := strings.TrimSpace(result.AgentID)
			if host := strings.TrimSpace(result.HostID); host != "" {
				target = host + "/" + target
			}
			if strings.TrimSpace(target) == "" {
				target = "(unknown target)"
			}
			summary := strings.TrimSpace(result.Summary)
			if summary == "" {
				summary = strings.TrimSpace(result.Error)
			}
			if summary == "" {
				summary = strings.TrimSpace(result.Output)
			}
			if summary == "" {
				summary = "(no output)"
			}
			lines = append(lines, fmt.Sprintf(
				"- %s [%s] target=%s latency=%dms %s",
				firstNonEmpty(strings.TrimSpace(result.TaskID), "task"),
				firstNonEmpty(strings.TrimSpace(result.Status), "unknown"),
				target,
				result.LatencyMs,
				truncateOrchestrateText(summary, 160),
			))
		}
	}
	if len(resp.Workers) > 0 {
		lines = append(lines, "workers:")
		for _, worker := range resp.Workers {
			lines = append(lines, fmt.Sprintf(
				"- %s/%s state=%s",
				firstNonEmpty(strings.TrimSpace(worker.HostID), "unknown"),
				firstNonEmpty(strings.TrimSpace(worker.AgentID), "unknown"),
				firstNonEmpty(strings.TrimSpace(worker.State), "unknown"),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func renderOrchestrateExecutionArtifacts(executionID string, artifacts []orchestrateArtifactSnapshot) string {
	lines := []string{fmt.Sprintf("execution artifacts %s", firstNonEmpty(strings.TrimSpace(executionID), "(unknown)"))}
	if len(artifacts) == 0 {
		lines = append(lines, "no artifacts recorded")
		return strings.Join(lines, "\n")
	}
	for _, artifact := range artifacts {
		name := firstNonEmpty(strings.TrimSpace(artifact.Name), strings.TrimSpace(artifact.ID))
		parts := []string{firstNonEmpty(strings.TrimSpace(artifact.ID), "artifact")}
		if taskID := strings.TrimSpace(artifact.TaskID); taskID != "" {
			parts = append(parts, "task="+taskID)
		}
		if kind := strings.TrimSpace(artifact.Kind); kind != "" {
			parts = append(parts, "kind="+kind)
		}
		if contentType := strings.TrimSpace(artifact.ContentType); contentType != "" {
			parts = append(parts, contentType)
		}
		if artifact.SizeBytes > 0 {
			parts = append(parts, fmt.Sprintf("size=%dB", artifact.SizeBytes))
		}
		lines = append(lines, fmt.Sprintf("- %s · %s", name, strings.Join(parts, " · ")))
	}
	return strings.Join(lines, "\n")
}

func filterEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func renderOrchestrateExecutionEvidence(executionID string, evidence orchestrateEvidenceBundleSnapshot) string {
	execution := evidence.Execution
	id := firstNonEmpty(strings.TrimSpace(execution.ID), strings.TrimSpace(executionID), "(unknown)")
	lines := []string{fmt.Sprintf("execution evidence %s", id)}
	if goal := strings.TrimSpace(execution.Goal); goal != "" {
		lines = append(lines, "goal: "+goal)
	}
	if generatedAt := strings.TrimSpace(evidence.GeneratedAt); generatedAt != "" {
		lines = append(lines, "generated: "+generatedAt)
	}
	lines = append(lines, fmt.Sprintf("artifacts: %d", len(evidence.ArtifactManifest)))
	lines = append(lines, fmt.Sprintf("audit events: %d", len(evidence.Audit)))
	return strings.Join(lines, "\n")
}

func renderOrchestrateExecutionList(executions []orchestrateExecutionSnapshot) string {
	if len(executions) == 0 {
		return "No orchestration executions found."
	}
	lines := []string{"Orchestration executions:"}
	for _, execution := range executions {
		total, completed, failed := summarizeOrchestrateExecution(execution)
		line := fmt.Sprintf(
			"- %s status=%s tasks=%d completed=%d failed=%d",
			firstNonEmpty(strings.TrimSpace(execution.ID), "unknown"),
			firstNonEmpty(strings.TrimSpace(execution.Status), "unknown"),
			total,
			completed,
			failed,
		)
		if updated := strings.TrimSpace(execution.UpdatedAt); updated != "" {
			line += " updated=" + updated
		}
		lines = append(lines, line)
		if goal := strings.TrimSpace(execution.Goal); goal != "" {
			lines = append(lines, "  goal: "+truncateOrchestrateText(goal, 120))
		}
	}
	return strings.Join(lines, "\n")
}

func renderOrchestratePlan(plan orchestratePlanSnapshot) string {
	lines := []string{
		"orchestration plan",
		fmt.Sprintf("goal: %s", strings.TrimSpace(plan.Goal)),
		fmt.Sprintf("approval: %s", firstNonEmpty(strings.TrimSpace(plan.ApprovalScope), "infrastructure_only")),
		fmt.Sprintf("tasks: %d", len(plan.TaskUnits)),
		fmt.Sprintf("max concurrency: %d", plan.MaxConcurrency),
	}
	if provider := strings.TrimSpace(plan.Provider); provider != "" {
		lines = append(lines, "planner provider: "+provider)
	}
	if len(plan.TaskUnits) > 0 {
		lines = append(lines, "task units:")
		for _, task := range plan.TaskUnits {
			target := strings.TrimSpace(task.AgentID)
			if host := strings.TrimSpace(task.HostID); host != "" {
				target = host + "/" + target
			} else if len(task.HostLabels) > 0 {
				target = "labels[" + strings.Join(task.HostLabels, ",") + "]/" + target
			}
			if target == "" {
				target = "(unassigned)"
			}
			lines = append(lines, fmt.Sprintf(
				"- %s target=%s %s",
				firstNonEmpty(strings.TrimSpace(task.ID), "task"),
				target,
				truncateOrchestrateText(task.Input, 160),
			))
		}
	}
	if len(plan.RequiredWorkers) > 0 {
		lines = append(lines, "required workers:")
		for _, worker := range plan.RequiredWorkers {
			hostTarget := firstNonEmpty(strings.TrimSpace(worker.HostID), orchestratorLocalHostID)
			if strings.TrimSpace(worker.HostID) == "" && len(worker.HostLabels) > 0 {
				hostTarget = "labels[" + strings.Join(worker.HostLabels, ",") + "]"
			}
			lines = append(lines, fmt.Sprintf(
				"- %s/%s x%d",
				hostTarget,
				firstNonEmpty(strings.TrimSpace(worker.AgentID), "zeroclaw"),
				maxInt(worker.Count, 1),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeStringSelectorSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func summarizeOrchestrateExecution(execution orchestrateExecutionSnapshot) (total, completed, failed int) {
	total = len(execution.TaskUnits)
	for _, result := range execution.Results {
		switch strings.ToLower(strings.TrimSpace(result.Status)) {
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}
	if total == 0 {
		total = len(execution.Results)
	}
	return total, completed, failed
}

func truncateOrchestrateText(input string, limit int) string {
	trimmed := strings.TrimSpace(input)
	if limit <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "..."
}

func writePrettyJSON(out io.Writer, raw []byte) error {
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		_, writeErr := fmt.Fprintln(out, strings.TrimSpace(string(raw)))
		return writeErr
	}
	formatted, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(formatted))
	return err
}

func writePrettyJSONValue(out io.Writer, value interface{}) error {
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(formatted))
	return err
}

type remoteHostCheckResponse struct {
	Check struct {
		SSHOK          bool                    `json:"sshOk"`
		OpenClawFound  bool                    `json:"openclawFound"`
		GatewayHealthy bool                    `json:"gatewayHealthy"`
		Platform       remoteHostPlatformCheck `json:"platform"`
		Details        []string                `json:"details"`
	} `json:"check"`
	Instances                []remoteInstanceSummary `json:"instances"`
	PendingPullInstances     []remoteInstanceSummary `json:"pendingPullInstances"`
	PullConfirmationRequired bool                    `json:"pullConfirmationRequired"`
}

type remoteHostPlatformCheck struct {
	OS        string `json:"os"`
	Distro    string `json:"distro"`
	Version   string `json:"version"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
}

type remoteInstancesListResponse struct {
	Instances []remoteInstanceSummary `json:"instances"`
}

type remoteInstanceSummary struct {
	ID      string `json:"id"`
	AgentID string `json:"agentId"`
}

type remoteInstanceStatusSnapshot struct {
	ID           string `json:"id"`
	HostID       string `json:"hostId"`
	AgentID      string `json:"agentId"`
	RuntimeState string `json:"runtimeState"`
	Health       string `json:"health"`
	LastError    string `json:"lastError"`
	UpdatedAt    string `json:"updatedAt"`
}

type remoteInstanceStatusResponse struct {
	Instance remoteInstanceStatusSnapshot `json:"instance"`
}

type remoteInstanceLogsResponse struct {
	Logs string `json:"logs"`
}

type remoteInstanceRollbackSummary struct {
	RolledBack       bool   `json:"rolledBack"`
	FromCommit       string `json:"fromCommit"`
	NewCommit        string `json:"newCommit"`
	DriftState       string `json:"driftState"`
	LastRollbackAt   string `json:"lastRollbackAt"`
	RestoredSnapshot bool   `json:"restoredSnapshot"`
}

type remoteInstanceRollbackResponse struct {
	Rollback remoteInstanceRollbackSummary `json:"rollback"`
}

type remoteInstanceUninstallSummary struct {
	HostID      string `json:"hostId"`
	AgentID     string `json:"agentId"`
	Uninstalled bool   `json:"uninstalled"`
}

type remoteInstanceUninstallResponse struct {
	Uninstall remoteInstanceUninstallSummary `json:"uninstall"`
}

type remoteUploadedKeySummary struct {
	KeyRef      string `json:"keyRef"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	SizeBytes   int    `json:"sizeBytes"`
}

type remoteKeyUploadResponse struct {
	Key remoteUploadedKeySummary `json:"key"`
}

type remoteHostSummary struct {
	ID string `json:"id"`
}

func runRemoteCommand(in io.Reader, out io.Writer, opts remoteCommandOptions) error {
	switch opts.Action {
	case "add":
		return runRemoteAddCommand(in, out, opts)
	case "install":
		return runRemoteBatchInstall(out, opts)
	case "status":
		if opts.All {
			return runRemoteBatchStatus(out, opts)
		}
		return runRemoteStatusCommand(out, opts)
	case "logs":
		return runRemoteLogsCommand(out, opts)
	case "rollback":
		return runRemoteRollbackCommand(out, opts)
	case "uninstall":
		return runRemoteUninstallCommand(out, opts)
	case "key-import":
		return runRemoteKeyImportCommand(out, opts)
	case "key-generate":
		return runRemoteKeyGenerateCommand(out, opts)
	default:
		return fmt.Errorf("unsupported remote action: %s", opts.Action)
	}
}

func runRemoteAddCommand(in io.Reader, out io.Writer, opts remoteCommandOptions) error {
	if opts.Action != "add" {
		return fmt.Errorf("unsupported remote action: %s", opts.Action)
	}
	agentName := remoteAgentDisplayName(opts.AgentID)
	resolvedKeyPath, err := resolveRemoteKeyPath(out, opts.KeyPath)
	if err != nil {
		return err
	}
	opts.KeyPath = resolvedKeyPath
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}

	printRemoteStep(out, 1, 8, fmt.Sprintf("Register or update remote host: %s", opts.HostID))
	hostPayload := map[string]interface{}{
		"id":          opts.HostID,
		"name":        opts.HostName,
		"host":        opts.HostAddr,
		"port":        opts.Port,
		"user":        opts.User,
		"authMode":    opts.AuthMode,
		"keyPath":     resolvedKeyPath,
		"runtimeMode": opts.RuntimeMode,
	}
	if strings.TrimSpace(opts.KeyPath) != "" {
		hostPayload["keyPath"] = opts.KeyPath
	}
	if strings.TrimSpace(opts.KeyRef) != "" {
		hostPayload["keyRef"] = opts.KeyRef
	}
	if strings.TrimSpace(opts.SSHConfigHost) != "" {
		hostPayload["sshConfigHost"] = opts.SSHConfigHost
	}
	if _, _, err := gatewayRequestWithTimeout(http.MethodPost, "/api/v1/remote/hosts", hostPayload, 30*time.Second); err != nil {
		return err
	}

	printRemoteStep(out, 2, 8, "Verify remote host connectivity")
	preCheck, err := runRemoteHostCheckWithRetry(opts.HostID, opts.CheckRetries, opts.CheckRetryDelaySec, false, nil)
	if err != nil {
		return err
	}
	printRemoteCheckSummary(out, preCheck, "")
	if preCheck.Check.OpenClawFound {
		_, _ = fmt.Fprintf(out, "  Existing %s runtime detected; upgrade-safe install will run.\n", agentName)
	} else {
		_, _ = fmt.Fprintf(out, "  %s runtime not detected; fresh install will run.\n", agentName)
	}
	hadTargetBeforeInstall := remoteInstanceListHasAgent(preCheck.Instances, opts.InstallAgentID)
	installCompleted := false
	failWithRollback := func(stage string, cause error) error {
		if !installCompleted || !opts.AutoRollback {
			return fmt.Errorf("remote add failed at %s: %w", stage, cause)
		}
		if rollbackErr := attemptRemoteAddRollback(out, opts, hadTargetBeforeInstall); rollbackErr != nil {
			return fmt.Errorf("remote add failed at %s: %w (automatic rollback failed: %v)", stage, cause, rollbackErr)
		}
		return fmt.Errorf("remote add failed at %s: %w (automatic rollback succeeded)", stage, cause)
	}

	printRemoteStep(out, 3, 8, fmt.Sprintf("Install %s on remote host", agentName))
	if err := runRemoteInstallStream(out, opts.HostID, opts.InstallAgentID, opts.Isolation); err != nil {
		return failWithRollback("install", err)
	}
	installCompleted = true
	_, _ = fmt.Fprintf(out, "  %s installation stream completed successfully.\n", agentName)

	if len(opts.SyncChannels) > 0 || len(opts.SyncProviders) > 0 {
		printRemoteStep(out, 4, 8, "Sync selected local configuration")
		if err := runRemoteSelectedConfigSync(opts); err != nil {
			return failWithRollback("config sync", err)
		}
		_, _ = fmt.Fprintln(out, "  Local configuration sync completed.")
	} else {
		printRemoteStep(out, 4, 8, "Sync selected local configuration")
		_, _ = fmt.Fprintln(out, "  Skipped (no --sync-channel/--sync-provider provided).")
	}

	printRemoteStep(out, 5, 8, "Post-install health check")
	postCheck, err := runRemoteHostCheckWithRetry(opts.HostID, opts.CheckRetries, opts.CheckRetryDelaySec, false, nil)
	if err != nil {
		return failWithRollback("post-install health check", err)
	}
	postCheck, err = maybeConfirmPullForPendingInstances(in, out, opts.HostID, postCheck)
	if err != nil {
		return failWithRollback("post-install pull confirmation", err)
	}
	printRemoteCheckSummary(out, postCheck, "")

	printRemoteStep(out, 6, 8, "List remote instances")
	if err := printRemoteInstances(out, opts.HostID, "Detected instances"); err != nil {
		return failWithRollback("list remote instances", err)
	}

	if opts.SkipReconnectCheck {
		printRemoteStep(out, 7, 8, "Reconnect simulation skipped (--skip-reconnect-check)")
		printRemoteStep(out, 8, 8, "Reconnect verification skipped (--skip-reconnect-check)")
		_, _ = fmt.Fprintln(out, colorizeForTTY(out, fmt.Sprintf("Completed: %s remote install finished for host %s.", agentName, opts.HostID), ansiGreenBold))
		return nil
	}

	printRemoteStep(out, 7, 8, "Reconnect simulation (remove host record and re-register)")
	if _, _, err := gatewayRequestWithTimeout(http.MethodDelete, "/api/v1/remote/hosts/"+neturl.PathEscape(opts.HostID), nil, 30*time.Second); err != nil {
		return failWithRollback("reconnect simulation delete host", err)
	}
	if _, _, err := gatewayRequestWithTimeout(http.MethodPost, "/api/v1/remote/hosts", hostPayload, 30*time.Second); err != nil {
		return failWithRollback("reconnect simulation upsert host", err)
	}

	printRemoteStep(out, 8, 8, "Reconnect verification and instance refresh")
	reconnectCheck, err := runRemoteHostCheckWithRetry(opts.HostID, opts.CheckRetries, opts.CheckRetryDelaySec, false, nil)
	if err != nil {
		return failWithRollback("reconnect verification check", err)
	}
	reconnectCheck, err = maybeConfirmPullForPendingInstances(in, out, opts.HostID, reconnectCheck)
	if err != nil {
		return failWithRollback("reconnect pull confirmation", err)
	}
	printRemoteCheckSummary(out, reconnectCheck, "Reconnect verification")
	if err := printRemoteInstances(out, opts.HostID, "Instances after reconnect"); err != nil {
		return failWithRollback("list instances after reconnect", err)
	}

	_, _ = fmt.Fprintln(out, colorizeForTTY(out, fmt.Sprintf("Completed: %s is installed on host %s and reconnect verification passed.", agentName, opts.HostID), ansiGreenBold))
	return nil
}

func remoteInstanceListHasAgent(instances []remoteInstanceSummary, agentID string) bool {
	target := strings.ToLower(strings.TrimSpace(agentID))
	if target == "" {
		return false
	}
	for _, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.AgentID), target) {
			return true
		}
	}
	return false
}

func attemptRemoteAddRollback(out io.Writer, opts remoteCommandOptions, hadTargetBeforeInstall bool) error {
	if hadTargetBeforeInstall {
		_, _ = fmt.Fprintln(out, "  Auto-rollback: existing instance detected before install, attempting rollback to latest known baseline.")
		if _, err := runRemoteInstanceRollback(opts.HostID, opts.InstallAgentID, ""); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "  Auto-rollback: rollback completed.")
		return nil
	}
	_, _ = fmt.Fprintln(out, "  Auto-rollback: fresh install detected, attempting uninstall cleanup.")
	if _, err := runRemoteInstanceUninstall(opts.HostID, opts.InstallAgentID); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "  Auto-rollback: uninstall cleanup completed.")
	return nil
}

func runRemoteStatusCommand(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	status, err := runRemoteInstanceStatus(opts.HostID, opts.TargetAgentID)
	if err != nil {
		return err
	}
	runtimeState := strings.TrimSpace(status.RuntimeState)
	if runtimeState == "" {
		runtimeState = "unknown"
	}
	health := strings.TrimSpace(status.Health)
	if health == "" {
		health = "unknown"
	}
	_, _ = fmt.Fprintf(out, "Remote instance %s (%s)\n", strings.TrimSpace(status.ID), strings.TrimSpace(status.AgentID))
	_, _ = fmt.Fprintf(out, "  runtime=%s health=%s\n", runtimeState, health)
	if lastErr := strings.TrimSpace(status.LastError); lastErr != "" {
		_, _ = fmt.Fprintf(out, "  lastError=%s\n", lastErr)
	}
	if updatedAt := strings.TrimSpace(status.UpdatedAt); updatedAt != "" {
		_, _ = fmt.Fprintf(out, "  updatedAt=%s\n", updatedAt)
	}
	return nil
}

func runRemoteLogsCommand(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	logs, err := runRemoteInstanceLogs(opts.HostID, opts.TargetAgentID, opts.Tail)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(logs)
	if trimmed == "" {
		_, _ = fmt.Fprintln(out, "No remote logs returned.")
		return nil
	}
	_, _ = fmt.Fprintln(out, trimmed)
	return nil
}

func runRemoteRollbackCommand(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	rollback, err := runRemoteInstanceRollback(opts.HostID, opts.TargetAgentID, opts.Commit)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Rollback completed for %s:%s\n", strings.TrimSpace(opts.HostID), strings.TrimSpace(opts.TargetAgentID))
	_, _ = fmt.Fprintf(out, "  rolledBack=%t drift=%s\n", rollback.RolledBack, strings.TrimSpace(rollback.DriftState))
	if fromCommit := strings.TrimSpace(rollback.FromCommit); fromCommit != "" {
		_, _ = fmt.Fprintf(out, "  fromCommit=%s\n", fromCommit)
	}
	if newCommit := strings.TrimSpace(rollback.NewCommit); newCommit != "" {
		_, _ = fmt.Fprintf(out, "  newCommit=%s\n", newCommit)
	}
	return nil
}

func runRemoteUninstallCommand(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	uninstall, err := runRemoteInstanceUninstall(opts.HostID, opts.TargetAgentID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Remote uninstall completed for %s:%s (uninstalled=%t)\n",
		strings.TrimSpace(uninstall.HostID),
		strings.TrimSpace(uninstall.AgentID),
		uninstall.Uninstalled,
	)
	return nil
}

func runRemoteKeyImportCommand(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	key, err := runRemoteKeyImportPath(opts.KeyImportPath)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Remote key uploaded: keyRef=%s fingerprint=%s size=%d\n",
		strings.TrimSpace(key.KeyRef),
		strings.TrimSpace(key.Fingerprint),
		key.SizeBytes,
	)
	return nil
}

func runRemoteKeyGenerateCommand(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	outputPath := strings.TrimSpace(opts.KeyOutputPath)
	if outputPath == "" {
		defaultPath, err := defaultGeneratedKeyOutputPath(opts.KeyType)
		if err != nil {
			return err
		}
		outputPath = defaultPath
	}
	outputPath = filepath.Clean(outputPath)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("create key output directory: %w", err)
	}
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("key output path already exists: %s", outputPath)
	}
	keygenArgs := []string{
		"-t", opts.KeyType,
		"-N", "",
		"-C", "carrier-generated",
		"-f", outputPath,
	}
	if opts.KeyType == "rsa" {
		keygenArgs = []string{
			"-t", "rsa",
			"-b", "4096",
			"-N", "",
			"-C", "carrier-generated",
			"-f", outputPath,
		}
	}
	cmd := exec.Command("ssh-keygen", keygenArgs...)
	if raw, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("generate ssh key: %w (%s)", err, strings.TrimSpace(string(raw)))
	}

	key, err := runRemoteKeyImportPath(outputPath)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Generated keypair: private=%s public=%s\n", outputPath, outputPath+".pub")
	_, _ = fmt.Fprintf(out, "Uploaded private key: keyRef=%s fingerprint=%s\n",
		strings.TrimSpace(key.KeyRef),
		strings.TrimSpace(key.Fingerprint),
	)
	return nil
}

func defaultGeneratedKeyOutputPath(keyType string) (string, error) {
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for key generation: %w", err)
	}
	ext := "id_ed25519"
	if strings.EqualFold(strings.TrimSpace(keyType), "rsa") {
		ext = "id_rsa"
	}
	dir := filepath.Join(home, ".carrier", "keys", "generated")
	return filepath.Join(dir, ext), nil
}

func runRemoteKeyImportPath(path string) (remoteUploadedKeySummary, error) {
	source := strings.TrimSpace(path)
	raw, err := os.ReadFile(source)
	if err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("read key file: %w", err)
	}
	if len(raw) == 0 {
		return remoteUploadedKeySummary{}, errors.New("key file is empty")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(source))
	if err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("create multipart key form: %w", err)
	}
	if _, err := part.Write(raw); err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("write multipart key payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("finalize multipart payload: %w", err)
	}

	base := strings.TrimRight(strings.TrimSpace(gatewayProbeBaseURL()), "/")
	if base == "" {
		return remoteUploadedKeySummary{}, errors.New("gateway base url is empty")
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/remote/keys", body)
	if err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("build key upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addGatewayAuthHeader(req)
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("key upload request failed: %w", err)
	}
	defer resp.Body.Close()
	respRaw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("read key upload response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return remoteUploadedKeySummary{}, fmt.Errorf("key upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}
	var payload remoteKeyUploadResponse
	if err := json.Unmarshal(respRaw, &payload); err != nil {
		return remoteUploadedKeySummary{}, fmt.Errorf("decode key upload response: %w", err)
	}
	if strings.TrimSpace(payload.Key.KeyRef) == "" {
		return remoteUploadedKeySummary{}, errors.New("key upload response missing keyRef")
	}
	return payload.Key, nil
}

func runRemoteInstanceStatus(hostID, agentID string) (remoteInstanceStatusSnapshot, error) {
	path := fmt.Sprintf("/api/v1/remote/hosts/%s/instances/%s/status",
		neturl.PathEscape(strings.TrimSpace(hostID)),
		neturl.PathEscape(strings.TrimSpace(agentID)),
	)
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return remoteInstanceStatusSnapshot{}, err
	}
	var payload remoteInstanceStatusResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return remoteInstanceStatusSnapshot{}, fmt.Errorf("decode remote status response: %w", err)
	}
	return payload.Instance, nil
}

func runRemoteInstanceLogs(hostID, agentID string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	path := fmt.Sprintf("/api/v1/remote/hosts/%s/instances/%s/logs?tail=%d",
		neturl.PathEscape(strings.TrimSpace(hostID)),
		neturl.PathEscape(strings.TrimSpace(agentID)),
		tail,
	)
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, path, nil, 45*time.Second)
	if err != nil {
		return "", err
	}
	var payload remoteInstanceLogsResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("decode remote logs response: %w", err)
	}
	return payload.Logs, nil
}

func runRemoteInstanceRollback(hostID, agentID, commit string) (remoteInstanceRollbackSummary, error) {
	path := fmt.Sprintf("/api/v1/remote/hosts/%s/instances/%s/rollback",
		neturl.PathEscape(strings.TrimSpace(hostID)),
		neturl.PathEscape(strings.TrimSpace(agentID)),
	)
	body := map[string]interface{}{}
	if trimmed := strings.TrimSpace(commit); trimmed != "" {
		body["commit"] = trimmed
	}
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, path, body, 60*time.Second)
	if err != nil {
		return remoteInstanceRollbackSummary{}, err
	}
	var payload remoteInstanceRollbackResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return remoteInstanceRollbackSummary{}, fmt.Errorf("decode remote rollback response: %w", err)
	}
	return payload.Rollback, nil
}

func runRemoteInstanceUninstall(hostID, agentID string) (remoteInstanceUninstallSummary, error) {
	path := fmt.Sprintf("/api/v1/remote/hosts/%s/instances/%s/uninstall",
		neturl.PathEscape(strings.TrimSpace(hostID)),
		neturl.PathEscape(strings.TrimSpace(agentID)),
	)
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, path, map[string]interface{}{}, 60*time.Second)
	if err != nil {
		return remoteInstanceUninstallSummary{}, err
	}
	var payload remoteInstanceUninstallResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return remoteInstanceUninstallSummary{}, fmt.Errorf("decode remote uninstall response: %w", err)
	}
	return payload.Uninstall, nil
}

func runRemoteBatchInstall(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	hosts, err := remoteHostsLister()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		_, _ = fmt.Fprintln(out, "no remote hosts registered")
		return nil
	}
	sem := make(chan struct{}, maxInt(1, opts.Concurrency))
	type result struct {
		hostID string
		err    error
	}
	results := make(chan result, len(hosts))
	launched := 0
	for _, host := range hosts {
		hostID := strings.TrimSpace(host.ID)
		if hostID == "" {
			continue
		}
		_, _ = fmt.Fprintf(out, "%s pending\n", hostID)
		go func(id string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			_, _ = fmt.Fprintf(out, "%s installing\n", id)
			err := remoteInstallStreamer(out, id, opts.InstallAgentID, opts.Isolation)
			results <- result{hostID: id, err: err}
		}(hostID)
		launched++
	}
	if launched == 0 {
		_, _ = fmt.Fprintln(out, "no eligible remote hosts found")
		return nil
	}
	var firstErr error
	for i := 0; i < launched; i++ {
		r := <-results
		if r.err != nil {
			_, _ = fmt.Fprintf(out, "%s failed: %v\n", r.hostID, r.err)
			if firstErr == nil {
				firstErr = r.err
			}
		} else {
			_, _ = fmt.Fprintf(out, "%s done\n", r.hostID)
		}
	}
	return firstErr
}

func runRemoteBatchStatus(out io.Writer, opts remoteCommandOptions) error {
	if _, err := ensureGatewayRunning(out, startGatewayInBackgroundAndWait); err != nil {
		return err
	}
	hosts, err := remoteHostsLister()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		_, _ = fmt.Fprintln(out, "no remote hosts registered")
		return nil
	}
	_, _ = fmt.Fprintln(out, "host\tssh\topenclaw")
	for _, host := range hosts {
		check, err := remoteHostChecker(strings.TrimSpace(host.ID), false, nil)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s\tfailed\tfailed\n", host.ID)
			continue
		}
		ssh := "down"
		if check.Check.SSHOK {
			ssh = "ok"
		}
		runtime := "missing"
		if check.Check.OpenClawFound {
			runtime = "detected"
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", host.ID, ssh, runtime)
	}
	return nil
}

func listRemoteHosts() ([]remoteHostSummary, error) {
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, "/api/v1/remote/hosts", nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Hosts []remoteHostSummary `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload.Hosts, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func printRemoteInstances(out io.Writer, hostID, label string) error {
	raw, _, err := gatewayRequestWithTimeout(http.MethodGet, "/api/v1/remote/hosts/"+neturl.PathEscape(strings.TrimSpace(hostID))+"/instances", nil, 30*time.Second)
	if err != nil {
		return err
	}
	var payload remoteInstancesListResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode instances response: %w", err)
	}
	names := summarizeRemoteInstanceNames(payload.Instances)
	if len(names) == 0 {
		_, _ = fmt.Fprintf(out, "  %s: none\n", strings.TrimSpace(label))
		return nil
	}
	_, _ = fmt.Fprintf(out, "  %s: %s\n", strings.TrimSpace(label), strings.Join(names, ", "))
	return nil
}

func runRemoteHostCheckWithRetry(hostID string, retries int, delaySec int, pullNew bool, pullAgentIDs []string) (remoteHostCheckResponse, error) {
	if retries < 0 {
		retries = 0
	}
	if delaySec < 0 {
		delaySec = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		resp, err := runRemoteHostCheck(hostID, pullNew, pullAgentIDs)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt == retries {
			break
		}
		if delaySec > 0 {
			time.Sleep(time.Duration(delaySec) * time.Second)
		}
	}
	return remoteHostCheckResponse{}, lastErr
}

func runRemoteHostCheck(hostID string, pullNew bool, pullAgentIDs []string) (remoteHostCheckResponse, error) {
	payload := map[string]interface{}{}
	if pullNew {
		payload["pullNewInstances"] = true
	}
	ids := make([]string, 0, len(pullAgentIDs))
	for _, id := range pullAgentIDs {
		trimmed := strings.ToLower(strings.TrimSpace(id))
		if trimmed == "" {
			continue
		}
		ids = append(ids, trimmed)
	}
	if len(ids) > 0 {
		payload["pullAgentIds"] = ids
	}
	raw, _, err := gatewayRequestWithTimeout(http.MethodPost, "/api/v1/remote/hosts/"+neturl.PathEscape(strings.TrimSpace(hostID))+"/check", payload, 45*time.Second)
	if err != nil {
		return remoteHostCheckResponse{}, err
	}
	var resp remoteHostCheckResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return remoteHostCheckResponse{}, fmt.Errorf("decode host check response: %w", err)
	}
	return resp, nil
}

func maybeConfirmPullForPendingInstances(in io.Reader, out io.Writer, hostID string, checkResp remoteHostCheckResponse) (remoteHostCheckResponse, error) {
	if !checkResp.PullConfirmationRequired || len(checkResp.PendingPullInstances) == 0 {
		return checkResp, nil
	}
	pendingIDs := summarizeRemoteInstanceNames(checkResp.PendingPullInstances)
	if len(pendingIDs) == 0 {
		return checkResp, nil
	}
	_, _ = fmt.Fprintf(out, "  Discovered %d previously unknown remote instance(s): %s\n", len(pendingIDs), strings.Join(pendingIDs, ", "))
	if !isInteractiveReader(in) {
		_, _ = fmt.Fprintln(out, "  Non-interactive input detected; skipping import of newly discovered remote configs.")
		return checkResp, nil
	}
	reader := bufio.NewReader(in)
	confirm, err := promptYesNo(reader, out, "Import these remote instance configs into local Carrier profile store?", false)
	if err != nil {
		return checkResp, err
	}
	if !confirm {
		_, _ = fmt.Fprintln(out, "  Skipped importing newly discovered remote instance configs.")
		return checkResp, nil
	}
	confirmed, err := runRemoteHostCheck(hostID, true, pendingIDs)
	if err != nil {
		return checkResp, err
	}
	_, _ = fmt.Fprintf(out, "  Imported %d remote instance config(s) into local Carrier profile store.\n", len(pendingIDs))
	return confirmed, nil
}

func isInteractiveReader(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func listWindowsWSLDistros() ([]string, error) {
	cmd := exec.Command("wsl.exe", "-l", "-q")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			return nil, fmt.Errorf("wsl -l -q: %w", err)
		}
		return nil, fmt.Errorf("wsl -l -q: %w (%s)", err, msg)
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r", ""), "\n")
	distros := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.ReplaceAll(line, "\x00", ""))
		if trimmed == "" {
			continue
		}
		distros = append(distros, trimmed)
	}
	return distros, nil
}

func ensureWindowsWSLPrereqForOpenClaw(agentID string) error {
	if !strings.EqualFold(strings.TrimSpace(agentID), "openclaw") {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(carrierGOOS), "windows") {
		return nil
	}

	distros, err := windowsWSLListDistros()
	if err != nil {
		return fmt.Errorf("openclaw on Windows requires WSL2; failed to query WSL distros: %w", err)
	}
	if len(distros) == 0 {
		return errors.New("openclaw on Windows requires WSL2 with at least one Linux distro installed. Run `wsl --install -d Ubuntu` and retry")
	}
	return nil
}

func runRemoteInstallStream(out io.Writer, hostID, agentID string, isolation bool) error {
	base := strings.TrimRight(strings.TrimSpace(gatewayProbeBaseURL()), "/")
	if base == "" {
		return errors.New("gateway base url is empty")
	}
	path := fmt.Sprintf("%s/api/v1/remote/hosts/%s/instances/%s/install/stream", base, neturl.PathEscape(strings.TrimSpace(hostID)), neturl.PathEscape(strings.TrimSpace(agentID)))
	rawPayload, marshalErr := json.Marshal(map[string]bool{
		"isolation": isolation,
	})
	if marshalErr != nil {
		return fmt.Errorf("marshal install stream payload: %w", marshalErr)
	}
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(rawPayload))
	if err != nil {
		return fmt.Errorf("build install stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	addGatewayAuthHeader(req)

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("remote install stream request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if readErr != nil {
			return fmt.Errorf("remote install stream failed with status %d", resp.StatusCode)
		}
		return fmt.Errorf("remote install stream failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	installed := false
	emitted := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		eventRaw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if eventRaw == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(eventRaw), &payload); err != nil {
			continue
		}
		eventType := strings.ToLower(strings.TrimSpace(anyToString(payload["type"])))
		switch eventType {
		case "log":
			stream := strings.TrimSpace(anyToString(payload["stream"]))
			logLine := strings.TrimSpace(anyToString(payload["line"]))
			if logLine == "" {
				continue
			}
			message, warning, ok := formatRemoteInstallLogLine(stream, logLine)
			if !ok {
				continue
			}
			if emitted[message] {
				continue
			}
			emitted[message] = true
			if warning {
				_, _ = fmt.Fprintf(out, "  %s\n", colorizeForTTY(out, "Warning: "+message, ansiYellow))
			} else {
				style := ansiCyan
				lowerMessage := strings.ToLower(strings.TrimSpace(message))
				if strings.Contains(lowerMessage, "installed successfully") || strings.Contains(lowerMessage, "upgrade completed") {
					style = ansiGreen
				}
				_, _ = fmt.Fprintf(out, "  %s\n", colorizeForTTY(out, message, style))
			}
		case "error":
			message := strings.TrimSpace(anyToString(payload["message"]))
			if message == "" {
				message = "remote install stream returned error"
			}
			return errors.New(message)
		case "result":
			installObj, _ := payload["install"].(map[string]interface{})
			installed = anyToBool(installObj["installed"])
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read install stream: %w", err)
	}
	if !installed {
		return errors.New("remote install stream finished without installed=true")
	}
	return nil
}

const (
	ansiReset     = "\033[0m"
	ansiCyanBold  = "\033[1;36m"
	ansiCyan      = "\033[36m"
	ansiGreen     = "\033[32m"
	ansiGreenBold = "\033[1;32m"
	ansiYellow    = "\033[33m"
)

func printRemoteStep(out io.Writer, index, total int, message string) {
	line := fmt.Sprintf("[%d/%d] %s", index, total, strings.TrimSpace(message))
	_, _ = fmt.Fprintln(out, colorizeForTTY(out, line, ansiCyanBold))
}

func colorizeForTTY(out io.Writer, text, ansiCode string) string {
	if !supportsANSIColor(out) || strings.TrimSpace(ansiCode) == "" {
		return text
	}
	return ansiCode + text + ansiReset
}

func supportsANSIColor(out io.Writer) bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func remoteAgentDisplayName(agentID string) string {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "openclaw":
		return "OpenClaw"
	case "picoclaw":
		return "PicoClaw"
	case "zeroclaw":
		return "ZeroClaw"
	default:
		return strings.TrimSpace(agentID)
	}
}

func printRemoteCheckSummary(out io.Writer, check remoteHostCheckResponse, label string) {
	label = strings.TrimSpace(label)
	if label != "" {
		_, _ = fmt.Fprintf(out, "  %s:\n", label)
	}
	if check.Check.SSHOK {
		_, _ = fmt.Fprintln(out, "  SSH connectivity: OK.")
	} else {
		_, _ = fmt.Fprintln(out, "  SSH connectivity: FAILED.")
	}
	if check.Check.OpenClawFound {
		_, _ = fmt.Fprintln(out, "  OpenClaw runtime: detected.")
	} else {
		_, _ = fmt.Fprintln(out, "  OpenClaw runtime: not detected.")
	}
	gatewayHealth := "unknown"
	if check.Check.GatewayHealthy {
		gatewayHealth = "healthy"
	} else if check.Check.OpenClawFound {
		gatewayHealth = "unhealthy"
	}
	_, _ = fmt.Fprintf(out, "  Remote gateway health: %s.\n", gatewayHealth)

	platform := check.Check.Platform
	platformParts := []string{}
	if osPart := strings.TrimSpace(platform.OS); osPart != "" {
		platformParts = append(platformParts, osPart)
	}
	if distroPart := strings.TrimSpace(platform.Distro); distroPart != "" {
		platformParts = append(platformParts, distroPart)
	}
	platformLabel := strings.Join(platformParts, "/")
	if version := strings.TrimSpace(platform.Version); version != "" {
		if platformLabel == "" {
			platformLabel = version
		} else {
			platformLabel += " " + version
		}
	}
	if platformLabel != "" {
		support := "supported"
		if !platform.Supported {
			support = "unsupported"
		}
		_, _ = fmt.Fprintf(out, "  Remote platform: %s (%s).\n", platformLabel, support)
	}
	if reason := strings.TrimSpace(platform.Reason); reason != "" {
		_, _ = fmt.Fprintf(out, "  Platform note: %s\n", reason)
	}
	for _, detail := range check.Check.Details {
		trimmed := strings.TrimSpace(detail)
		if trimmed == "" {
			continue
		}
		_, _ = fmt.Fprintf(out, "  Detail: %s\n", trimmed)
	}
}

func summarizeRemoteInstanceNames(instances []remoteInstanceSummary) []string {
	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		agentID := strings.ToLower(strings.TrimSpace(inst.AgentID))
		if agentID == "" {
			id := strings.TrimSpace(inst.ID)
			if idx := strings.LastIndex(id, ":"); idx >= 0 && idx+1 < len(id) {
				agentID = strings.ToLower(strings.TrimSpace(id[idx+1:]))
			} else {
				agentID = strings.ToLower(strings.TrimSpace(id))
			}
		}
		if agentID == "" {
			continue
		}
		names = append(names, agentID)
	}
	return dedupeLowerStrings(names)
}

func formatRemoteInstallLogLine(stream, line string) (string, bool, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false, false
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "existing openclaw installation detected"):
		return "Existing OpenClaw installation found; upgrading in place.", false, true
	case strings.Contains(lower, "installing openclaw v"):
		return trimmed, false, true
	case strings.Contains(lower, "openclaw installed successfully"):
		return trimmed, false, true
	case strings.Contains(lower, "upgrade complete"):
		return "Upgrade completed.", false, true
	case strings.Contains(lower, "doctor failed; skipping plugin updates"):
		return "Doctor step reported a non-blocking issue; plugin updates were skipped.", true, true
	case strings.HasPrefix(lower, "dashboard url:"):
		return trimmed, false, true
	case strings.Contains(lower, "setlocale:"):
		return "Remote locale warning (non-blocking): " + trimmed, true, true
	case strings.EqualFold(strings.TrimSpace(stream), "stderr") && strings.Contains(lower, "warning"):
		return "Remote warning: " + trimmed, true, true
	default:
		return "", false, false
	}
}

func runRemoteSelectedConfigSync(opts remoteCommandOptions) error {
	cfg, _, err := configv2.Load()
	if err != nil {
		return fmt.Errorf("load local config.v2: %w", err)
	}
	if cfg == nil {
		return errors.New("local config.v2 is not available")
	}

	patch, err := buildRemoteConfigPatch(opts, cfg)
	if err != nil {
		return err
	}
	if len(patch) == 0 {
		return nil
	}
	path := fmt.Sprintf("/api/v1/remote/hosts/%s/instances/%s/config", neturl.PathEscape(strings.TrimSpace(opts.HostID)), neturl.PathEscape(strings.TrimSpace(opts.InstallAgentID)))
	body := map[string]interface{}{"patch": patch}
	_, _, err = gatewayRequestWithTimeout(http.MethodPatch, path, body, 90*time.Second)
	return err
}

func buildRemoteConfigPatch(opts remoteCommandOptions, cfg *configv2.Config) (map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(opts.AgentID)) {
	case "zeroclaw":
		return buildZeroClawRemoteConfigPatch(opts, cfg)
	case "picoclaw":
		return buildPicoClawRemoteConfigPatch(opts, cfg)
	default:
		return buildOpenClawRemoteConfigPatch(opts, cfg)
	}
}

func buildJSONRemoteConfigPatch(opts remoteCommandOptions, cfg *configv2.Config) (map[string]interface{}, error) {
	return buildPicoClawRemoteConfigPatch(opts, cfg)
}

func buildPicoClawRemoteConfigPatch(opts remoteCommandOptions, cfg *configv2.Config) (map[string]interface{}, error) {
	patch := map[string]interface{}{}
	if len(opts.SyncChannels) > 0 {
		channelPatch := map[string]interface{}{}
		for _, selected := range dedupeLowerStrings(opts.SyncChannels) {
			local, ok := findLocalChannel(cfg, selected)
			if !ok {
				return nil, fmt.Errorf("local channel %q is not configured", selected)
			}
			token := strings.TrimSpace(local.BotToken)
			if token == "" {
				return nil, fmt.Errorf("local channel %q has empty bot_token", selected)
			}
			entry := map[string]interface{}{
				"enabled": true,
				"token":   token,
			}
			if secret := strings.TrimSpace(local.WebhookSecret); secret != "" {
				entry["webhook_secret"] = secret
			}
			allowFrom := make([]string, 0)
			switch selected {
			case "telegram":
				if len(opts.TelegramAllowFrom) > 0 {
					allowFrom = append(allowFrom, dedupeTrimmedStrings(opts.TelegramAllowFrom)...)
				}
			case "discord":
				if len(opts.DiscordAllowFrom) > 0 {
					allowFrom = append(allowFrom, dedupeTrimmedStrings(opts.DiscordAllowFrom)...)
				}
			}
			if len(allowFrom) == 0 {
				allowFrom = dedupeTrimmedStrings(local.AllowFrom)
			}
			if len(allowFrom) > 0 {
				entry["allow_from"] = allowFrom
			}
			channelPatch[selected] = entry
		}
		patch["channels"] = channelPatch
	}

	if len(opts.SyncProviders) > 0 {
		defaultModelName := strings.TrimSpace(cfg.DefaultModel)
		defaultProviderKey := ""
		defaultManagedModel := ""
		modelList := make([]interface{}, 0, len(opts.SyncProviders))
		providers := map[string]interface{}{}

		for _, requestedProvider := range dedupeLowerStrings(opts.SyncProviders) {
			model, ok := resolveLocalModelForProvider(cfg, requestedProvider)
			if !ok {
				return nil, fmt.Errorf("local provider %q is not found in model_list", requestedProvider)
			}
			managedProviderKey, managedModelName, managedModelID, providerItem, modelItem, err := buildManagedProviderAndModelEntry(model)
			if err != nil {
				return nil, err
			}
			providers[managedProviderKey] = providerItem
			modelList = append(modelList, modelItem)
			if defaultProviderKey == "" {
				defaultProviderKey = managedProviderKey
				defaultManagedModel = managedModelName
			}
			if defaultModelName != "" && (strings.EqualFold(defaultModelName, strings.TrimSpace(model.ModelName)) || strings.EqualFold(defaultModelName, strings.TrimSpace(model.Model))) {
				defaultProviderKey = managedProviderKey
				defaultManagedModel = managedModelName
			}
			_ = managedModelID
		}

		patch["providers"] = providers
		patch["model_list"] = modelList
		if defaultProviderKey != "" && defaultManagedModel != "" {
			patch["agents"] = map[string]interface{}{
				"defaults": map[string]interface{}{
					"provider": defaultProviderKey,
					"model":    defaultManagedModel,
				},
			}
		}
	}
	return patch, nil
}

func buildOpenClawRemoteConfigPatch(opts remoteCommandOptions, cfg *configv2.Config) (map[string]interface{}, error) {
	patch := map[string]interface{}{}
	if len(opts.SyncChannels) > 0 {
		channelPatch := map[string]interface{}{}
		for _, selected := range dedupeLowerStrings(opts.SyncChannels) {
			local, ok := findLocalChannel(cfg, selected)
			if !ok {
				return nil, fmt.Errorf("local channel %q is not configured", selected)
			}
			token := strings.TrimSpace(local.BotToken)
			if token == "" {
				return nil, fmt.Errorf("local channel %q has empty bot_token", selected)
			}
			entry := map[string]interface{}{
				"enabled": true,
			}
			entry[openclawcfg.ChannelTokenField(selected)] = token
			if secret := strings.TrimSpace(local.WebhookSecret); secret != "" {
				entry["webhookSecret"] = secret
			}
			allowFrom := make([]string, 0)
			switch selected {
			case "telegram":
				if len(opts.TelegramAllowFrom) > 0 {
					allowFrom = append(allowFrom, dedupeTrimmedStrings(opts.TelegramAllowFrom)...)
				}
			case "discord":
				if len(opts.DiscordAllowFrom) > 0 {
					allowFrom = append(allowFrom, dedupeTrimmedStrings(opts.DiscordAllowFrom)...)
				}
			}
			if len(allowFrom) == 0 {
				allowFrom = dedupeTrimmedStrings(local.AllowFrom)
			}
			if len(allowFrom) > 0 {
				entry["allowFrom"] = allowFrom
			}
			channelPatch[selected] = entry
		}
		patch["channels"] = channelPatch
	}

	if len(opts.SyncProviders) > 0 {
		defaultModelName := strings.TrimSpace(cfg.DefaultModel)
		modelsProviders := map[string]interface{}{}
		secretProviders := map[string]interface{}{
			openclawcfg.CarrierFileSecretProviderAlias: map[string]interface{}{
				"source": "file",
				"path":   openclawcfg.CarrierFileSecretsPath,
				"mode":   "json",
			},
		}
		secretsFilePayload := map[string]interface{}{}
		defaultModelID := ""

		for _, requestedProvider := range dedupeLowerStrings(opts.SyncProviders) {
			model, ok := resolveLocalModelForProvider(cfg, requestedProvider)
			if !ok {
				return nil, fmt.Errorf("local provider %q is not found in model_list", requestedProvider)
			}
			providerKey, modelName, modelID, providerItem, secretValue, err := buildOpenClawProviderEntry(model)
			if err != nil {
				return nil, err
			}
			modelsProviders[providerKey] = providerItem
			if secretValue != "" {
				secretsFilePayload[providerKey] = map[string]interface{}{
					"apiKey": secretValue,
				}
			}
			if defaultModelID == "" {
				defaultModelID = modelID
			}
			if defaultModelName != "" && (strings.EqualFold(defaultModelName, strings.TrimSpace(model.ModelName)) || strings.EqualFold(defaultModelName, strings.TrimSpace(model.Model)) || strings.EqualFold(defaultModelName, modelName)) {
				defaultModelID = modelID
			}
		}

		if len(modelsProviders) > 0 {
			patch["models"] = map[string]interface{}{
				"providers": modelsProviders,
			}
			patch["secrets"] = map[string]interface{}{
				"providers": secretProviders,
				"defaults": map[string]interface{}{
					"file": openclawcfg.CarrierFileSecretProviderAlias,
				},
			}
		}
		if defaultModelID != "" {
			patch["agents"] = map[string]interface{}{
				"defaults": map[string]interface{}{
					"model": map[string]interface{}{
						"primary": defaultModelID,
					},
				},
			}
		}
		if len(secretsFilePayload) > 0 {
			patch[openclawcfg.CarrierSecretFilePatchKey] = map[string]interface{}{
				"providers": secretsFilePayload,
			}
		}
	}
	return patch, nil
}

func buildOpenClawProviderEntry(model configv2.Model) (providerKey, modelName, modelID string, providerItem map[string]interface{}, secretValue string, err error) {
	modelID = strings.TrimSpace(model.Model)
	modelName = strings.TrimSpace(model.ModelName)
	if modelID == "" && modelName == "" {
		return "", "", "", nil, "", errors.New("model entry has empty model and model_name")
	}
	if modelID == "" {
		modelID = modelName
	}
	if modelName == "" {
		if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
			modelName = strings.TrimSpace(name)
		} else {
			modelName = modelID
		}
	}

	providerKey = strings.TrimSpace(model.ProviderID)
	if vendor, _, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(vendor) != "" {
		providerKey = strings.TrimSpace(vendor)
	}
	providerKey = mapCarrierProviderToManagedProvider(providerKey)
	if providerKey == "" {
		providerKey = "openai"
	}

	credentialRef := strings.TrimSpace(model.CredentialRef)
	if credentialRef == "" {
		credentialRef = strings.TrimSpace(model.ProviderID)
	}
	token, _, ok, credErr := loadProviderCredential(credentialRef)
	if credErr != nil {
		return "", "", "", nil, "", fmt.Errorf("load provider credential %q: %w", credentialRef, credErr)
	}
	if !ok || strings.TrimSpace(token) == "" {
		return "", "", "", nil, "", fmt.Errorf("missing credential for provider %q (ref=%q)", model.ProviderID, credentialRef)
	}
	secretValue = strings.TrimSpace(token)

	providerBaseURL := ""
	if providerSpec := catalog.GetProvider(model.ProviderID); providerSpec != nil {
		providerBaseURL = strings.TrimSpace(providerSpec.DefaultBase)
	}
	providerItem = openclawcfg.BuildProviderEntry(
		model.ProviderID,
		providerKey,
		providerBaseURL,
		modelID,
		true,
	)
	return providerKey, modelName, modelID, providerItem, secretValue, nil
}

func buildZeroClawRemoteConfigPatch(opts remoteCommandOptions, cfg *configv2.Config) (map[string]interface{}, error) {
	syncChannels := dedupeLowerStrings(opts.SyncChannels)
	syncProviders := dedupeLowerStrings(opts.SyncProviders)
	if (len(syncChannels) == 0) != (len(syncProviders) == 0) {
		return nil, errors.New("zeroclaw sync requires both --sync-channel and --sync-provider to avoid overwriting existing remote config")
	}

	var (
		defaultProvider = "openai"
		defaultModel    = "openai/gpt-5.3-codex"
		defaultAPIKey   = ""
	)
	if len(syncProviders) > 0 {
		model, ok := resolveLocalModelForProvider(cfg, syncProviders[0])
		if !ok {
			return nil, fmt.Errorf("local provider %q is not found in model_list", syncProviders[0])
		}
		modelID := strings.TrimSpace(model.Model)
		if modelID == "" {
			modelID = strings.TrimSpace(model.ModelName)
		}
		providerID := strings.TrimSpace(model.ProviderID)
		if vendor, _, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(vendor) != "" {
			providerID = strings.TrimSpace(vendor)
		}
		defaultProvider = mapCarrierProviderToManagedProvider(providerID)
		defaultModel = modelID
		credentialRef := strings.TrimSpace(model.CredentialRef)
		if credentialRef == "" {
			credentialRef = strings.TrimSpace(model.ProviderID)
		}
		token, _, okCred, err := loadProviderCredential(credentialRef)
		if err != nil {
			return nil, fmt.Errorf("load provider credential %q: %w", credentialRef, err)
		}
		if !catalog.IsOpenAICodexProviderID(model.ProviderID) {
			if !okCred || strings.TrimSpace(token) == "" {
				return nil, fmt.Errorf("missing credential for provider %q (ref=%q)", model.ProviderID, credentialRef)
			}
			defaultAPIKey = strings.TrimSpace(token)
		}
	}

	channelSections := make([]string, 0)
	for _, selected := range syncChannels {
		local, ok := findLocalChannel(cfg, selected)
		if !ok {
			return nil, fmt.Errorf("local channel %q is not configured", selected)
		}
		token := strings.TrimSpace(local.BotToken)
		if token == "" {
			return nil, fmt.Errorf("local channel %q has empty bot_token", selected)
		}
		allowFrom := []string{}
		switch selected {
		case "telegram":
			if len(opts.TelegramAllowFrom) > 0 {
				allowFrom = dedupeTrimmedStrings(opts.TelegramAllowFrom)
			}
		case "discord":
			if len(opts.DiscordAllowFrom) > 0 {
				allowFrom = dedupeTrimmedStrings(opts.DiscordAllowFrom)
			}
		}
		if len(allowFrom) == 0 {
			allowFrom = dedupeTrimmedStrings(local.AllowFrom)
		}
		quotedUsers := make([]string, 0, len(allowFrom))
		for _, userID := range allowFrom {
			quotedUsers = append(quotedUsers, strconv.Quote(userID))
		}
		users := "[]"
		if len(quotedUsers) > 0 {
			users = "[" + strings.Join(quotedUsers, ", ") + "]"
		}
		channelSections = append(channelSections,
			fmt.Sprintf("[channels_config.%s]", selected),
			fmt.Sprintf("bot_token = %s", strconv.Quote(token)),
			fmt.Sprintf("allowed_users = %s", users),
			"mention_only = false",
		)
	}
	if len(channelSections) == 0 {
		channelSections = append(channelSections,
			"[channels_config.telegram]",
			"bot_token = \"\"",
			"allowed_users = []",
			"mention_only = false",
		)
	}

	lines := []string{
		"# Generated by Carrier CLI remote add",
		fmt.Sprintf("api_key = %s", strconv.Quote(defaultAPIKey)),
		fmt.Sprintf("default_provider = %s", strconv.Quote(defaultProvider)),
		fmt.Sprintf("default_model = %s", strconv.Quote(defaultModel)),
		"default_temperature = 0.7",
		"",
		"[agent]",
		"max_tool_iterations = 20",
		"",
	}
	lines = append(lines, channelSections...)
	return map[string]interface{}{
		"raw_toml": strings.Join(lines, "\n") + "\n",
	}, nil
}

func resolveLocalModelForProvider(cfg *configv2.Config, requestedProvider string) (configv2.Model, bool) {
	target := strings.ToLower(strings.TrimSpace(requestedProvider))
	targetCanonical := mapCarrierProviderToManagedProvider(target)
	for _, model := range cfg.ModelList {
		providerID := strings.ToLower(strings.TrimSpace(model.ProviderID))
		if providerID == target {
			return model, true
		}
	}
	for _, model := range cfg.ModelList {
		providerID := strings.ToLower(strings.TrimSpace(model.ProviderID))
		if mapCarrierProviderToManagedProvider(providerID) == targetCanonical {
			return model, true
		}
	}
	return configv2.Model{}, false
}

func buildManagedProviderAndModelEntry(model configv2.Model) (providerKey string, modelName string, modelID string, providerItem map[string]interface{}, modelItem map[string]interface{}, err error) {
	modelID = strings.TrimSpace(model.Model)
	modelName = strings.TrimSpace(model.ModelName)
	if modelID == "" && modelName == "" {
		return "", "", "", nil, nil, errors.New("model entry has empty model and model_name")
	}
	if modelID == "" {
		modelID = modelName
	}
	if modelName == "" {
		if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
			modelName = strings.TrimSpace(name)
		} else {
			modelName = modelID
		}
	}

	providerKey = strings.TrimSpace(model.ProviderID)
	if vendor, _, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(vendor) != "" {
		providerKey = strings.TrimSpace(vendor)
	}
	providerKey = mapCarrierProviderToManagedProvider(providerKey)
	if providerKey == "" {
		providerKey = "openai"
	}

	credentialRef := strings.TrimSpace(model.CredentialRef)
	if credentialRef == "" {
		credentialRef = strings.TrimSpace(model.ProviderID)
	}
	providerItem = map[string]interface{}{
		"credential_ref": credentialRef,
	}
	modelItem = map[string]interface{}{
		"model_name": modelName,
		"model":      modelID,
	}
	if catalog.IsOpenAICodexProviderID(model.ProviderID) {
		providerItem["auth_method"] = "oauth"
		modelItem["auth_method"] = "oauth"
		return providerKey, modelName, modelID, providerItem, modelItem, nil
	}

	token, _, ok, credErr := loadProviderCredential(credentialRef)
	if credErr != nil {
		return "", "", "", nil, nil, fmt.Errorf("load provider credential %q: %w", credentialRef, credErr)
	}
	if !ok || strings.TrimSpace(token) == "" {
		return "", "", "", nil, nil, fmt.Errorf("missing credential for provider %q (ref=%q)", model.ProviderID, credentialRef)
	}
	providerItem["api_key"] = strings.TrimSpace(token)
	return providerKey, modelName, modelID, providerItem, modelItem, nil
}

func findLocalChannel(cfg *configv2.Config, channelID string) (configv2.Channel, bool) {
	target := strings.ToLower(strings.TrimSpace(channelID))
	for _, channel := range cfg.Channels {
		if strings.EqualFold(strings.TrimSpace(channel.ID), target) {
			return channel, true
		}
	}
	return configv2.Channel{}, false
}

func dedupeLowerStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.ToLower(strings.TrimSpace(raw))
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func dedupeTrimmedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func anyToString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func anyToBool(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		lower := strings.ToLower(strings.TrimSpace(typed))
		return lower == "true" || lower == "1" || lower == "yes"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}

func resolveManagedInstanceTarget(instances []managedAgentInstance, target string) (managedAgentInstance, int, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return managedAgentInstance{}, -1, errors.New("instance id or name is required")
	}
	matches := make([]int, 0, 1)
	for i, inst := range instances {
		if managedInstanceMatchesTarget(inst, trimmed) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return managedAgentInstance{}, -1, fmt.Errorf("instance %s not found", trimmed)
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, idx := range matches {
			inst := instances[idx]
			names = append(names, fmt.Sprintf("%s(%s)", strings.TrimSpace(inst.ID), managedInstanceDisplayName(inst)))
		}
		sort.Strings(names)
		return managedAgentInstance{}, -1, fmt.Errorf("target %s is ambiguous; matches: %s", trimmed, strings.Join(names, ", "))
	}
	idx := matches[0]
	return instances[idx], idx, nil
}

func managedInstanceMatchesTarget(inst managedAgentInstance, target string) bool {
	candidates := []string{
		inst.ID,
		inst.Name,
		inst.Type,
		inst.AgentID,
	}
	for _, candidate := range candidates {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(target)) && strings.TrimSpace(candidate) != "" {
			return true
		}
	}
	return false
}

func managedInstanceDisplayName(inst managedAgentInstance) string {
	if name := strings.TrimSpace(inst.Name); name != "" {
		return name
	}
	if agent := strings.TrimSpace(inst.AgentID); agent != "" {
		return agent
	}
	return strings.TrimSpace(inst.Type)
}

func stopBackgroundService(
	name string,
	baseURL string,
	healthProbe func(string) bool,
) (bool, string) {
	stoppedByPID, pidErr := stopServiceByPIDFile(name)
	if pidErr == nil && stoppedByPID {
		return true, "stopped (pid file)"
	}

	if !healthProbe(baseURL) {
		return false, "not running"
	}

	port := portFromBaseURL(baseURL)
	if port <= 0 {
		if pidErr != nil {
			return false, fmt.Sprintf("running but unable to stop via pid file: %v", pidErr)
		}
		return false, "running but no pid file available"
	}
	stoppedByPort, stopSource, portErr := stopServiceByPort(name, port)
	if portErr == nil && stoppedByPort > 0 {
		return true, fmt.Sprintf("stopped (%d process via %s on port %d)", stoppedByPort, stopSource, port)
	}

	if pidErr != nil {
		return false, fmt.Sprintf("running but stop failed (pid file err=%v, port err=%v)", pidErr, portErr)
	}
	if portErr != nil {
		return false, fmt.Sprintf("running but stop by port failed: %v", portErr)
	}
	return false, "running but no stoppable process found"
}

func stopServiceByPIDFile(name string) (bool, error) {
	path, err := bootstrapPIDPath(name)
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read pid file %s: %w", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		_ = os.Remove(path)
		return false, fmt.Errorf("invalid pid in %s", path)
	}
	stopped, stopErr := stopPID(pid)
	_ = os.Remove(path)
	return stopped, stopErr
}

func stopPID(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid pid %d", pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}

	_ = proc.Signal(os.Interrupt)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !processLikelyAlive(proc) {
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := proc.Kill(); err != nil && !isProcessAlreadyGone(err) {
		return false, err
	}
	return true, nil
}

func processLikelyAlive(proc *os.Process) bool {
	if proc == nil {
		return false
	}
	err := proc.Signal(syscall.Signal(0))
	return err == nil
}

func isProcessAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrProcessDone) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "finished") || strings.Contains(msg, "no such process") || strings.Contains(msg, "process already done")
}

func stopServiceByPort(name string, port int) (int, string, error) {
	if port <= 0 {
		return 0, "", errors.New("invalid port")
	}
	if runtime.GOOS == "windows" {
		return 0, "", errors.New("port-based stop is not supported on windows")
	}

	pids, source, err := lookupPIDsByPort(port)
	if err != nil {
		return 0, source, err
	}
	if len(pids) == 0 {
		return 0, source, nil
	}

	stopped := 0
	for _, pid := range pids {
		ok, stopErr := stopPID(pid)
		if stopErr == nil && ok {
			stopped++
		}
	}
	if stopped == 0 {
		return 0, source, nil
	}
	if pidPath := mustBootstrapPIDPath(name); strings.TrimSpace(pidPath) != "" {
		_ = os.Remove(pidPath)
	}
	return stopped, source, nil
}

func lookupPIDsByPort(port int) ([]int, string, error) {
	if lsofPath, err := exec.LookPath("lsof"); err == nil {
		pids, lookupErr := lookupPIDsByPortViaLsof(lsofPath, port)
		return pids, "lsof", lookupErr
	}
	if runtime.GOOS == "linux" {
		pids, err := lookupPIDsByPortViaProc(port)
		if err != nil {
			return nil, "/proc fallback", err
		}
		return pids, "/proc fallback", nil
	}
	return nil, "", errors.New("lsof is not available")
}

func lookupPIDsByPortViaLsof(lsofPath string, port int) ([]int, error) {
	cmd := exec.Command(lsofPath, "-nP", "-ti", fmt.Sprintf("tcp:%d", port))
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("query pid by port %d: %w", port, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	pids := make([]int, 0, len(lines))
	seen := map[int]struct{}{}
	for _, line := range lines {
		pid, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids, nil
}

func lookupPIDsByPortViaProc(port int) ([]int, error) {
	inode, err := lookupListeningSocketInode(port)
	if err != nil {
		return nil, err
	}
	pids, err := lookupPIDsBySocketInode(inode)
	if err != nil {
		return nil, err
	}
	sort.Ints(pids)
	return pids, nil
}

func lookupListeningSocketInode(port int) (string, error) {
	for _, procFile := range []string{
		filepath.Join(procFSRoot, "net", "tcp"),
		filepath.Join(procFSRoot, "net", "tcp6"),
	} {
		inode, err := lookupListeningSocketInodeInFile(procFile, port)
		if err == nil && strings.TrimSpace(inode) != "" {
			return inode, nil
		}
	}
	return "", errors.New("listening socket not found")
}

func lookupListeningSocketInodeInFile(path string, port int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	firstLine := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if firstLine {
			firstLine = false
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
		_, localPortHex, ok := strings.Cut(localAddress, ":")
		if !ok {
			continue
		}
		parsedPort, parseErr := strconv.ParseInt(localPortHex, 16, 32)
		if parseErr != nil || int(parsedPort) != port {
			continue
		}
		inode := strings.TrimSpace(fields[9])
		if inode != "" {
			return inode, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("socket inode not found")
}

// lookupPIDsBySocketInode scans /proc/*/fd and is intentionally scoped to
// stop-path diagnostics/recovery, not hot-path request handling.
func lookupPIDsBySocketInode(inode string) ([]int, error) {
	if strings.TrimSpace(inode) == "" {
		return nil, errors.New("socket inode is empty")
	}
	target := "socket:[" + strings.TrimSpace(inode) + "]"
	entries, err := os.ReadDir(procFSRoot)
	if err != nil {
		return nil, err
	}
	pids := make([]int, 0)
	seen := map[int]struct{}{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil || pid <= 0 {
			continue
		}
		fdDir := filepath.Join(procFSRoot, entry.Name(), "fd")
		fds, fdErr := os.ReadDir(fdDir)
		if fdErr != nil {
			continue
		}
		found := false
		for _, fd := range fds {
			linkPath := filepath.Join(fdDir, fd.Name())
			linkTarget, linkErr := os.Readlink(linkPath)
			if linkErr != nil {
				continue
			}
			if linkTarget == target {
				found = true
				break
			}
		}
		if found {
			if _, ok := seen[pid]; ok {
				continue
			}
			seen[pid] = struct{}{}
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func portFromBaseURL(baseURL string) int {
	parsed, err := neturl.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return 0
	}
	if parsed.Port() == "" {
		return 0
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 {
		return 0
	}
	return port
}

func managedInstancesPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_INSTANCE_STORE")); custom != "" {
		return custom, nil
	}
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for instance store: %w", err)
	}
	return filepath.Join(home, ".carrier", "instances.json"), nil
}

func generateManagedInstanceID(prefix string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		p = "agent"
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random id: %w", err)
	}
	return fmt.Sprintf("%s-%s", p, hex.EncodeToString(buf)), nil
}

func loadManagedInstances() ([]managedAgentInstance, string, error) {
	path, err := managedInstancesPath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []managedAgentInstance{}, path, nil
		}
		return nil, "", fmt.Errorf("read instance store: %w", err)
	}
	var file managedAgentInstanceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, "", fmt.Errorf("parse instance store: %w", err)
	}
	if file.Instances == nil {
		file.Instances = []managedAgentInstance{}
	}
	return file.Instances, path, nil
}

func saveManagedInstances(path string, instances []managedAgentInstance) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("instance store path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create instance store dir: %w", err)
	}
	payload := managedAgentInstanceFile{Instances: instances}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal instance store: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write instance store: %w", err)
	}
	return nil
}

func findManagedInstanceIndex(instances []managedAgentInstance, instanceID string) int {
	id := strings.TrimSpace(instanceID)
	for i, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.ID), id) {
			return i
		}
	}
	return -1
}

func findManagedInstanceIndexByAgentID(instances []managedAgentInstance, agentID string) int {
	id := strings.TrimSpace(agentID)
	for i, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.AgentID), id) || strings.EqualFold(strings.TrimSpace(inst.Type), id) {
			return i
		}
	}
	return -1
}

func upsertManagedInstance(inst managedAgentInstance) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	idx := findManagedInstanceIndex(instances, inst.ID)
	if idx >= 0 {
		instances[idx] = inst
	} else {
		instances = append(instances, inst)
	}
	return saveManagedInstances(path, instances)
}

func cleanupManagedInstanceFiles(inst managedAgentInstance) error {
	var firstErr error
	removeFile := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	removeDir := func(path string) {
		p := strings.TrimSpace(path)
		if p == "" {
			return
		}
		if err := os.RemoveAll(p); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}

	removeFile(strings.TrimSpace(inst.RecordPath))
	removeFile(strings.TrimSpace(inst.ConfigPath))
	removeDir(strings.TrimSpace(inst.Workspace))
	return firstErr
}

func startGatewayInBackgroundAndWait() error {
	if err := gatewayBackgroundStarter(); err != nil {
		return err
	}
	gatewayURL := gatewayProbeBaseURL()
	if err := waitForGatewayHealthy(gatewayURL, gatewayBootTimeout); err != nil {
		return fmt.Errorf("gateway failed to become healthy at %s within %s: %w", gatewayURL, gatewayBootTimeout, err)
	}
	return nil
}

func printRuntimeSummary(out io.Writer) {
	daemonURL := daemonProbeBaseURL()
	gatewayURL := gatewayProbeBaseURL()
	daemonReady := daemonHealthProbe(daemonURL)
	gatewayReady := gatewayHealthProbe(gatewayURL)
	webUIReady := checkWebUIReady(gatewayURL)

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Runtime summary:")
	_, _ = fmt.Fprintf(out, "- daemon: %s (%s)\n", statusText(daemonReady), daemonURL)
	_, _ = fmt.Fprintf(out, "- gateway: %s (%s)\n", statusText(gatewayReady), gatewayURL)
	if webUIReady {
		_, _ = fmt.Fprintf(out, "- webui: ready (%s/)\n", strings.TrimRight(gatewayURL, "/"))
	} else {
		_, _ = fmt.Fprintf(out, "- webui: unavailable (%s/)\n", strings.TrimRight(gatewayURL, "/"))
	}
	if !bootstrapVerboseEnabled() {
		if daemonLogPath, err := bootstrapLogPath("daemon"); err == nil {
			if _, statErr := os.Stat(daemonLogPath); statErr == nil {
				_, _ = fmt.Fprintf(out, "- daemon log: %s\n", daemonLogPath)
			}
		}
		if gatewayLogPath, err := bootstrapLogPath("gateway"); err == nil {
			if _, statErr := os.Stat(gatewayLogPath); statErr == nil {
				_, _ = fmt.Fprintf(out, "- gateway log: %s\n", gatewayLogPath)
			}
		}
	}

	if cfg, cfgPath, err := configv2.Load(); err == nil && cfg != nil {
		_, _ = fmt.Fprintf(out, "- config: %s\n", cfgPath)
		if chatInfo := summarizeConfiguredChannels(cfg); chatInfo != "" {
			_, _ = fmt.Fprintf(out, "- chat apps: %s\n", chatInfo)
		}
		if modelInfo := summarizeDefaultModel(cfg); modelInfo != "" {
			_, _ = fmt.Fprintf(out, "- default model: %s\n", modelInfo)
		}
	}

	if daemonReady {
		pairCode, pairCodeExpiresAt, err := daemonPairCodeFetcher(daemonURL)
		if err == nil && strings.TrimSpace(pairCode) != "" {
			_, _ = fmt.Fprintf(out, "- pair code: %s\n", pairCode)
			if expiry, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(pairCodeExpiresAt)); parseErr == nil {
				remaining := time.Until(expiry)
				if remaining < 0 {
					remaining = 0
				}
				_, _ = fmt.Fprintf(out, "  expires in %s\n", remaining.Round(time.Second))
			}
		}
	}

	if gatewayReady {
		if transport, err := fetchGatewayTelegramTransportStatus(gatewayURL); err == nil && strings.TrimSpace(transport.SelectedMode) != "" && transport.SelectedMode != "unknown" {
			line := fmt.Sprintf("- telegram transport: %s", transport.SelectedMode)
			if strings.TrimSpace(transport.ReasonCode) != "" {
				line += fmt.Sprintf(" (reason_code=%s)", transport.ReasonCode)
			}
			_, _ = fmt.Fprintln(out, line)
			if strings.EqualFold(strings.TrimSpace(transport.SelectedMode), "polling") && strings.TrimSpace(transport.Reason) != "" {
				_, _ = fmt.Fprintf(out, "  reason: %s\n", transport.Reason)
			}
			if strings.TrimSpace(transport.Hint) != "" {
				_, _ = fmt.Fprintf(out, "  hint: %s\n", transport.Hint)
			}
		}
	}
}

type gatewayTelegramTransportStatus struct {
	SelectedMode string `json:"selected_mode"`
	ReasonCode   string `json:"reason_code"`
	Reason       string `json:"reason"`
	Hint         string `json:"hint"`
}

func fetchGatewayTelegramTransportStatus(gatewayURL string) (gatewayTelegramTransportStatus, error) {
	target := strings.TrimRight(strings.TrimSpace(gatewayURL), "/") + "/api/v1/telegram/transport"
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return gatewayTelegramTransportStatus{}, err
	}
	addGatewayAuthHeader(req)
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return gatewayTelegramTransportStatus{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return gatewayTelegramTransportStatus{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return gatewayTelegramTransportStatus{}, fmt.Errorf("transport status request failed with status %d", resp.StatusCode)
	}
	var payload struct {
		Transport gatewayTelegramTransportStatus `json:"transport"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gatewayTelegramTransportStatus{}, err
	}
	return payload.Transport, nil
}

func statusText(ready bool) string {
	if ready {
		return "running"
	}
	return "not running"
}

func summarizeConfiguredChannels(cfg *configv2.Config) string {
	if cfg == nil {
		return ""
	}
	items := make([]string, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		if !ch.Enabled {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(ch.ID))
		if id == "" {
			continue
		}
		details := []string{id}
		if id == "telegram" {
			mode := strings.ToLower(strings.TrimSpace(ch.TransportMode))
			if mode == "" {
				mode = "auto"
			}
			details = append(details, "mode="+mode)
		}
		if strings.TrimSpace(ch.BotToken) != "" {
			details = append(details, "token=set")
		}
		if strings.TrimSpace(ch.WebhookURL) != "" {
			details = append(details, "webhook=url-set")
		}
		items = append(items, strings.Join(details, ","))
	}
	if len(items) == 0 {
		return ""
	}
	sort.Strings(items)
	return strings.Join(items, " | ")
}

func summarizeDefaultModel(cfg *configv2.Config) string {
	pick, err := configv2.ResolveDefaultModel(cfg)
	if err != nil || pick == nil {
		return ""
	}
	modelID := strings.TrimSpace(pick.Model)
	if modelID == "" {
		modelID = strings.TrimSpace(pick.ModelName)
	}
	if modelID == "" {
		return ""
	}
	if providerID := strings.TrimSpace(pick.ProviderID); providerID != "" {
		return fmt.Sprintf("%s (provider=%s)", modelID, providerID)
	}
	return modelID
}

func configuredDefaultProviderID() string {
	cfg, _, err := configv2.Load()
	if err != nil {
		return ""
	}
	pick, err := configv2.ResolveDefaultModel(cfg)
	if err != nil || pick == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(pick.ProviderID))
}

func applyConfigV2Env(out io.Writer) {
	cfg, _, err := configv2.Load()
	if err != nil {
		_, _ = fmt.Fprintf(out, "warning: failed to load config.v2: %v\n", err)
		return
	}
	if err := configv2.ApplyGatewayEnvironment(cfg); err != nil {
		_, _ = fmt.Fprintf(out, "warning: failed to apply config.v2 environment: %v\n", err)
	}
}

func runOnboard(in io.Reader, out io.Writer, startGateway func() error) error {
	reader := bufio.NewReader(in)

	_, _ = fmt.Fprintln(out, "Carrier TUI Onboard")
	_, _ = fmt.Fprintln(out, "-------------------")
	_, _ = fmt.Fprintln(out, "Tip: for browser onboarding, run `carrier onboard --webui` and open http://127.0.0.1:8787/")
	_, _ = fmt.Fprintln(out, "Step 1/4: Configure chat channel")
	channel, hasChannel, err := promptMinimalChannelSelection(reader, out)
	if err != nil {
		return err
	}
	channelEnv := map[string]string{}
	channelConfigs := []configv2.Channel{}
	if hasChannel {
		_, _ = fmt.Fprintf(out, "Using channel: %s\n", channel.Name)
		var channelCfg configv2.Channel
		channelEnv, channelCfg, err = promptChannelCredentialsMinimal(reader, out, channel)
		if err != nil {
			return err
		}
		channelConfigs = append(channelConfigs, channelCfg)
	} else {
		_, _ = fmt.Fprintln(out, "WebUI-only mode: skipping chat channel configuration.")
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Step 2/4: Configure LLM provider")
	provider, providerReason, err := pickMinimalProviderWithReason()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Auto-selected provider: %s (%s)\n", provider.Name, provider.ID)
	_, _ = fmt.Fprintf(out, "Selection reason: %s\n", providerReason)
	provider, override, err := promptMinimalProviderOverride(reader, out, provider)
	if err != nil {
		return err
	}
	if override {
		_, _ = fmt.Fprintf(out, "Provider override selected: %s (%s)\n", provider.Name, provider.ID)
	} else {
		_, _ = fmt.Fprintf(out, "Using provider: %s (%s)\n", provider.Name, provider.ID)
	}
	providerEnv, providerCredentialProvided, err := promptProviderAuthMinimal(reader, out, provider)
	if err != nil {
		return err
	}
	modelName := provider.ID + "-default"
	modelID := strings.TrimSpace(provider.ExampleModel)
	if modelID == "" {
		modelID = provider.ID + "/default"
	}
	model := configv2.Model{
		ModelName:  modelName,
		Model:      modelID,
		ProviderID: provider.ID,
		AuthMode:   string(provider.AuthMode),
		EnvVar:     provider.ProviderEnv,
	}
	if providerCredentialProvided {
		model.CredentialRef = provider.ID
	}
	modelList := []configv2.Model{model}
	defaultModel := modelName

	cfg := &configv2.Config{
		ConfigVersion: configv2.CurrentVersion,
		Channels:      channelConfigs,
		ModelList:     modelList,
		DefaultModel:  defaultModel,
		BaseAgent: configv2.BaseAgentSpec{
			Enabled:           true,
			PublicMemoryID:    "carrier.base.public.v1",
			ActiveMemoryID:    "carrier.base.active.v1",
			SelfHealBackupDir: "base-agent-memory-backups",
		},
		ConfiguredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	cfgPath, err := configv2.Save(cfg)
	if err != nil {
		return err
	}

	if err := configv2.ApplyGatewayEnvironment(cfg); err != nil {
		return err
	}

	combinedEnv := mergeEnvVars(channelEnv, providerEnv)
	if err := applyEnvVars(combinedEnv); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "\nSaved onboarding config to %s\n", cfgPath)
	if len(combinedEnv) > 0 {
		envKeys := make([]string, 0, len(combinedEnv))
		for k := range combinedEnv {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		_, _ = fmt.Fprintf(out, "Configured env vars for this process: %s\n", strings.Join(envKeys, ", "))
	}
	_, _ = fmt.Fprintln(out, "Step 3/4: Credential setup complete")
	_, _ = fmt.Fprintln(out, "Step 4/4: Launch gateway")
	if hasChannel {
		_, _ = fmt.Fprintf(out, "Selected channel: %s\n", channel.Name)
	} else {
		_, _ = fmt.Fprintln(out, "Selected channel: WebUI-only")
	}
	_, _ = fmt.Fprintf(out, "Selected provider: %s\n", provider.Name)
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Ensuring daemon is running...")

	daemonURL, err := ensureDaemonRunning(out)
	if err != nil {
		return err
	}
	if hasChannel {
		printDaemonPairCode(out, daemonURL)
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Starting gateway...")
	if hasChannel {
		_, _ = fmt.Fprintln(out, buildSlashCommandGuide(channel))
	} else {
		_, _ = fmt.Fprintln(out, "WebUI-only mode: connect from browser UI instead of chat slash commands.")
	}

	if _, err := ensureGatewayRunning(out, startGateway); err != nil {
		return err
	}
	return nil
}

func runOnboardWebUI(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "Carrier WebUI Onboard")
	_, _ = fmt.Fprintln(out, "--------------------")

	gatewayURL, err := ensureWebUIServices(out)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "\nOpen WebUI: %s/\n", gatewayURL)
	_, _ = fmt.Fprintln(out, "Use the browser flow to onboard Carrier and add agents.")
	return nil
}

func runAddWebUI(out io.Writer, agentID string, isolation bool) error {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return errors.New("agent_id is required")
	}
	_, _ = fmt.Fprintln(out, "Carrier Add (WebUI)")
	_, _ = fmt.Fprintln(out, "-------------------")
	_, _ = fmt.Fprintf(out, "Target agent: %s\n", agentID)

	gatewayURL, err := ensureWebUIServices(out)
	if err != nil {
		return err
	}
	target := fmt.Sprintf("%s/#/add/%s", strings.TrimRight(gatewayURL, "/"), neturl.PathEscape(agentID))
	if isolation {
		target = target + "?isolation=1"
	}
	_, _ = fmt.Fprintf(out, "\nOpening browser: %s\n", target)
	if err := openBrowserFunc(target); err != nil {
		_, _ = fmt.Fprintf(out, "Warning: failed to open browser automatically: %v\n", err)
		_, _ = fmt.Fprintln(out, "Please open the URL manually.")
	}
	return nil
}

func ensureWebUIServices(out io.Writer) (string, error) {
	daemonURL, err := ensureDaemonRunning(out)
	if err != nil {
		return "", err
	}
	printDaemonPairCode(out, daemonURL)

	probeURL := gatewayProbeBaseURL()
	gatewayURL, err := ensureGatewayRunning(out, func() error {
		if err := gatewayBackgroundStarter(); err != nil {
			return fmt.Errorf("start gateway in background: %w", err)
		}
		if err := waitForGatewayHealthy(probeURL, gatewayBootTimeout); err != nil {
			return fmt.Errorf("gateway failed to become healthy at %s within %s: %w", probeURL, gatewayBootTimeout, err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if !checkWebUIReady(gatewayURL) {
		return "", fmt.Errorf("gateway is running at %s but WebUI root route returned 404", gatewayURL)
	}
	return gatewayURL, nil
}

func runAddTUI(in io.Reader, out io.Writer, agentID string, quiet bool, isolation bool) error {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return errors.New("agent_id is required")
	}
	if !isManagedAgent(agentID) {
		if isolation {
			return fmt.Errorf("isolation is not supported for unmanaged agent %s", agentID)
		}
		_, _ = fmt.Fprintf(out, "Adding %s via direct install/start...\n", agentID)
		if _, err := ensureDaemonRunning(out); err != nil {
			return err
		}
		if err := daemonAgentActionWithProgress(out, agentID, "install", quiet); err != nil {
			return err
		}
		if err := daemonAgentActionWithProgress(out, agentID, "start", quiet); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "✅ %s installed and started.\n", agentID)
		return nil
	}
	return runAddManagedAgentTUI(in, out, agentID, quiet, isolation)
}

func runAddManagedAgentTUI(in io.Reader, out io.Writer, agentID string, quiet bool, isolation bool) error {
	cfg, ok := managedAgentByID(agentID)
	if !ok {
		return fmt.Errorf("managed agent %q is not supported", agentID)
	}
	if err := ensureWindowsWSLPrereqForOpenClaw(cfg.ID); err != nil {
		return err
	}
	reader := bufio.NewReader(in)
	_, _ = fmt.Fprintln(out, "Carrier Add (TUI)")
	_, _ = fmt.Fprintln(out, "-----------------")
	_, _ = fmt.Fprintf(out, "Agent: %s\n", cfg.Name)
	if isolation {
		_, _ = fmt.Fprintln(out, "Isolation runtime: enabled")
	} else {
		_, _ = fmt.Fprintln(out, "Isolation runtime: disabled")
	}
	_, _ = fmt.Fprintf(out, "Tip: for browser flow, run `carrier add %s --webui`.\n", cfg.ID)
	instanceName := cfg.ID
	instanceID := ""
	createdAt := ""
	existingInstances, _, err := loadManagedInstances()
	if err != nil {
		return err
	}
	if existingIdx := findManagedInstanceIndexByAgentID(existingInstances, cfg.ID); existingIdx >= 0 {
		instanceID = strings.TrimSpace(existingInstances[existingIdx].ID)
		createdAt = strings.TrimSpace(existingInstances[existingIdx].CreatedAt)
		_, _ = fmt.Fprintf(out, "Reusing existing instance for %s.\n", cfg.ID)
	}
	if instanceID == "" {
		instanceID, err = generateManagedInstanceID(cfg.ID)
		if err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(out, "Instance: %s\n", instanceID)
	_, _ = fmt.Fprintf(out, "Name: %s\n", instanceName)
	_, _ = fmt.Fprintln(out, "Step 1/4: Configure chat channel")
	channel, hasChannel, err := promptManagedChannelSelection(reader, out, cfg.ID, quiet)
	if err != nil {
		return err
	}
	channelID := ""
	channelName := ""
	token := ""
	tokenSource := ""
	if hasChannel {
		channelID = channel.ID
		channelName = channel.Name
		_, _ = fmt.Fprintf(out, "Using channel: %s\n", channel.Name)
	}
	reuseChannelToken := hasChannel && managedAddReusesChannelToken(cfg.ID, channel.ID)
	if reuseChannelToken {
		token, tokenSource = resolveManagedChannelToken(channel.ID)
		if tokenSource != "" {
			_, _ = fmt.Fprintf(out, "Reused %s token from %s.\n", channel.Name, tokenSource)
		}
	}
	if hasChannel && tokenSource == "" {
		if !reuseChannelToken {
			_, _ = fmt.Fprintf(out, "Token reuse is disabled for %s to avoid shared bot conflicts.\n", cfg.Name)
		}
		token, err = promptInput(reader, out, channel.TokenLabel+" (press Enter to skip)", false)
		if err != nil {
			return err
		}
		if strings.TrimSpace(token) == "" {
			_, _ = fmt.Fprintln(out, "Channel token skipped. Channel will be created but disabled.")
			_, _ = fmt.Fprintln(out, "Configure later via WebUI or `carrier config set`.")
		}
	}
	pairedChatID := ""
	pairedChatIDSource := ""
	if hasChannel {
		pairedChatID, pairedChatIDSource = latestManagedPairedChatID(cfg.ID, channel.ID)
	}
	if hasChannel && pairedChatIDSource != "" {
		_, _ = fmt.Fprintf(out, "Reused paired %s user id from %s: %s\n", channel.Name, pairedChatIDSource, pairedChatID)
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Step 2/4: Configure LLM provider")
	provider, providerReason, err := pickManagedAddProviderWithReason(cfg.ID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Auto-selected provider: %s (%s)\n", provider.Name, provider.ID)
	_, _ = fmt.Fprintf(out, "Selection reason: %s\n", providerReason)
	_, _ = fmt.Fprintf(out, "Using provider: %s (%s)\n", provider.Name, provider.ID)
	_, _ = fmt.Fprintln(out, "Saved provider credential will be reused automatically when available.")
	providerEnv, _, err := promptProviderAuthMinimal(reader, out, provider)
	if err != nil {
		return err
	}
	envVars := mergeEnvVars(providerEnv, nil)
	if err := ensureManagedAgentEnvRequirements(cfg.ID, envVars, provider); err != nil {
		return err
	}
	if err := applyEnvVars(envVars); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintf(out, "Step 3/4: Prepare %s configuration\n", cfg.Name)
	result, err := prepareManagedAgentAddArtifacts(cfg.ID, instanceID, channelID, token, provider, envVars, pairedChatID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s workspace: %s\n", cfg.Name, result.WorkspacePath)
	_, _ = fmt.Fprintf(out, "%s config: %s\n", cfg.Name, result.ConfigPath)
	_, _ = fmt.Fprintf(out, "Carrier record: %s\n", result.RecordPath)

	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintf(out, "Step 4/4: Install and start %s\n", cfg.Name)
	if _, err := ensureDaemonRunning(out); err != nil {
		return err
	}
	var installPayload map[string]interface{}
	if isolation {
		installPayload = map[string]interface{}{
			"isolation": true,
		}
	}
	if err := daemonAgentActionWithPayloadWithProgress(out, cfg.ID, "install", installPayload, quiet); err != nil {
		return err
	}
	startPayload := map[string]interface{}{}
	if isolation {
		startPayload["isolation"] = true
	}
	if err := daemonAgentActionWithPayloadWithProgress(out, cfg.ID, "start", startPayload, quiet); err != nil {
		return err
	}
	if strings.EqualFold(cfg.ID, "picoclaw") && hasChannel && strings.EqualFold(channel.ID, "telegram") && strings.TrimSpace(token) != "" {
		if pairCode, _ := daemonExtractPairCodeFromLogs(cfg.ID); strings.TrimSpace(pairCode) != "" {
			_, _ = fmt.Fprintf(out, "PicoClaw pair code: %s\n", pairCode)
			_, _ = fmt.Fprintf(out, "Next: send `/pair %s` in your PicoClaw Telegram bot chat.\n", pairCode)
		} else {
			_, _ = fmt.Fprintln(out, "Pair code not detected yet. Open PicoClaw Telegram bot chat and follow `/start` -> `/pair` prompts.")
		}
	} else if hasChannel && strings.TrimSpace(token) == "" {
		_, _ = fmt.Fprintf(out, "%s channel token is not set yet; configure it to enable chat pairing.\n", channelName)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if createdAt == "" {
		createdAt = now
	}
	inst := managedAgentInstance{
		ID:           instanceID,
		Name:         instanceName,
		Type:         cfg.ID,
		AgentID:      cfg.ID,
		Isolation:    isolation,
		GatewayURL:   gatewayProbeBaseURL(),
		Workspace:    result.WorkspacePath,
		ConfigPath:   result.ConfigPath,
		RecordPath:   result.RecordPath,
		Channel:      result.ChannelID,
		Provider:     result.ProviderID,
		PairRequired: hasChannel && strings.TrimSpace(token) != "" && strings.TrimSpace(result.PairedChatID) == "",
		PairedChatID: result.PairedChatID,
		Port:         result.Port,
		RuntimeState: "running",
		CreatedAt:    createdAt,
		UpdatedAt:    now,
	}
	if err := upsertManagedInstance(inst); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "✅ %s installed and started.\n", cfg.Name)
	return nil
}

func resolveManagedChannelToken(channelID string) (string, string) {
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	if channelID == "" {
		return "", ""
	}

	for _, ch := range channelOptions {
		if !strings.EqualFold(ch.ID, channelID) {
			continue
		}
		if envKey := strings.TrimSpace(ch.TokenEnv); envKey != "" {
			if token := strings.TrimSpace(os.Getenv(envKey)); token != "" {
				return token, fmt.Sprintf("environment variable %s", envKey)
			}
		}
		break
	}

	cfg, _, err := configv2.Load()
	if err != nil || cfg == nil {
		return "", ""
	}
	for _, ch := range cfg.Channels {
		if !strings.EqualFold(ch.ID, channelID) {
			continue
		}
		if token := strings.TrimSpace(ch.BotToken); token != "" {
			return token, "Carrier config channels"
		}
	}
	return "", ""
}

func managedAddReusesChannelToken(agentID, channelID string) bool {
	if !strings.EqualFold(strings.TrimSpace(channelID), "telegram") {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(agentID), "openclaw")
}

func pickManagedAddProviderWithReason(agentID string) (choiceOption, string, error) {
	defaultID := configuredDefaultProviderID()
	if defaultID != "" {
		if p, ok := resolveChoice(defaultID, providerOptions); ok && providerCompatibleForManagedAgent(agentID, p) {
			return p, fmt.Sprintf("config default from config.v2 default_model (provider=%s)", defaultID), nil
		}
	}

	if p, reason, ok := pickProviderFromLatestManagedInstance(agentID); ok {
		return p, reason, nil
	}

	if p, reason, ok := pickProviderWithSavedCredential(agentID); ok {
		return p, reason, nil
	}

	p, reason, err := pickMinimalProviderWithReason()
	if err != nil {
		return choiceOption{}, "", err
	}
	if providerCompatibleForManagedAgent(agentID, p) {
		return p, reason, nil
	}

	for _, candidate := range providerOptions {
		if providerCompatibleForManagedAgent(agentID, candidate) {
			return candidate, "fallback to first compatible provider", nil
		}
	}
	return choiceOption{}, "", fmt.Errorf("no compatible provider available for %s", agentID)
}

func pickProviderFromLatestManagedInstance(agentID string) (choiceOption, string, bool) {
	providerID := latestManagedInstanceProvider(agentID)
	if providerID == "" {
		return choiceOption{}, "", false
	}
	provider, ok := resolveChoice(providerID, providerOptions)
	if !ok || !providerCompatibleForManagedAgent(agentID, provider) {
		return choiceOption{}, "", false
	}
	return provider, fmt.Sprintf("reused provider from latest %s instance (%s)", strings.ToLower(strings.TrimSpace(agentID)), provider.ID), true
}

func pickProviderWithSavedCredential(agentID string) (choiceOption, string, bool) {
	for _, candidate := range providerOptions {
		if !providerCompatibleForManagedAgent(agentID, candidate) {
			continue
		}
		if candidate.AuthMode == authModeNone {
			continue
		}
		_, backend, ok, err := loadProviderCredential(candidate.ID)
		if err != nil || !ok {
			continue
		}
		return candidate, fmt.Sprintf("reused saved credential for %s (%s)", candidate.ID, backend), true
	}
	return choiceOption{}, "", false
}

func latestManagedInstanceProvider(agentID string) string {
	instances, _, err := loadManagedInstances()
	if err != nil || len(instances) == 0 {
		return ""
	}

	target := strings.ToLower(strings.TrimSpace(agentID))
	bestIdx := latestManagedInstanceIndex(instances, func(inst managedAgentInstance) bool {
		if !managedInstanceMatchesAgent(inst, target) {
			return false
		}
		return strings.TrimSpace(inst.Provider) != ""
	})

	if bestIdx < 0 {
		return ""
	}
	return catalog.NormalizeProviderID(strings.TrimSpace(instances[bestIdx].Provider))
}

func latestManagedPairedChatID(agentID, channelID string) (string, string) {
	instances, _, err := loadManagedInstances()
	if err != nil || len(instances) == 0 {
		return "", ""
	}

	targetAgent := strings.ToLower(strings.TrimSpace(agentID))
	targetChannel := strings.ToLower(strings.TrimSpace(channelID))
	bestIdx := latestManagedInstanceIndex(instances, func(inst managedAgentInstance) bool {
		if !managedInstanceMatchesAgent(inst, targetAgent) {
			return false
		}
		if targetChannel != "" && !strings.EqualFold(strings.TrimSpace(inst.Channel), targetChannel) {
			return false
		}
		pairedChatID := strings.TrimSpace(inst.PairedChatID)
		return pairedChatID != ""
	})

	if bestIdx < 0 {
		return "", ""
	}
	return strings.TrimSpace(instances[bestIdx].PairedChatID), "latest managed instance"
}

func latestManagedInstanceIndex(instances []managedAgentInstance, match func(managedAgentInstance) bool) int {
	if len(instances) == 0 || match == nil {
		return -1
	}

	bestIdx := -1
	var bestTime time.Time
	bestHasTime := false

	for i, inst := range instances {
		if !match(inst) {
			continue
		}
		updated, hasTime := parseManagedTimestamp(inst.UpdatedAt)

		if bestIdx == -1 {
			bestIdx = i
			bestTime = updated
			bestHasTime = hasTime
			continue
		}
		if hasTime && !bestHasTime {
			bestIdx = i
			bestTime = updated
			bestHasTime = true
			continue
		}
		if hasTime && bestHasTime {
			if updated.After(bestTime) || updated.Equal(bestTime) {
				bestIdx = i
				bestTime = updated
			}
			continue
		}
		if !hasTime && !bestHasTime {
			bestIdx = i
		}
	}

	return bestIdx
}

func managedInstanceMatchesAgent(inst managedAgentInstance, targetAgent string) bool {
	if strings.TrimSpace(targetAgent) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(inst.AgentID), targetAgent) ||
		strings.EqualFold(strings.TrimSpace(inst.Type), targetAgent)
}

func parseManagedTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func providerCompatibleForManagedAgent(agentID string, provider choiceOption) bool {
	cfg, ok := managedAgentByID(agentID)
	if !ok || strings.TrimSpace(cfg.RequiredEnvKey) == "" {
		return true
	}
	if strings.TrimSpace(provider.ProviderEnv) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv(cfg.RequiredEnvKey)) != ""
}

func parsePicoclawChannel(input string) (picoclawChannel, bool) {
	id := strings.ToLower(strings.TrimSpace(input))
	for _, ch := range picoclawChannels {
		if id == ch.ID {
			return ch, true
		}
	}
	return picoclawChannel{}, false
}

func promptAdditionalEnvVars(reader *bufio.Reader, out io.Writer) (map[string]string, error) {
	vars := map[string]string{}
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "Step 2.5/4: Additional environment variables (optional)")
	_, _ = fmt.Fprintln(out, "Enter KEY=VALUE pairs; type `done` to continue.")
	for {
		line, err := promptInput(reader, out, "env", false)
		if err != nil {
			return nil, err
		}
		text := strings.TrimSpace(line)
		if text == "" || strings.EqualFold(text, "done") || strings.EqualFold(text, "skip") {
			return vars, nil
		}
		eq := strings.IndexByte(text, '=')
		if eq <= 0 {
			_, _ = fmt.Fprintln(out, "Invalid format. Please use KEY=VALUE or `done`.")
			continue
		}
		key := strings.TrimSpace(text[:eq])
		value := strings.TrimSpace(text[eq+1:])
		if key == "" || value == "" {
			_, _ = fmt.Fprintln(out, "Invalid format. Please use KEY=VALUE with non-empty key/value.")
			continue
		}
		vars[key] = value
	}
}

func ensureDaemonRunning(out io.Writer) (string, error) {
	daemonURL := daemonProbeBaseURL()
	if daemonHealthProbe(daemonURL) {
		_, _ = fmt.Fprintf(out, "Daemon already running at %s, reusing existing process.\n", daemonURL)
		return daemonURL, nil
	}
	_, _ = fmt.Fprintf(out, "Daemon not detected at %s, starting in background...\n", daemonURL)
	if err := daemonBackgroundStarter(); err != nil {
		return "", fmt.Errorf("start daemon in background: %w", err)
	}
	if err := waitForDaemonHealthy(daemonURL, daemonBootTimeout); err != nil {
		return "", fmt.Errorf("daemon failed to become healthy at %s within %s: %w", daemonURL, daemonBootTimeout, err)
	}
	_, _ = fmt.Fprintf(out, "Daemon started and healthy at %s.\n", daemonURL)
	return daemonURL, nil
}

func printDaemonPairCode(out io.Writer, daemonURL string) {
	pairCode, pairCodeExpiresAt, err := daemonPairCodeFetcher(daemonURL)
	if err != nil {
		_, _ = fmt.Fprintf(out, "Warning: failed to fetch pair code from daemon: %v\n", err)
		return
	}
	if strings.TrimSpace(pairCode) == "" {
		return
	}
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintf(out, "PAIR_CODE: %s\n", pairCode)
	if strings.TrimSpace(pairCodeExpiresAt) == "" {
		return
	}
	if expiry, parseErr := time.Parse(time.RFC3339Nano, pairCodeExpiresAt); parseErr == nil {
		remaining := time.Until(expiry)
		if remaining < 0 {
			remaining = 0
		}
		_, _ = fmt.Fprintf(out, "(expires in %s)\n", remaining.Round(time.Second))
		return
	}
	_, _ = fmt.Fprintf(out, "(expiresAt: %s)\n", pairCodeExpiresAt)
}

func ensureGatewayRunning(out io.Writer, startGateway func() error) (string, error) {
	gatewayURL := gatewayProbeBaseURL()
	if gatewayHealthProbe(gatewayURL) {
		_, _ = fmt.Fprintf(out, "Gateway already running at %s, reusing existing process.\n", gatewayURL)
		return gatewayURL, nil
	}
	_, _ = fmt.Fprintf(out, "Gateway not detected at %s, starting in background...\n", gatewayURL)
	if err := startGateway(); err != nil {
		if isAddressInUseError(err) && gatewayHealthProbe(gatewayURL) {
			_, _ = fmt.Fprintf(out, "Gateway already running at %s, reusing existing process.\n", gatewayURL)
			return gatewayURL, nil
		}
		if isAddressInUseError(err) {
			return "", fmt.Errorf("gateway port is already in use; stop the conflicting process or set CARRIER_GATEWAY_PORT (probe: %s): %w", gatewayURL, err)
		}
		return "", err
	}
	return gatewayURL, nil
}

func daemonAgentActionWithProgress(out io.Writer, agentID, action string, quiet bool) error {
	return daemonAgentActionWithPayloadWithProgress(out, agentID, action, nil, quiet)
}

func daemonAgentActionWithPayloadWithProgress(out io.Writer, agentID, action string, body any, quiet bool) error {
	if quiet {
		return daemonAgentActionWithPayload(agentID, action, body)
	}

	previousLogs, _ := daemonFetchAgentLogs(agentID, 1000)
	startedAt := time.Now()
	lastHeartbeat := startedAt

	done := make(chan error, 1)
	go func() {
		done <- daemonAgentActionWithPayload(agentID, action, body)
	}()

	ticker := time.NewTicker(daemonActionLogPollInterval)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			if currentLogs, logErr := daemonFetchAgentLogs(agentID, 1000); logErr == nil {
				for _, line := range diffNewLogLines(previousLogs, currentLogs) {
					_, _ = fmt.Fprintln(out, line)
				}
			}
			return err
		case <-ticker.C:
			currentLogs, logErr := daemonFetchAgentLogs(agentID, 1000)
			if logErr != nil {
				if time.Since(lastHeartbeat) >= daemonActionHeartbeatInterval {
					_, _ = fmt.Fprintf(out, "[%s] in progress (%s elapsed)\n", action, time.Since(startedAt).Round(time.Second))
					lastHeartbeat = time.Now()
				}
				continue
			}
			newLines := diffNewLogLines(previousLogs, currentLogs)
			for _, line := range newLines {
				_, _ = fmt.Fprintln(out, line)
			}
			previousLogs = currentLogs
			if len(newLines) == 0 && time.Since(lastHeartbeat) >= daemonActionHeartbeatInterval {
				_, _ = fmt.Fprintf(out, "[%s] in progress (%s elapsed)\n", action, time.Since(startedAt).Round(time.Second))
				lastHeartbeat = time.Now()
			}
			if len(newLines) > 0 {
				lastHeartbeat = time.Now()
			}
		}
	}
}

func diffNewLogLines(previous, current []string) []string {
	if len(current) == 0 {
		return nil
	}
	maxOverlap := 0
	limit := len(previous)
	if len(current) < limit {
		limit = len(current)
	}
	for overlap := limit; overlap > 0; overlap-- {
		if logSliceEqual(previous[len(previous)-overlap:], current[:overlap]) {
			maxOverlap = overlap
			break
		}
	}
	if maxOverlap >= len(current) {
		return nil
	}
	return append([]string(nil), current[maxOverlap:]...)
}

func logSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func daemonAgentAction(agentID, action string) error {
	return daemonAgentActionWithPayload(agentID, action, nil)
}

func daemonAgentActionWithPayload(agentID, action string, body any) error {
	agentID = strings.TrimSpace(agentID)
	action = strings.TrimSpace(action)
	if agentID == "" || action == "" {
		return errors.New("agentID and action are required")
	}
	payload := body
	if payload == nil {
		payload = map[string]string{}
	}
	path := fmt.Sprintf("/api/v1/agents/%s/%s", neturl.PathEscape(agentID), neturl.PathEscape(action))
	_, status, err := daemonRequestWithTimeout(http.MethodPost, path, payload, daemonActionTimeout(action))
	if err != nil {
		reconciled, reconcileErr := reconcileDaemonActionOnTransportError(agentID, action, err)
		if reconciled {
			return nil
		}
		if reconcileErr != nil {
			return fmt.Errorf("daemon %s %s failed: %v (request error: %w)", action, agentID, reconcileErr, err)
		}
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("daemon %s %s failed with status %d", action, agentID, status)
	}
	return nil
}

type daemonAgentStatusSnapshot struct {
	InstallState string
	RuntimeState string
	LastError    string
}

type daemonAgentMetricsSnapshot struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemoryRSS    int64   `json:"memoryRSS"`
	Uptime       int64   `json:"uptime"`
	RestartCount int     `json:"restartCount"`
	LastErrorAt  string  `json:"lastErrorAt"`
}

func reconcileDaemonActionOnTransportError(agentID, action string, reqErr error) (bool, error) {
	if !isDaemonTransportErrorRecoverable(reqErr) {
		return false, nil
	}
	switch action {
	case "install", "start":
	default:
		return false, nil
	}
	status, err := daemonFetchAgentStatus(agentID)
	if err != nil {
		return false, err
	}
	switch action {
	case "install":
		switch strings.ToLower(strings.TrimSpace(status.InstallState)) {
		case "installed":
			return true, nil
		case "broken", "failed", "error":
			if lastErr := strings.TrimSpace(status.LastError); lastErr != "" {
				return false, fmt.Errorf("install state=%s: %s", strings.TrimSpace(status.InstallState), lastErr)
			}
			return false, fmt.Errorf("install state=%s", strings.TrimSpace(status.InstallState))
		default:
			return false, nil
		}
	case "start":
		runtimeState := strings.ToLower(strings.TrimSpace(status.RuntimeState))
		if runtimeState == "running" || runtimeState == "starting" {
			return true, nil
		}
		if runtimeState == "stopped" || runtimeState == "error" || runtimeState == "crashed" || runtimeState == "broken" {
			if lastErr := strings.TrimSpace(status.LastError); lastErr != "" {
				return false, fmt.Errorf("runtime state=%s: %s", strings.TrimSpace(status.RuntimeState), lastErr)
			}
			return false, fmt.Errorf("runtime state=%s", strings.TrimSpace(status.RuntimeState))
		}
		return false, nil
	default:
		return false, nil
	}
}

func daemonFetchAgentStatus(agentID string) (daemonAgentStatusSnapshot, error) {
	raw, status, err := daemonRequest(http.MethodGet, fmt.Sprintf("/api/v1/agents/%s/status", neturl.PathEscape(strings.TrimSpace(agentID))), nil)
	if err != nil {
		return daemonAgentStatusSnapshot{}, err
	}
	if status < 200 || status >= 300 {
		return daemonAgentStatusSnapshot{}, fmt.Errorf("daemon status request failed with status %d", status)
	}
	var direct struct {
		InstallState string `json:"installState"`
		RuntimeState string `json:"runtimeState"`
		Install      string `json:"install"`
		Runtime      string `json:"runtime"`
		LastError    string `json:"lastError"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil {
		installState := firstNonEmpty(direct.InstallState, direct.Install)
		runtimeState := firstNonEmpty(direct.RuntimeState, direct.Runtime)
		if installState != "" || runtimeState != "" {
			return daemonAgentStatusSnapshot{
				InstallState: installState,
				RuntimeState: runtimeState,
				LastError:    strings.TrimSpace(direct.LastError),
			}, nil
		}
	}
	var wrapped struct {
		Statuses []struct {
			InstallState string `json:"installState"`
			RuntimeState string `json:"runtimeState"`
			Install      string `json:"install"`
			Runtime      string `json:"runtime"`
			LastError    string `json:"lastError"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return daemonAgentStatusSnapshot{}, fmt.Errorf("decode daemon status response: %w", err)
	}
	if len(wrapped.Statuses) == 0 {
		return daemonAgentStatusSnapshot{}, errors.New("daemon status response did not include statuses")
	}
	first := wrapped.Statuses[0]
	return daemonAgentStatusSnapshot{
		InstallState: firstNonEmpty(first.InstallState, first.Install),
		RuntimeState: firstNonEmpty(first.RuntimeState, first.Runtime),
		LastError:    strings.TrimSpace(first.LastError),
	}, nil
}

func daemonFetchAgentMetrics(agentID string) (daemonAgentMetricsSnapshot, error) {
	raw, status, err := daemonRequest(http.MethodGet, fmt.Sprintf("/api/v1/agents/%s/metrics", neturl.PathEscape(strings.TrimSpace(agentID))), nil)
	if err != nil {
		return daemonAgentMetricsSnapshot{}, err
	}
	if status < 200 || status >= 300 {
		return daemonAgentMetricsSnapshot{}, fmt.Errorf("daemon metrics request failed with status %d", status)
	}
	var metrics daemonAgentMetricsSnapshot
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return daemonAgentMetricsSnapshot{}, fmt.Errorf("decode daemon metrics response: %w", err)
	}
	return metrics, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isDaemonEOFError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "eof") {
		fmt.Fprintf(os.Stderr, "[debug] isDaemonEOFError: string-match fallback hit for error: %v\n", err)
		return true
	}
	return false
}

func isDaemonTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "client.timeout exceeded while awaiting headers") ||
		strings.Contains(lower, "timeout exceeded")
}

func isDaemonTransportErrorRecoverable(err error) bool {
	return isDaemonEOFError(err) || isDaemonTimeoutError(err)
}

func daemonFetchAgentLogs(agentID string, tail int) ([]string, error) {
	trimmedAgentID := strings.TrimSpace(agentID)
	if trimmedAgentID == "" {
		return nil, errors.New("agent id is required")
	}
	if tail <= 0 {
		tail = 200
	}
	raw, status, err := daemonRequest(
		http.MethodGet,
		fmt.Sprintf("/api/v1/agents/%s/logs?tail=%d", neturl.PathEscape(trimmedAgentID), tail),
		nil,
	)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("daemon logs request failed with status %d", status)
	}
	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode logs response: %w", err)
	}
	return payload.Lines, nil
}

func daemonExtractPairCodeFromLogs(agentID string) (string, error) {
	lines, err := daemonFetchAgentLogs(agentID, 120)
	if err != nil {
		return "", err
	}
	for _, line := range lines {
		if code := strings.TrimSpace(picoclawPairCodePattern.FindString(line)); code != "" {
			return code, nil
		}
	}
	return "", nil
}

func daemonRequest(method, path string, body any) ([]byte, int, error) {
	return daemonRequestWithTimeout(method, path, body, 5*time.Minute)
}

func daemonActionTimeout(action string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "install":
		const (
			installTimeoutBuffer = 2 * time.Minute
			installTimeoutFloor  = 30 * time.Minute
		)
		timeout := daemonCommandTimeout() + installTimeoutBuffer
		if timeout < installTimeoutFloor {
			return installTimeoutFloor
		}
		return timeout
	default:
		return 5 * time.Minute
	}
}

func daemonCommandTimeout() time.Duration {
	const defaultDaemonCommandTimeout = 20 * time.Minute
	raw := strings.TrimSpace(os.Getenv("CARRIER_COMMAND_TIMEOUT"))
	if raw == "" {
		return defaultDaemonCommandTimeout
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return defaultDaemonCommandTimeout
	}
	return timeout
}

func daemonRequestWithTimeout(method, path string, body any, timeout time.Duration) ([]byte, int, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	base := strings.TrimRight(strings.TrimSpace(daemonProbeBaseURL()), "/")
	if base == "" {
		return nil, 0, errors.New("daemon base url is empty")
	}
	target := base + "/" + strings.TrimLeft(strings.TrimSpace(path), "/")

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	addDaemonAuthHeader(req)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, resp.StatusCode, nil
	}

	var errBody struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &errBody) == nil && strings.TrimSpace(errBody.Error.Message) != "" {
		return raw, resp.StatusCode, fmt.Errorf("daemon request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(errBody.Error.Message))
	}
	if msg := strings.TrimSpace(string(raw)); msg != "" {
		return raw, resp.StatusCode, fmt.Errorf("daemon request failed with status %d: %s", resp.StatusCode, msg)
	}
	return raw, resp.StatusCode, fmt.Errorf("daemon request failed with status %d", resp.StatusCode)
}

func gatewayRequest(method, path string, body any) ([]byte, int, error) {
	return gatewayRequestWithTimeout(method, path, body, 2*time.Minute)
}

func gatewayRequestWithTimeout(method, path string, body any, timeout time.Duration) ([]byte, int, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	base := strings.TrimRight(strings.TrimSpace(gatewayProbeBaseURL()), "/")
	if base == "" {
		return nil, 0, errors.New("gateway base url is empty")
	}
	target := base + "/" + strings.TrimLeft(strings.TrimSpace(path), "/")

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	addGatewayAuthHeader(req)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, resp.StatusCode, nil
	}

	var errBody struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &errBody) == nil && strings.TrimSpace(errBody.Error.Message) != "" {
		return raw, resp.StatusCode, fmt.Errorf("gateway request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(errBody.Error.Message))
	}
	if msg := strings.TrimSpace(string(raw)); msg != "" {
		return raw, resp.StatusCode, fmt.Errorf("gateway request failed with status %d: %s", resp.StatusCode, msg)
	}
	return raw, resp.StatusCode, fmt.Errorf("gateway request failed with status %d", resp.StatusCode)
}

func findAvailablePort(basePort, maxPort int) (int, error) {
	if basePort <= 0 || maxPort < basePort {
		return 0, errors.New("invalid port range")
	}
	for port := basePort; port <= maxPort; port++ {
		if portAvailableProbe(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port in range %d-%d", basePort, maxPort)
}

func isPortAvailable(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		// Some sandboxed CI/runtime environments deny socket bind attempts.
		// In that case, treat the port as available and keep deterministic allocation.
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			return true
		}
		return false
	}
	_ = l.Close()
	return true
}

func prepareManagedAgentAddArtifacts(agentID, instanceID, channelID, channelToken string, provider choiceOption, envVars map[string]string, pairedChatID string) (*managedAgentAddResult, error) {
	cfg, ok := managedAgentByID(agentID)
	if !ok {
		return nil, fmt.Errorf("managed agent %q is not supported", agentID)
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("%s instance id is required", cfg.ID)
	}
	channelID = strings.TrimSpace(channelID)
	channelToken = strings.TrimSpace(channelToken)
	pairedChatID = strings.TrimSpace(pairedChatID)
	if strings.TrimSpace(provider.ID) == "" {
		return nil, fmt.Errorf("%s provider is required", cfg.ID)
	}
	if err := ensureManagedAgentEnvRequirements(cfg.ID, envVars, provider); err != nil {
		return nil, err
	}

	home, err := resolveCarrierHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	workspacePath := filepath.Join(home, cfg.ConfigDir, "instances", instanceID, "workspace")
	configFileName := "config.json"
	if strings.EqualFold(cfg.ID, "openclaw") {
		configFileName = "openclaw.json"
	}
	configPath := filepath.Join(home, cfg.ConfigDir, configFileName)
	recordPath := filepath.Join(home, ".carrier", "agents", instanceID+".json")
	openClawSecretsPath := filepath.Join(home, ".openclaw", "carrier-secrets.json")
	allocatedPort, err := findAvailablePort(9090, 9190)
	if err != nil {
		return nil, fmt.Errorf("allocate port: %w", err)
	}

	if err := os.MkdirAll(workspacePath, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o700); err != nil {
		return nil, fmt.Errorf("create record dir: %w", err)
	}
	if err := backupIfExists(configPath); err != nil {
		return nil, fmt.Errorf("backup existing %s config: %w", cfg.ID, err)
	}

	modelID := strings.TrimSpace(provider.ExampleModel)
	if modelID == "" {
		modelID = provider.ID + "/default"
	}
	if catalog.IsOpenAICodexProviderID(provider.ID) {
		if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
			modelID = "openai/" + strings.TrimSpace(name)
		} else {
			modelID = "openai/gpt-5.3-codex"
		}
	}
	modelName := modelID
	if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
		modelName = strings.TrimSpace(name)
	}

	providerKey := provider.ID
	if vendor, _, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(vendor) != "" {
		providerKey = strings.TrimSpace(vendor)
	}
	providerKey = mapCarrierProviderToManagedProvider(providerKey)
	token := pickProviderTokenForManaged(provider, envVars)
	if catalog.IsOpenAICodexProviderID(provider.ID) && strings.EqualFold(cfg.ID, "picoclaw") {
		accountID := extractOpenAIAccountIDFromToken(token)
		if err := savePicoclawAuthCredential(home, "openai", token, accountID); err != nil {
			return nil, fmt.Errorf("write picoclaw auth store: %w", err)
		}
	}

	allowFrom := []string{}
	if pairedChatID != "" {
		allowFrom = []string{pairedChatID}
	}
	channelSetupPending := channelID != "" && channelToken == ""

	var payload map[string]interface{}
	if strings.EqualFold(cfg.ID, "openclaw") {
		payload = buildManagedOpenClawConfigPayload(
			channelID,
			channelToken,
			channelSetupPending,
			allowFrom,
			provider,
			providerKey,
			token,
			modelID,
			workspacePath,
		)
		if strings.TrimSpace(token) != "" {
			secrets := map[string]interface{}{
				"providers": map[string]interface{}{
					providerKey: map[string]interface{}{
						"apiKey": token,
					},
				},
			}
			secretsRaw, marshalErr := json.MarshalIndent(secrets, "", "  ")
			if marshalErr != nil {
				return nil, fmt.Errorf("marshal openclaw carrier secrets: %w", marshalErr)
			}
			if err := os.WriteFile(openClawSecretsPath, append(secretsRaw, '\n'), 0o600); err != nil {
				return nil, fmt.Errorf("write openclaw carrier secrets: %w", err)
			}
		}
	} else {
		payload = buildManagedPicoClawConfigPayload(
			channelID,
			channelToken,
			channelSetupPending,
			allowFrom,
			provider,
			providerKey,
			token,
			modelID,
			modelName,
			workspacePath,
		)
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s config: %w", cfg.ID, err)
	}
	if err := os.WriteFile(configPath, append(raw, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write %s config: %w", cfg.ID, err)
	}

	record := map[string]interface{}{
		"instance_id":    instanceID,
		"agent_id":       cfg.ID,
		"workspace_path": workspacePath,
		"config_path":    configPath,
		"port":           allocatedPort,
		"channel":        channelID,
		"provider":       provider.ID,
		"paired_chat_id": pairedChatID,
		"updated_at":     time.Now().UTC().Format(time.RFC3339Nano),
	}
	recordRaw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal carrier record: %w", err)
	}
	if err := os.WriteFile(recordPath, append(recordRaw, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write carrier record: %w", err)
	}

	return &managedAgentAddResult{
		InstanceID:    instanceID,
		WorkspacePath: workspacePath,
		ConfigPath:    configPath,
		RecordPath:    recordPath,
		ChannelID:     channelID,
		ProviderID:    provider.ID,
		PairedChatID:  pairedChatID,
		Port:          allocatedPort,
	}, nil
}

func buildManagedPicoClawConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider choiceOption,
	providerKey, providerToken, modelID, modelName, workspacePath string,
) map[string]interface{} {
	modelItem := map[string]interface{}{
		"model_name": modelName,
		"model":      modelID,
	}
	providerItem := map[string]interface{}{
		"credential_ref": provider.ID,
	}
	if catalog.IsOpenAICodexProviderID(provider.ID) {
		modelItem["auth_method"] = "oauth"
		providerItem["auth_method"] = "oauth"
	} else if providerToken != "" {
		providerItem["api_key"] = providerToken
	}

	channels := map[string]interface{}{}
	if strings.TrimSpace(channelID) != "" {
		channelConfig := map[string]interface{}{
			"enabled":    true,
			"allow_from": allowFrom,
		}
		if channelSetupPending {
			channelConfig["enabled"] = false
			channelConfig["setup_pending"] = true
		} else {
			channelConfig["token"] = channelToken
		}
		channels[channelID] = channelConfig
	}

	return map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace":             workspacePath,
				"provider":              providerKey,
				"model":                 modelName,
				"max_tokens":            8192,
				"temperature":           0.7,
				"max_tool_iterations":   20,
				"restrict_to_workspace": true,
			},
		},
		"model_list": []interface{}{modelItem},
		"providers": map[string]interface{}{
			providerKey: providerItem,
		},
		"channels": channels,
	}
}

func buildManagedOpenClawConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider choiceOption,
	providerKey, providerToken, modelID, workspacePath string,
) map[string]interface{} {
	return openclawcfg.BuildManagedConfigPayload(openclawcfg.ManagedPayloadParams{
		ChannelID:           channelID,
		ChannelToken:        channelToken,
		ChannelSetupPending: channelSetupPending,
		AllowFrom:           allowFrom,
		ProviderID:          provider.ID,
		ProviderKey:         providerKey,
		IncludeAPIKeyRef:    strings.TrimSpace(providerToken) != "",
		ModelID:             modelID,
		WorkspacePath:       workspacePath,
	})
}

func backupIfExists(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backupPath := fmt.Sprintf("%s.bak.%s", path, time.Now().UTC().Format("20060102T150405Z"))
	return os.Rename(path, backupPath)
}

func runRemoteStoreCommand(out io.Writer, opts remoteStoreCommandOptions) error {
	storePath, err := resolveRemoteControlStorePath()
	if err != nil {
		return err
	}
	switch opts.Action {
	case "backup":
		output := strings.TrimSpace(opts.OutputPath)
		if output == "" {
			output, err = defaultBackupOutputPath("remote-control")
			if err != nil {
				return err
			}
		}
		if err := copyFileSecure(storePath, output, 0o600); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Remote store backup saved to %s\n", output)
		return nil
	case "restore":
		raw, err := os.ReadFile(strings.TrimSpace(opts.FromPath))
		if err != nil {
			return fmt.Errorf("read backup file: %w", err)
		}
		if err := validateJSONBlob(raw, "remote-store backup payload must be valid JSON"); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
			return fmt.Errorf("create remote store directory: %w", err)
		}
		if err := backupIfExists(storePath); err != nil {
			return fmt.Errorf("backup existing remote store before restore: %w", err)
		}
		if err := os.WriteFile(storePath, raw, 0o600); err != nil {
			return fmt.Errorf("write restored remote store: %w", err)
		}
		_, _ = fmt.Fprintf(out, "Remote store restored from %s\n", strings.TrimSpace(opts.FromPath))
		return nil
	default:
		return fmt.Errorf("unsupported remote-store action: %s", opts.Action)
	}
}

func resolveRemoteControlStorePath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_REMOTE_CONTROL_STORE")); custom != "" {
		return custom, nil
	}
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for remote store: %w", err)
	}
	return filepath.Join(home, ".carrier", "remote-control.json"), nil
}

func defaultBackupOutputPath(prefix string) (string, error) {
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for backup output: %w", err)
	}
	dir := filepath.Join(home, ".carrier", "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	name := fmt.Sprintf("%s.%s.json", strings.TrimSpace(prefix), time.Now().UTC().Format("20060102T150405Z"))
	return filepath.Join(dir, name), nil
}

func copyFileSecure(sourcePath, destinationPath string, mode os.FileMode) error {
	raw, err := os.ReadFile(strings.TrimSpace(sourcePath))
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(strings.TrimSpace(destinationPath)), 0o700); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.WriteFile(strings.TrimSpace(destinationPath), raw, mode); err != nil {
		return fmt.Errorf("write destination file: %w", err)
	}
	return nil
}

func validateConfigBackup(raw []byte) error {
	if err := validateJSONBlob(raw, "config backup payload must be valid JSON"); err != nil {
		return err
	}
	var cfg configv2.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("decode config backup: %w", err)
	}
	if cfg.ConfigVersion != configv2.CurrentVersion {
		return fmt.Errorf("config backup version mismatch: got %d want %d", cfg.ConfigVersion, configv2.CurrentVersion)
	}
	return nil
}

func validateJSONBlob(raw []byte, message string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return errors.New("backup payload is empty")
	}
	var doc interface{}
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("invalid JSON payload: %w", err)
		}
		return fmt.Errorf("%s: %w", message, err)
	}
	return nil
}

func mapCarrierProviderToManagedProvider(providerID string) string {
	return catalog.MapToManagedProvider(providerID)
}

func pickProviderTokenForManaged(provider choiceOption, envVars map[string]string) string {
	if envVars == nil {
		return ""
	}
	if catalog.IsOpenAICodexProviderID(provider.ID) {
		for _, key := range []string{"OPENAI_CODEX_TOKEN", "OPENAI_API_KEY", provider.ProviderEnv} {
			if token := strings.TrimSpace(envVars[key]); token != "" {
				return token
			}
		}
		return ""
	}
	if provider.ProviderEnv == "" {
		return ""
	}
	return strings.TrimSpace(envVars[provider.ProviderEnv])
}

func managedAgentByID(agentID string) (managedAgentConfig, bool) {
	cfg, ok := managedAgents[strings.ToLower(strings.TrimSpace(agentID))]
	return cfg, ok
}

func managedAgentChannels(agentID string) ([]picoclawChannel, bool) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw":
		return picoclawChannels, true
	case "openclaw":
		return openclawChannels, true
	case "zeroclaw":
		return zeroclawChannels, true
	default:
		return nil, false
	}
}

func resolveManagedAgentChannel(agentID string) (picoclawChannel, bool) {
	channels, ok := managedAgentChannels(agentID)
	if !ok {
		return picoclawChannel{}, false
	}
	if len(channels) == 0 {
		return picoclawChannel{}, false
	}
	return channels[0], true
}

func isManagedAgent(agentID string) bool {
	_, ok := managedAgentByID(agentID)
	return ok
}

func ensureManagedAgentEnvRequirements(agentID string, envVars map[string]string, provider choiceOption) error {
	cfg, ok := managedAgentByID(agentID)
	if !ok {
		return fmt.Errorf("managed agent %q is not supported", agentID)
	}
	if envVars == nil {
		if cfg.RequiredEnvKey != "" {
			return fmt.Errorf("%s requires %s", cfg.ID, cfg.RequiredEnvKey)
		}
		return nil
	}
	if cfg.RequiredEnvKey != "" && strings.TrimSpace(envVars[cfg.RequiredEnvKey]) == "" {
		if token := pickProviderTokenForManaged(provider, envVars); token != "" {
			envVars[cfg.RequiredEnvKey] = token
		}
	}
	if cfg.RequiredEnvKey != "" && strings.TrimSpace(envVars[cfg.RequiredEnvKey]) == "" {
		if strings.EqualFold(cfg.ID, "openclaw") {
			return errors.New("openclaw requires OPENAI_API_KEY (select a provider with credentials)")
		}
		return fmt.Errorf("%s requires %s", cfg.ID, cfg.RequiredEnvKey)
	}
	if cfg.OptionalPopulateKey != "" && strings.TrimSpace(envVars[cfg.OptionalPopulateKey]) == "" {
		if token := pickProviderTokenForManaged(provider, envVars); token != "" {
			envVars[cfg.OptionalPopulateKey] = token
		}
	}
	return nil
}

type picoclawAuthCredential struct {
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id,omitempty"`
	Provider    string `json:"provider"`
	AuthMethod  string `json:"auth_method"`
}

type picoclawAuthStore struct {
	Credentials map[string]*picoclawAuthCredential `json:"credentials"`
}

func savePicoclawAuthCredential(home, providerID, accessToken, accountID string) error {
	if strings.TrimSpace(accessToken) == "" {
		return fmt.Errorf("empty access token")
	}
	path := filepath.Join(home, ".picoclaw", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}

	store := picoclawAuthStore{Credentials: map[string]*picoclawAuthCredential{}}
	if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 {
		if err := json.Unmarshal(existing, &store); err != nil {
			return fmt.Errorf("parse existing auth store: %w", err)
		}
	}
	if store.Credentials == nil {
		store.Credentials = map[string]*picoclawAuthCredential{}
	}
	store.Credentials[strings.TrimSpace(providerID)] = &picoclawAuthCredential{
		AccessToken: strings.TrimSpace(accessToken),
		AccountID:   strings.TrimSpace(accountID),
		Provider:    strings.TrimSpace(providerID),
		AuthMethod:  "oauth",
	}

	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth store: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}
	return nil
}

func extractOpenAIAccountIDFromToken(token string) string {
	claims, err := parseJWTClaims(token)
	if err != nil {
		return ""
	}
	if accountID, ok := claims["chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
		return strings.TrimSpace(accountID)
	}
	if accountID, ok := claims["https://api.openai.com/auth.chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
		return strings.TrimSpace(accountID)
	}
	if authClaim, ok := claims["https://api.openai.com/auth"].(map[string]interface{}); ok {
		if accountID, ok := authClaim["chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
			return strings.TrimSpace(accountID)
		}
	}
	return ""
}

func parseJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("token is not a JWT")
	}
	payload := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode jwt payload: %w", err)
		}
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("decode jwt claims: %w", err)
	}
	return claims, nil
}

func openBrowserURL(targetURL string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return errors.New("url is empty")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", targetURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
	default:
		return exec.Command("xdg-open", targetURL).Start()
	}
}

func pickMinimalProvider() (choiceOption, error) {
	provider, _, err := pickMinimalProviderWithReason()
	return provider, err
}

func pickMinimalProviderWithReason() (choiceOption, string, error) {
	defaultID := configuredDefaultProviderID()
	if defaultID != "" {
		if p, ok := resolveChoice(defaultID, providerOptions); ok {
			return p, fmt.Sprintf("config default from config.v2 default_model (provider=%s)", defaultID), nil
		}
	}
	if p, ok := resolveChoice("openai-codex", providerOptions); ok {
		return p, "fallback to openai-codex for minimal OAuth onboarding", nil
	}
	if len(providerOptions) == 0 {
		return choiceOption{}, "", errors.New("no provider available")
	}
	return providerOptions[0], "fallback to first available provider", nil
}

func promptMinimalChannelSelection(reader *bufio.Reader, out io.Writer) (choiceOption, bool, error) {
	if reader == nil {
		if channel, ok := resolveChoice("telegram", onboardChannelOptions); ok {
			return channel, true, nil
		}
		return choiceOption{}, false, errors.New("telegram channel is unavailable")
	}
	channelIDs := make([]string, 0, len(onboardChannelOptions))
	for _, ch := range onboardChannelOptions {
		channelIDs = append(channelIDs, ch.ID)
	}
	_, _ = fmt.Fprintf(out, "Type channel id to enable chat onboarding (%s), or press Enter for WebUI-only mode.\n", strings.Join(channelIDs, ", "))
	_, _ = fmt.Fprintf(out, "Channel id [%s/WebUI-only]: ", strings.Join(channelIDs, "/"))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return choiceOption{}, false, err
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return choiceOption{}, false, nil
	}
	channel, ok := resolveChoice(trimmed, onboardChannelOptions)
	if !ok {
		return choiceOption{}, false, fmt.Errorf("unknown channel %q", trimmed)
	}
	return channel, true, nil
}

func promptManagedChannelSelection(reader *bufio.Reader, out io.Writer, agentID string, quiet bool) (picoclawChannel, bool, error) {
	channels, ok := managedAgentChannels(agentID)
	if !ok || len(channels) == 0 {
		return picoclawChannel{}, false, nil
	}

	preferred := channels[0]
	if quiet {
		if token, _ := resolveManagedChannelToken(preferred.ID); token != "" {
			return preferred, true, nil
		}
		_, _ = fmt.Fprintln(out, "Channel: WebUI-only (no token available, quiet mode)")
		return picoclawChannel{}, false, nil
	}

	channelIDs := make([]string, 0, len(channels))
	for _, ch := range channels {
		channelIDs = append(channelIDs, ch.ID)
	}
	_, _ = fmt.Fprintf(out, "  Available channels: %s\n", strings.Join(channelIDs, ", "))
	_, _ = fmt.Fprintln(out, "  Type a channel ID to configure, or press Enter for WebUI-only mode.")
	_, _ = fmt.Fprintf(out, "  Channel [%s/WebUI-only]: ", strings.Join(channelIDs, "/"))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return picoclawChannel{}, false, err
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		_, _ = fmt.Fprintln(out, "WebUI-only mode selected.")
		_, _ = fmt.Fprintln(out, "No chat channel will be configured. Add one later via WebUI or `carrier config set`.")
		return picoclawChannel{}, false, nil
	}
	for _, ch := range channels {
		if strings.EqualFold(ch.ID, trimmed) {
			return ch, true, nil
		}
	}
	return picoclawChannel{}, false, fmt.Errorf("unknown channel %q for %s", trimmed, agentID)
}

func promptMinimalProviderOverride(reader *bufio.Reader, out io.Writer, selected choiceOption) (choiceOption, bool, error) {
	if reader == nil {
		return selected, false, nil
	}
	_, _ = fmt.Fprint(out, "Press Enter to keep this provider, or type another provider ID to override: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				return selected, false, nil
			}
			override, ok := resolveChoice(trimmed, providerOptions)
			if !ok {
				return choiceOption{}, false, fmt.Errorf("unknown provider override %q", trimmed)
			}
			return override, true, nil
		}
		return choiceOption{}, false, err
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return selected, false, nil
	}
	override, ok := resolveChoice(trimmed, providerOptions)
	if !ok {
		return choiceOption{}, false, fmt.Errorf("unknown provider override %q", trimmed)
	}
	return override, true, nil
}

func promptChannelCredentialsMinimal(reader *bufio.Reader, out io.Writer, channel choiceOption) (map[string]string, configv2.Channel, error) {
	env := make(map[string]string)
	cfg := configv2.Channel{
		ID:      channel.ID,
		Enabled: true,
	}
	token := ""
	if existingToken, source := resolveManagedChannelToken("telegram"); strings.TrimSpace(existingToken) != "" {
		reuse, err := promptYesNo(reader, out, fmt.Sprintf("Reuse existing Telegram token from %s", source), true)
		if err != nil {
			return nil, configv2.Channel{}, err
		}
		if reuse {
			token = strings.TrimSpace(existingToken)
		}
	}
	if token == "" {
		var err error
		token, err = promptInput(reader, out, "Telegram bot token (required, from @BotFather)", true)
		if err != nil {
			return nil, configv2.Channel{}, err
		}
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, configv2.Channel{}, errors.New("telegram bot token is required")
	}
	cfg.BotToken = token
	cfg.TransportMode = "auto"
	env["CARRIER_TELEGRAM_BOT_TOKEN"] = token
	env["CARRIER_TELEGRAM_TRANSPORT_MODE"] = "auto"
	return env, cfg, nil
}

func promptProviderAuthMinimal(reader *bufio.Reader, out io.Writer, provider choiceOption) (map[string]string, bool, error) {
	switch provider.AuthMode {
	case authModeAPIKey:
		reused, ok, err := maybeReuseSavedProviderCredential(reader, out, provider)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return reused, true, nil
		}
		label := fmt.Sprintf("%s API key", provider.Name)
		apiKey, err := promptInput(reader, out, label, true)
		if err != nil {
			return nil, false, err
		}
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return nil, false, fmt.Errorf("%s API key is required", provider.Name)
		}
		if err := saveProviderCredentialAuto(out, provider, apiKey); err != nil {
			return nil, false, err
		}
		return map[string]string{provider.ProviderEnv: apiKey}, true, nil
	case authModeOAuthDeviceCode:
		reused, ok, err := maybeReuseSavedProviderCredential(reader, out, provider)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return reused, true, nil
		}
		if catalog.IsOpenAICodexProviderID(provider.ID) {
			token, err := runOpenAICodexDeviceCodeFlow(out)
			if err != nil {
				return nil, false, fmt.Errorf("openai-codex device-code login: %w", err)
			}
			token = strings.TrimSpace(token)
			if token == "" {
				return nil, false, errors.New("openai-codex token is empty")
			}
			if err := saveProviderCredentialAuto(out, provider, token); err != nil {
				return nil, false, err
			}
			return map[string]string{provider.ProviderEnv: token}, true, nil
		}
		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintf(out, "%s uses OAuth device-code authorization.\n", provider.Name)
		_, _ = fmt.Fprintf(out, "Please complete the device-code flow in your auth tool, then paste %s.\n", provider.ProviderEnv)
		token, err := promptInput(reader, out, fmt.Sprintf("%s token (%s)", provider.Name, provider.ProviderEnv), true)
		if err != nil {
			return nil, false, err
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, false, fmt.Errorf("%s token is required", provider.Name)
		}
		if err := saveProviderCredentialAuto(out, provider, token); err != nil {
			return nil, false, err
		}
		return map[string]string{provider.ProviderEnv: token}, true, nil
	case authModeNone:
		_, _ = fmt.Fprintf(out, "%s does not require provider auth.\n", provider.Name)
		return nil, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported provider auth mode: %s", provider.AuthMode)
	}
}

func promptChannelCredentials(reader *bufio.Reader, out io.Writer, channel choiceOption) (map[string]string, configv2.Channel, error) {
	env := make(map[string]string)
	cfg := configv2.Channel{
		ID:      channel.ID,
		Enabled: true,
	}

	if channel.TokenEnv != "" {
		required := channel.RequireToken
		tokenLabel := fmt.Sprintf("%s access token", channel.Name)
		if channel.ID == "telegram" {
			tokenLabel = "Telegram bot token (required, from @BotFather)"
		}
		token, err := promptInput(reader, out, tokenLabel, required)
		if err != nil {
			return nil, configv2.Channel{}, err
		}
		if strings.TrimSpace(token) != "" {
			env[channel.TokenEnv] = strings.TrimSpace(token)
			cfg.BotToken = strings.TrimSpace(token)
		}
	}

	if channel.SecretEnv != "" {
		secretLabel := fmt.Sprintf("%s webhook secret (optional, press Enter to skip)", channel.Name)
		if channel.ID == "discord" {
			secretLabel = "Discord public key (optional, press Enter to skip)"
		}
		secret, err := promptInput(reader, out, secretLabel, false)
		if err != nil {
			return nil, configv2.Channel{}, err
		}
		if strings.TrimSpace(secret) != "" {
			env[channel.SecretEnv] = strings.TrimSpace(secret)
			cfg.WebhookSecret = strings.TrimSpace(secret)
		}
	}

	if channel.ID == "telegram" {
		mode, webhookURL, err := promptTelegramTransport(reader, out)
		if err != nil {
			return nil, configv2.Channel{}, err
		}
		cfg.TransportMode = mode
		cfg.WebhookURL = webhookURL
		if strings.TrimSpace(mode) != "" {
			env["CARRIER_TELEGRAM_TRANSPORT_MODE"] = mode
		}
		if strings.TrimSpace(webhookURL) != "" {
			env["CARRIER_TELEGRAM_WEBHOOK_URL"] = webhookURL
		}
	}
	return env, cfg, nil
}

func promptProviderAuth(reader *bufio.Reader, out io.Writer, provider choiceOption) (map[string]string, bool, error) {
	switch provider.AuthMode {
	case authModeAPIKey:
		reused, ok, err := maybeReuseSavedProviderCredential(reader, out, provider)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return reused, true, nil
		}

		label := fmt.Sprintf("%s API key", provider.Name)
		apiKey, err := promptInput(reader, out, label, true)
		if err != nil {
			return nil, false, err
		}
		cred := map[string]string{provider.ProviderEnv: apiKey}
		if err := saveProviderCredentialFlow(reader, out, provider, apiKey); err != nil {
			return nil, false, err
		}
		return cred, true, nil
	case authModeOAuthDeviceCode:
		reused, ok, err := maybeReuseSavedProviderCredential(reader, out, provider)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return reused, true, nil
		}

		if catalog.IsOpenAICodexProviderID(provider.ID) {
			token, err := runOpenAICodexDeviceCodeFlow(out)
			if err != nil {
				return nil, false, fmt.Errorf("openai-codex device-code login: %w", err)
			}
			cred := map[string]string{provider.ProviderEnv: token}
			if err := saveProviderCredentialAuto(out, provider, token); err != nil {
				return nil, false, err
			}
			return cred, true, nil
		}

		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintf(out, "%s uses OAuth device-code authorization.\n", provider.Name)
		_, _ = fmt.Fprintf(out, "Please complete the device-code flow in your auth tool, then paste %s.\n", provider.ProviderEnv)
		token, err := promptInput(reader, out, fmt.Sprintf("%s token (%s)", provider.Name, provider.ProviderEnv), true)
		if err != nil {
			return nil, false, err
		}
		cred := map[string]string{provider.ProviderEnv: token}
		if err := saveProviderCredentialAuto(out, provider, token); err != nil {
			return nil, false, err
		}
		return cred, true, nil
	case authModeNone:
		_, _ = fmt.Fprintf(out, "%s does not require provider auth.\n", provider.Name)
		return nil, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported provider auth mode: %s", provider.AuthMode)
	}
}

func maybeReuseSavedProviderCredential(reader *bufio.Reader, out io.Writer, provider choiceOption) (map[string]string, bool, error) {
	if strings.TrimSpace(provider.ProviderEnv) == "" {
		return nil, false, nil
	}
	value, backend, ok, err := loadProviderCredentialWithEnv(provider)
	if err != nil {
		_, _ = fmt.Fprintf(out, "Warning: failed to read saved credential for %s: %v\n", provider.Name, err)
		return nil, false, nil
	}
	if !ok {
		return nil, false, nil
	}
	reuse, err := promptYesNo(reader, out, fmt.Sprintf("Reuse saved %s credential from %s", provider.Name, backend), true)
	if err != nil {
		return nil, false, err
	}
	if !reuse {
		return nil, false, nil
	}
	return map[string]string{provider.ProviderEnv: value}, true, nil
}

func loadProviderCredentialWithEnv(provider choiceOption) (string, string, bool, error) {
	if envKey := strings.TrimSpace(provider.ProviderEnv); envKey != "" {
		if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
			return value, fmt.Sprintf("environment variable %s", envKey), true, nil
		}
	}
	value, backend, ok, err := loadProviderCredential(provider.ID)
	if err != nil || !ok {
		return "", "", false, err
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", "", false, nil
	}
	return trimmed, backend, true, nil
}

func saveProviderCredentialFlow(reader *bufio.Reader, out io.Writer, provider choiceOption, value string) error {
	save, err := promptYesNo(reader, out, fmt.Sprintf("Save %s credential for future reuse", provider.Name), true)
	if err != nil {
		return err
	}
	if !save {
		return nil
	}
	backend, err := saveProviderCredential(provider.ID, value)
	if err != nil {
		return fmt.Errorf("save credential: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Credential saved (%s).\n", backend)
	return nil
}

func saveProviderCredentialAuto(out io.Writer, provider choiceOption, value string) error {
	backend, err := saveProviderCredential(provider.ID, value)
	if err != nil {
		_, _ = fmt.Fprintf(out, "Warning: failed to save %s credential: %v\n", provider.Name, err)
		return nil
	}
	_, _ = fmt.Fprintf(out, "Credential saved (%s).\n", backend)
	return nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	for {
		_, _ = fmt.Fprintf(out, "%s %s: ", label, suffix)
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				text := strings.ToLower(strings.TrimSpace(line))
				if text == "" {
					return defaultYes, nil
				}
				switch text {
				case "y", "yes":
					return true, nil
				case "n", "no":
					return false, nil
				default:
					return false, errors.New("input interrupted")
				}
			}
			return false, err
		}
		text := strings.ToLower(strings.TrimSpace(line))
		if text == "" {
			return defaultYes, nil
		}
		switch text {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			_, _ = fmt.Fprintln(out, "Please answer yes or no.")
		}
	}
}

func promptInput(reader *bufio.Reader, out io.Writer, label string, required bool) (string, error) {
	for {
		_, _ = fmt.Fprintf(out, "%s: ", label)
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				value := strings.TrimSpace(line)
				if value != "" || !required {
					return value, nil
				}
				return "", errors.New("input interrupted")
			}
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" && required {
			_, _ = fmt.Fprintln(out, "This field is required, please retry.")
			continue
		}
		return value, nil
	}
}

func performOpenAICodexDeviceCodeFlow(out io.Writer) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	deviceAuthID, userCode, pollInterval, err := requestOpenAICodexUserCode(client)
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "OpenAI Codex device-code login")
	_, _ = fmt.Fprintln(out, "1. Open this URL in your browser:")
	_, _ = fmt.Fprintf(out, "   %s\n", openAICodexVerificationURL)
	_, _ = fmt.Fprintln(out, "2. Enter this one-time code:")
	_, _ = fmt.Fprintf(out, "   %s\n", userCode)
	_, _ = fmt.Fprintln(out, "Waiting for authorization (up to 15 minutes)...")

	authorizationCode, codeVerifier, err := pollOpenAICodexAuthorization(client, deviceAuthID, userCode, pollInterval, openAICodexDeviceAuthTimeout)
	if err != nil {
		return "", err
	}
	token, err := exchangeOpenAICodexToken(client, authorizationCode, codeVerifier)
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintln(out, "OpenAI Codex authorization completed.")
	return token, nil
}

func requestOpenAICodexUserCode(client *http.Client) (string, string, int, error) {
	status, body, err := postJSON(
		client,
		openAICodexAuthBaseURL+"/api/accounts/deviceauth/usercode",
		map[string]string{
			"client_id": openAICodexClientID,
		},
	)
	if err != nil {
		return "", "", 0, err
	}
	if status < 200 || status >= 300 {
		return "", "", 0, fmt.Errorf("request user code failed: status=%d body=%s", status, strings.TrimSpace(string(body)))
	}
	var resp struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Interval     string `json:"interval"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", 0, fmt.Errorf("decode user code response: %w", err)
	}
	deviceAuthID := strings.TrimSpace(resp.DeviceAuthID)
	userCode := strings.TrimSpace(resp.UserCode)
	if deviceAuthID == "" || userCode == "" {
		return "", "", 0, errors.New("device-code response missing device_auth_id or user_code")
	}
	pollInterval := openAICodexDefaultPollSeconds
	if n, err := strconv.Atoi(strings.TrimSpace(resp.Interval)); err == nil && n > 0 {
		pollInterval = n
	}
	return deviceAuthID, userCode, pollInterval, nil
}

func pollOpenAICodexAuthorization(client *http.Client, deviceAuthID, userCode string, pollIntervalSeconds int, timeout time.Duration) (string, string, error) {
	deadline := time.Now().Add(timeout)
	first := true

	for time.Now().Before(deadline) {
		if !first {
			time.Sleep(time.Duration(pollIntervalSeconds) * time.Second)
		}
		first = false

		status, body, err := postJSON(
			client,
			openAICodexAuthBaseURL+"/api/accounts/deviceauth/token",
			map[string]string{
				"device_auth_id": deviceAuthID,
				"user_code":      userCode,
			},
		)
		if err != nil {
			return "", "", err
		}
		if status >= 200 && status < 300 {
			var resp struct {
				AuthorizationCode string `json:"authorization_code"`
				CodeVerifier      string `json:"code_verifier"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return "", "", fmt.Errorf("decode deviceauth token response: %w", err)
			}
			authCode := strings.TrimSpace(resp.AuthorizationCode)
			verifier := strings.TrimSpace(resp.CodeVerifier)
			if authCode == "" || verifier == "" {
				return "", "", errors.New("deviceauth token response missing authorization_code or code_verifier")
			}
			return authCode, verifier, nil
		}
		if status == http.StatusForbidden || status == http.StatusNotFound {
			continue
		}
		return "", "", fmt.Errorf("deviceauth polling failed: status=%d body=%s", status, strings.TrimSpace(string(body)))
	}
	return "", "", errors.New("openai-codex device-code authorization timed out")
}

func exchangeOpenAICodexToken(client *http.Client, authorizationCode, codeVerifier string) (string, error) {
	status, body, err := postJSON(
		client,
		openAICodexAuthBaseURL+"/oauth/token",
		map[string]string{
			"grant_type":    "authorization_code",
			"client_id":     openAICodexClientID,
			"code":          authorizationCode,
			"code_verifier": codeVerifier,
			"redirect_uri":  openAICodexAuthBaseURL + "/deviceauth/callback",
		},
	)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("token exchange failed: status=%d body=%s", status, strings.TrimSpace(string(body)))
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode oauth token response: %w", err)
	}
	token := strings.TrimSpace(resp.AccessToken)
	if token == "" {
		return "", errors.New("oauth token response missing access_token")
	}
	return token, nil
}

func postJSON(client *http.Client, url string, payload any) (int, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal request payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response body: %w", err)
	}
	return resp.StatusCode, body, nil
}

func gatewayProbeBaseURL() string {
	host := strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_HOST"))
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_PORT"))
	if port == "" {
		port = "8787"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func checkGatewayHealth(baseURL string) bool {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return false
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get(base + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func daemonProbeBaseURL() string {
	host := strings.TrimSpace(os.Getenv("CARRIER_SERVER_HOST"))
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("CARRIER_SERVER_PORT"))
	if port == "" {
		port = "9090"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func checkDaemonHealth(baseURL string) bool {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return false
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	req, err := http.NewRequest(http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false
	}
	addDaemonAuthHeader(req)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitForDaemonHealthy(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if daemonHealthProbe(baseURL) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("daemon health probe timed out")
		}
		time.Sleep(daemonBootPollInterval)
	}
}

func waitForGatewayHealthy(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if gatewayHealthProbe(baseURL) {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("gateway health probe timed out")
		}
		time.Sleep(daemonBootPollInterval)
	}
}

func startDaemonInBackground() error {
	return startBackgroundSubprocess("daemon", "daemon")
}

func startGatewayInBackground() error {
	return startBackgroundSubprocess("gateway", "gateway")
}

func startBackgroundSubprocess(subcommand, logName string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	cmd := exec.Command(exePath, subcommand)
	cmd.Env = withResolvedHomeEnv(os.Environ())
	if bootstrapVerboseEnabled() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		logFile, _, err := openBootstrapLogFile(logName)
		if err != nil {
			return err
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			_ = logFile.Close()
			return err
		}
		_ = logFile.Close()
		return persistBackgroundProcess(logName, cmd.Process)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := persistBackgroundProcess(logName, cmd.Process); err != nil {
		return err
	}
	return nil
}

func persistBackgroundProcess(logName string, proc *os.Process) error {
	if proc == nil || proc.Pid <= 0 {
		return errors.New("background process is not available")
	}
	writeErr := writeBootstrapPIDFileFunc(logName, proc.Pid)
	if writeErr == nil {
		return nil
	}
	cleanupErr := terminateBackgroundProcessFunc(proc)
	if cleanupErr != nil {
		return fmt.Errorf("write bootstrap pid file: %w (cleanup failed: %v)", writeErr, cleanupErr)
	}
	return fmt.Errorf("write bootstrap pid file: %w", writeErr)
}

func terminateBackgroundProcess(proc *os.Process) error {
	if proc == nil || proc.Pid <= 0 {
		return nil
	}
	if stopped, stopErr := stopPID(proc.Pid); stopErr == nil && stopped {
		return nil
	}
	if killErr := proc.Kill(); killErr != nil && !isProcessAlreadyGone(killErr) {
		return killErr
	}
	return nil
}

func bootstrapVerboseEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("CARRIER_BOOTSTRAP_VERBOSE")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func bootstrapLogDir() (string, error) {
	if customDir := strings.TrimSpace(os.Getenv("CARRIER_BOOTSTRAP_LOG_DIR")); customDir != "" {
		if err := os.MkdirAll(customDir, 0o700); err != nil {
			return "", fmt.Errorf("create bootstrap log dir %s: %w", customDir, err)
		}
		return customDir, nil
	}
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for logs: %w", err)
	}
	dir := filepath.Join(home, ".carrier", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create bootstrap log dir %s: %w", dir, err)
	}
	return dir, nil
}

func bootstrapRunDir() (string, error) {
	if customDir := strings.TrimSpace(os.Getenv("CARRIER_BOOTSTRAP_RUN_DIR")); customDir != "" {
		if err := os.MkdirAll(customDir, 0o700); err != nil {
			return "", fmt.Errorf("create bootstrap run dir %s: %w", customDir, err)
		}
		return customDir, nil
	}
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for run dir: %w", err)
	}
	dir := filepath.Join(home, ".carrier", "run")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create bootstrap run dir %s: %w", dir, err)
	}
	return dir, nil
}

func bootstrapPIDPath(name string) (string, error) {
	dir, err := bootstrapRunDir()
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = "carrier"
	}
	return filepath.Join(dir, base+".pid"), nil
}

func mustBootstrapPIDPath(name string) string {
	path, err := bootstrapPIDPath(name)
	if err != nil {
		return ""
	}
	return path
}

func writeBootstrapPIDFile(name string, pid int) error {
	path, err := bootstrapPIDPath(name)
	if err != nil {
		return err
	}
	raw := []byte(fmt.Sprintf("%d\n", pid))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write pid file %s: %w", path, err)
	}
	return nil
}

func bootstrapLogPath(logName string) (string, error) {
	dir, err := bootstrapLogDir()
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(logName)
	if base == "" {
		base = "carrier"
	}
	return filepath.Join(dir, base+".log"), nil
}

func openBootstrapLogFile(logName string) (*os.File, string, error) {
	path, err := bootstrapLogPath(logName)
	if err != nil {
		return nil, "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("open log file %s: %w", path, err)
	}
	return file, path, nil
}

type daemonPairCodeRecord struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

func fetchDaemonPairCode(baseURL string) (string, string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", "", errors.New("daemon probe url is empty")
	}
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequest(http.MethodGet, base+"/api/v1/pairing/codes", nil)
	if err != nil {
		return "", "", err
	}
	addDaemonAuthHeader(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return "", "", readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("list pairing codes failed with status %d", resp.StatusCode)
	}

	var listed struct {
		Codes []daemonPairCodeRecord `json:"codes"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		return "", "", fmt.Errorf("decode pairing code list: %w", err)
	}
	for _, rec := range listed.Codes {
		if strings.TrimSpace(rec.Code) != "" {
			return strings.TrimSpace(rec.Code), strings.TrimSpace(rec.ExpiresAt), nil
		}
	}

	issueReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/pairing/codes", strings.NewReader(fmt.Sprintf(`{"ttlSeconds":%d}`, defaultPairCodeTTLSeconds)))
	if err != nil {
		return "", "", err
	}
	issueReq.Header.Set("Content-Type", "application/json")
	addDaemonAuthHeader(issueReq)
	issueResp, err := client.Do(issueReq)
	if err != nil {
		return "", "", err
	}
	issueBody, readIssueErr := io.ReadAll(issueResp.Body)
	_ = issueResp.Body.Close()
	if readIssueErr != nil {
		return "", "", readIssueErr
	}
	if issueResp.StatusCode < 200 || issueResp.StatusCode >= 300 {
		return "", "", fmt.Errorf("issue pairing code failed with status %d", issueResp.StatusCode)
	}
	var issued daemonPairCodeRecord
	if err := json.Unmarshal(issueBody, &issued); err != nil {
		return "", "", fmt.Errorf("decode issued pairing code: %w", err)
	}
	code := strings.TrimSpace(issued.Code)
	if code == "" {
		return "", "", errors.New("daemon returned empty pairing code")
	}
	return code, strings.TrimSpace(issued.ExpiresAt), nil
}

func addDaemonAuthHeader(req *http.Request) {
	if req == nil {
		return
	}
	if token := strings.TrimSpace(os.Getenv("CARRIER_SERVER_API_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func addGatewayAuthHeader(req *http.Request) {
	if req == nil {
		return
	}
	if token := strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_API_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func checkWebUIReady(gatewayBaseURL string) bool {
	base := strings.TrimRight(strings.TrimSpace(gatewayBaseURL), "/")
	if base == "" {
		return false
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(base + "/")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func isAddressInUseError(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if strings.Contains(strings.ToLower(opErr.Error()), "address already in use") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
}

func loadProviderCredential(providerID string) (string, string, bool, error) {
	return credentialstore.LoadProviderCredential(providerID)
}

func saveProviderCredential(providerID, value string) (string, error) {
	return credentialstore.SaveProviderCredential(providerID, value)
}

func mergeEnvVars(sets ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, set := range sets {
		for k, v := range set {
			merged[k] = v
		}
	}
	return merged
}

func applyEnvVars(vars map[string]string) error {
	for k, v := range vars {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("set env %s: %w", k, err)
		}
	}
	return nil
}

func promptChoice(reader *bufio.Reader, out io.Writer, options []choiceOption) (choiceOption, error) {
	if len(options) == 0 {
		return choiceOption{}, errors.New("no options available")
	}

	for {
		for i, opt := range options {
			_, _ = fmt.Fprintf(out, "  %d) %s (%s)\n", i+1, opt.Name, opt.Setup)
		}
		_, _ = fmt.Fprint(out, "Select by number or id: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return choiceOption{}, errors.New("input interrupted")
			}
			return choiceOption{}, err
		}

		selection, ok := resolveChoice(strings.TrimSpace(line), options)
		if ok {
			return selection, nil
		}
		_, _ = fmt.Fprintln(out, "Invalid selection, please retry.")
	}
}

func promptMultiChoice(reader *bufio.Reader, out io.Writer, options []choiceOption, label string) ([]choiceOption, error) {
	selected := make([]choiceOption, 0, len(options))
	selectedSet := make(map[string]struct{})
	for {
		for i, opt := range options {
			_, _ = fmt.Fprintf(out, "  %d) %s (%s)\n", i+1, opt.Name, opt.Setup)
		}
		if len(selected) > 0 {
			names := make([]string, 0, len(selected))
			for _, s := range selected {
				names = append(names, s.Name)
			}
			_, _ = fmt.Fprintf(out, "Selected %s(s): %s\n", label, strings.Join(names, ", "))
		}
		_, _ = fmt.Fprintf(out, "Select %s by number or id (`done` to finish): ", label)
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(selected) == 0 {
					return nil, fmt.Errorf("at least one %s is required", label)
				}
				return selected, nil
			}
			return nil, err
		}
		input := strings.ToLower(strings.TrimSpace(line))
		if input == "done" {
			if len(selected) == 0 {
				_, _ = fmt.Fprintf(out, "Please select at least one %s.\n", label)
				continue
			}
			return selected, nil
		}
		choice, ok := resolveChoice(input, options)
		if !ok {
			_, _ = fmt.Fprintln(out, "Invalid selection, please retry.")
			continue
		}
		if _, exists := selectedSet[choice.ID]; exists {
			_, _ = fmt.Fprintln(out, "Already selected, choose another or type `done`.")
			continue
		}
		selected = append(selected, choice)
		selectedSet[choice.ID] = struct{}{}
	}
}

func promptTelegramTransport(reader *bufio.Reader, out io.Writer) (mode string, webhookURL string, err error) {
	_, _ = fmt.Fprintln(out, "Telegram transport mode: auto (recommended), webhook, polling")
	rawMode, err := promptInput(reader, out, "Transport mode [auto/webhook/polling] (default auto)", false)
	if err != nil {
		return "", "", err
	}
	mode = strings.ToLower(strings.TrimSpace(rawMode))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "webhook", "polling":
	default:
		return "", "", fmt.Errorf("invalid transport mode %q", mode)
	}

	if mode == "auto" || mode == "webhook" {
		urlLabel := "Public webhook URL (optional for auto; required for webhook)"
		webhookURL, err = promptInput(reader, out, urlLabel, mode == "webhook")
		if err != nil {
			return "", "", err
		}
		webhookURL = strings.TrimSpace(webhookURL)
	}
	return mode, webhookURL, nil
}

func resolveChoice(input string, options []choiceOption) (choiceOption, bool) {
	if input == "" {
		return choiceOption{}, false
	}

	if n, err := strconv.Atoi(input); err == nil {
		if n >= 1 && n <= len(options) {
			return options[n-1], true
		}
		return choiceOption{}, false
	}

	normalizedInput := catalog.NormalizeProviderID(input)
	for _, opt := range options {
		if catalog.NormalizeProviderID(opt.ID) == normalizedInput {
			return opt, true
		}
		for _, alias := range opt.Aliases {
			if catalog.NormalizeProviderID(alias) == normalizedInput {
				return opt, true
			}
		}
	}
	return choiceOption{}, false
}

func onboardConfigPath(
	getenv func(string) string,
	userHomeDir func() (string, error),
) (string, error) {
	if path := strings.TrimSpace(getenv("CARRIER_CONFIG")); path != "" {
		return path, nil
	}
	if path := strings.TrimSpace(getenv("CARRIER_ONBOARD_CONFIG")); path != "" {
		return path, nil
	}
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".carrier", "config.v2.json"), nil
}

func resolveCarrierHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home, nil
	}
	if home, err := carrierUserHomeDirFunc(); err == nil {
		if trimmed := strings.TrimSpace(home); trimmed != "" {
			return trimmed, nil
		}
	}
	if current, err := carrierCurrentUserFunc(); err == nil && current != nil {
		if trimmed := strings.TrimSpace(current.HomeDir); trimmed != "" {
			return trimmed, nil
		}
	}

	if runtime.GOOS == "windows" {
		if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
			return profile, nil
		}
		if drive, homePath := strings.TrimSpace(os.Getenv("HOMEDRIVE")), strings.TrimSpace(os.Getenv("HOMEPATH")); drive != "" && homePath != "" {
			return filepath.Clean(drive + homePath), nil
		}
	} else {
		switch username := strings.TrimSpace(os.Getenv("USER")); username {
		case "root":
			return "/root", nil
		case "":
			return "/root", nil
		default:
			return filepath.Join("/home", username), nil
		}
	}

	return "", errors.New("home directory unavailable")
}

func withResolvedHomeEnv(env []string) []string {
	for _, entry := range env {
		if !strings.HasPrefix(entry, "HOME=") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(entry, "HOME=")) != "" {
			return env
		}
	}
	home, err := resolveCarrierHomeDir()
	if err != nil {
		return env
	}
	return append(env, "HOME="+home)
}

func buildSlashCommandGuide(channel choiceOption) string {
	return fmt.Sprintf(
		"Next step in %s:\n1. Open your bot chat and send `/pair <PAIR_CODE>`\n2. Then send `/agents`\n3. Use `carrier install <agent_id>` / `carrier onboard` in terminal, or open Carrier WebUI for guided setup\n4. Use `/start <agent_id>` to start installed agents",
		channel.Name,
	)
}
