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
//	carrier add <agent_id> [--tui|--cli]
//	                       Add/install an agent via terminal flow
//	carrier add <agent_id> --webui
//	                       Add/install an agent via WebUI flow
//	carrier install <agent_id> [--tui|--cli|--webui]
//	                       Alias for `carrier add <agent_id>`
//	carrier --help          Show usage
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type providerAuthMode string

const (
	authModeAPIKey          providerAuthMode = "api_key"
	authModeOAuthDeviceCode providerAuthMode = "oauth_device_code"
	authModeNone            providerAuthMode = "none"
)

type addCommandOptions struct {
	AgentID string
	WebUI   bool
	CLI     bool
	TUI     bool
	Quiet   bool
}

type onboardCommandOptions struct {
	WebUI bool
	CLI   bool
	TUI   bool
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

type remoteCommandOptions struct {
	Action             string
	AgentID            string
	InstallAgentID     string
	HostID             string
	HostName           string
	HostAddr           string
	Port               int
	User               string
	KeyPath            string
	RuntimeMode        string
	CheckRetries       int
	CheckRetryDelaySec int
	SkipReconnectCheck bool
	SyncChannels       []string
	SyncProviders      []string
	TelegramAllowFrom  []string
	DiscordAllowFrom   []string
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

var channelOptions = []choiceOption{
	{
		ID: "telegram", Name: "Telegram", Setup: "Easy (bot token)",
		TokenEnv: "CARRIER_TELEGRAM_BOT_TOKEN", RequireToken: true,
		SecretEnv: "CARRIER_TELEGRAM_WEBHOOK_SECRET",
	},
	{
		ID: "discord", Name: "Discord", Setup: "Easy (bot token + intents)",
		TokenEnv:  "CARRIER_DISCORD_BOT_TOKEN",
		SecretEnv: "CARRIER_DISCORD_PUBLIC_KEY",
	},
	{
		ID: "feishu", Name: "Feishu", Setup: "Medium (app credentials + webhook)",
		TokenEnv:  "CARRIER_FEISHU_APP_TOKEN",
		SecretEnv: "CARRIER_FEISHU_VERIFICATION_TOKEN",
	},
	{ID: "qq", Name: "QQ", Setup: "Easy (AppID + AppSecret)", TokenEnv: "CARRIER_QQ_BOT_TOKEN"},
	{ID: "dingtalk", Name: "DingTalk", Setup: "Medium (app credentials)", TokenEnv: "CARRIER_DINGTALK_BOT_TOKEN"},
	{ID: "line", Name: "LINE", Setup: "Medium (credentials + webhook URL)", TokenEnv: "CARRIER_LINE_BOT_TOKEN"},
	{ID: "wecom", Name: "WeCom", Setup: "Medium (CorpID + webhook setup)", TokenEnv: "CARRIER_WECOM_BOT_TOKEN"},
}

var onboardChannelOptions = channelOptions

var providerOptions = []choiceOption{
	{ID: "anthropic", Name: "Anthropic", Setup: "Claude direct API key", AuthMode: authModeAPIKey, ProviderEnv: "ANTHROPIC_API_KEY", ExampleModel: "anthropic/claude-sonnet-4.6"},
	{ID: "openai", Name: "OpenAI", Setup: "GPT direct API key", AuthMode: authModeAPIKey, ProviderEnv: "OPENAI_API_KEY", ExampleModel: "openai/gpt-5.2"},
	{ID: "openai-codex", Name: "OpenAI Codex", Setup: "OAuth device-code login", AuthMode: authModeOAuthDeviceCode, ProviderEnv: "OPENAI_CODEX_TOKEN", ExampleModel: "openai-codex/gpt-5.3-codex"},
	{ID: "openai-compatible", Name: "OpenAI-Compatible (v1)", Setup: "OpenAI v1-compatible endpoint", AuthMode: authModeNone, ExampleModel: "openai-compatible/auto", Aliases: []string{"vllm", "openai-v1"}},
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
)

var runOpenAICodexDeviceCodeFlow = performOpenAICodexDeviceCodeFlow
var gatewayHealthProbe = checkGatewayHealth
var daemonHealthProbe = checkDaemonHealth
var daemonBackgroundStarter = startDaemonInBackground
var gatewayBackgroundStarter = startGatewayInBackground
var runStopFlow = runStop
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
  carrier add <agent_id> [--tui|--cli|--webui] [-q|--quiet]
                        Add/install an agent (default: terminal flow; use -q for quiet mode)
  carrier install <agent_id> [--tui|--cli|--webui] [-q|--quiet]
                        Alias for carrier add <agent_id>
  carrier remote add <agent_id> --host-id <id> --host <ip-or-domain> --port <port> --user <ssh-user> --key-path <private-key-path> [options]
                        Deterministic remote install workflow via gateway API
                        agent_id: openclaw | picoclaw | zeroclaw
                        options:
                          [--name <display-name>]
                          [--runtime-mode <on_demand|managed_gateway>]
                          [--sync-channel <telegram|discord|feishu>]...
                          [--sync-provider <provider-id>]...
                          [--telegram-allow-from <id>]...
                          [--discord-allow-from <id>]...
                          [--check-retries <n>] [--check-retry-delay <seconds>]
                          [--skip-reconnect-check]
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
			if len(commandArgs) < 1 {
				fmt.Fprintln(os.Stderr, "status failed: instance id or name is required")
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runStatusInstance(os.Stdout, commandArgs[0]); err != nil {
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
		case "remote":
			opts, err := parseRemoteCommandArgs(commandArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "remote failed: %v\n\n", err)
				fmt.Fprint(os.Stderr, usage)
				os.Exit(1)
			}
			if err := runRemoteAddCommand(os.Stdin, os.Stdout, opts); err != nil {
				fmt.Fprintf(os.Stderr, "remote failed: %v\n", err)
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
				if err := runAddWebUI(os.Stdout, opts.AgentID); err != nil {
					fmt.Fprintf(os.Stderr, "%s failed: %v\n", command, err)
					os.Exit(1)
				}
				return
			}
			if err := runAddTUI(os.Stdin, os.Stdout, opts.AgentID, opts.Quiet); err != nil {
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
	case "onboard":
		return "onboard", args[2:], nil
	case "add":
		return "add", args[2:], nil
	case "install":
		return "install", args[2:], nil
	case "remote":
		return "remote", args[2:], nil
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
		return addCommandOptions{}, errors.New("usage: carrier add <agent_id> [--tui|--cli|--webui] [-q|--quiet] (alias: carrier install <agent_id>)")
	}

	opts := addCommandOptions{}
	for _, raw := range args {
		arg := strings.ToLower(strings.TrimSpace(raw))
		switch arg {
		case "--webui", "--web", "webui":
			opts.WebUI = true
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

func parseRemoteCommandArgs(args []string) (remoteCommandOptions, error) {
	if len(args) < 2 {
		return remoteCommandOptions{}, errors.New("usage: carrier remote add <agent_id> --host-id <id> --host <ip-or-domain> --port <port> --user <ssh-user> --key-path <private-key-path> [options]")
	}

	opts := remoteCommandOptions{
		Action:             strings.ToLower(strings.TrimSpace(args[0])),
		AgentID:            strings.ToLower(strings.TrimSpace(args[1])),
		Port:               22,
		RuntimeMode:        "on_demand",
		CheckRetries:       10,
		CheckRetryDelaySec: 2,
		SyncChannels:       []string{},
		SyncProviders:      []string{},
		TelegramAllowFrom:  []string{},
		DiscordAllowFrom:   []string{},
	}
	if opts.Action != "add" {
		return remoteCommandOptions{}, fmt.Errorf("unsupported remote action: %s", opts.Action)
	}
	switch opts.AgentID {
	case "openclaw", "picoclaw", "zeroclaw":
	default:
		return remoteCommandOptions{}, fmt.Errorf("unsupported remote agent_id: %s (expected one of openclaw, picoclaw, zeroclaw)", opts.AgentID)
	}
	// OpenClaw runtime install endpoint uses the default agent slot "main".
	if opts.AgentID == "openclaw" {
		opts.InstallAgentID = "main"
	} else {
		opts.InstallAgentID = opts.AgentID
	}

	for i := 2; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		if raw == "" {
			continue
		}
		switch strings.ToLower(raw) {
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
		case "--runtime-mode":
			value, next, err := parseRequiredFlagValue(args, i, "--runtime-mode")
			if err != nil {
				return remoteCommandOptions{}, err
			}
			opts.RuntimeMode = strings.ToLower(strings.TrimSpace(value))
			i = next
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
		default:
			return remoteCommandOptions{}, fmt.Errorf("unknown remote option: %s", raw)
		}
	}

	if opts.HostID == "" {
		return remoteCommandOptions{}, errors.New("--host-id is required")
	}
	if opts.HostName == "" {
		opts.HostName = opts.HostID
	}
	if opts.HostAddr == "" {
		return remoteCommandOptions{}, errors.New("--host is required")
	}
	if opts.User == "" {
		return remoteCommandOptions{}, errors.New("--user is required")
	}
	if opts.KeyPath == "" {
		return remoteCommandOptions{}, errors.New("--key-path is required")
	}
	switch opts.RuntimeMode {
	case "on_demand", "managed_gateway":
	default:
		return remoteCommandOptions{}, fmt.Errorf("invalid --runtime-mode value: %s", opts.RuntimeMode)
	}
	for _, channel := range opts.SyncChannels {
		switch channel {
		case "telegram", "discord", "feishu":
		default:
			return remoteCommandOptions{}, fmt.Errorf("invalid --sync-channel value: %s", channel)
		}
	}
	for _, provider := range opts.SyncProviders {
		if strings.TrimSpace(provider) == "" {
			return remoteCommandOptions{}, errors.New("--sync-provider cannot be empty")
		}
	}
	return opts, nil
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
	enabledChannels := 0
	for _, ch := range cfg.Channels {
		if ch.Enabled {
			enabledChannels++
		}
	}
	if enabledChannels == 0 {
		return true
	}
	if len(cfg.ModelList) == 0 {
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
	if err := daemonAgentActionWithProgress(out, inst.AgentID, "start", false); err != nil {
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
		_, _ = fmt.Fprintf(out, "- id=%s name=%s type=%s state=%s gateway=%s\n",
			strings.TrimSpace(inst.ID),
			managedInstanceDisplayName(inst),
			strings.TrimSpace(inst.Type),
			runtimeState,
			strings.TrimSpace(inst.GatewayURL),
		)
	}
	return nil
}

type remoteHostCheckResponse struct {
	Check struct {
		SSHOK         bool `json:"sshOk"`
		OpenClawFound bool `json:"openclawFound"`
	} `json:"check"`
	Instances                []remoteInstanceSummary `json:"instances"`
	PendingPullInstances     []remoteInstanceSummary `json:"pendingPullInstances"`
	PullConfirmationRequired bool                    `json:"pullConfirmationRequired"`
}

type remoteInstancesListResponse struct {
	Instances []remoteInstanceSummary `json:"instances"`
}

type remoteInstanceSummary struct {
	ID      string `json:"id"`
	AgentID string `json:"agentId"`
}

func runRemoteAddCommand(in io.Reader, out io.Writer, opts remoteCommandOptions) error {
	if opts.Action != "add" {
		return fmt.Errorf("unsupported remote action: %s", opts.Action)
	}
	agentName := remoteAgentDisplayName(opts.AgentID)
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
		"authMode":    "private_key",
		"keyPath":     opts.KeyPath,
		"runtimeMode": opts.RuntimeMode,
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

	printRemoteStep(out, 3, 8, fmt.Sprintf("Install %s on remote host", agentName))
	if err := runRemoteInstallStream(out, opts.HostID, opts.InstallAgentID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "  %s installation stream completed successfully.\n", agentName)

	if len(opts.SyncChannels) > 0 || len(opts.SyncProviders) > 0 {
		printRemoteStep(out, 4, 8, "Sync selected local configuration")
		if err := runRemoteSelectedConfigSync(opts); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "  Local configuration sync completed.")
	} else {
		printRemoteStep(out, 4, 8, "Sync selected local configuration")
		_, _ = fmt.Fprintln(out, "  Skipped (no --sync-channel/--sync-provider provided).")
	}

	printRemoteStep(out, 5, 8, "Post-install health check")
	postCheck, err := runRemoteHostCheckWithRetry(opts.HostID, opts.CheckRetries, opts.CheckRetryDelaySec, false, nil)
	if err != nil {
		return err
	}
	postCheck, err = maybeConfirmPullForPendingInstances(in, out, opts.HostID, postCheck)
	if err != nil {
		return err
	}
	printRemoteCheckSummary(out, postCheck, "")

	printRemoteStep(out, 6, 8, "List remote instances")
	if err := printRemoteInstances(out, opts.HostID, "Detected instances"); err != nil {
		return err
	}

	if opts.SkipReconnectCheck {
		printRemoteStep(out, 7, 8, "Reconnect simulation skipped (--skip-reconnect-check)")
		printRemoteStep(out, 8, 8, "Reconnect verification skipped (--skip-reconnect-check)")
		_, _ = fmt.Fprintln(out, colorizeForTTY(out, fmt.Sprintf("Completed: %s remote install finished for host %s.", agentName, opts.HostID), ansiGreenBold))
		return nil
	}

	printRemoteStep(out, 7, 8, "Reconnect simulation (remove host record and re-register)")
	if _, _, err := gatewayRequestWithTimeout(http.MethodDelete, "/api/v1/remote/hosts/"+neturl.PathEscape(opts.HostID), nil, 30*time.Second); err != nil {
		return err
	}
	if _, _, err := gatewayRequestWithTimeout(http.MethodPost, "/api/v1/remote/hosts", hostPayload, 30*time.Second); err != nil {
		return err
	}

	printRemoteStep(out, 8, 8, "Reconnect verification and instance refresh")
	reconnectCheck, err := runRemoteHostCheckWithRetry(opts.HostID, opts.CheckRetries, opts.CheckRetryDelaySec, false, nil)
	if err != nil {
		return err
	}
	reconnectCheck, err = maybeConfirmPullForPendingInstances(in, out, opts.HostID, reconnectCheck)
	if err != nil {
		return err
	}
	printRemoteCheckSummary(out, reconnectCheck, "Reconnect verification")
	if err := printRemoteInstances(out, opts.HostID, "Instances after reconnect"); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, colorizeForTTY(out, fmt.Sprintf("Completed: %s is installed on host %s and reconnect verification passed.", agentName, opts.HostID), ansiGreenBold))
	return nil
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

func runRemoteInstallStream(out io.Writer, hostID, agentID string) error {
	base := strings.TrimRight(strings.TrimSpace(gatewayProbeBaseURL()), "/")
	if base == "" {
		return errors.New("gateway base url is empty")
	}
	path := fmt.Sprintf("%s/api/v1/remote/hosts/%s/instances/%s/install/stream", base, neturl.PathEscape(strings.TrimSpace(hostID)), neturl.PathEscape(strings.TrimSpace(agentID)))
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewBufferString("{}"))
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
	if strings.EqualFold(opts.AgentID, "zeroclaw") {
		return buildZeroClawRemoteConfigPatch(opts, cfg)
	}
	return buildJSONRemoteConfigPatch(opts, cfg)
}

func buildJSONRemoteConfigPatch(opts remoteCommandOptions, cfg *configv2.Config) (map[string]interface{}, error) {
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
		if !strings.EqualFold(strings.TrimSpace(model.ProviderID), "openai-codex") {
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
	if strings.EqualFold(strings.TrimSpace(model.ProviderID), "openai-codex") {
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

func runAddWebUI(out io.Writer, agentID string) error {
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

func runAddTUI(in io.Reader, out io.Writer, agentID string, quiet bool) error {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return errors.New("agent_id is required")
	}
	if !isManagedAgent(agentID) {
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
	return runAddManagedAgentTUI(in, out, agentID, quiet)
}

func runAddManagedAgentTUI(in io.Reader, out io.Writer, agentID string, quiet bool) error {
	cfg, ok := managedAgentByID(agentID)
	if !ok {
		return fmt.Errorf("managed agent %q is not supported", agentID)
	}
	reader := bufio.NewReader(in)
	_, _ = fmt.Fprintln(out, "Carrier Add (TUI)")
	_, _ = fmt.Fprintln(out, "-----------------")
	_, _ = fmt.Fprintf(out, "Agent: %s\n", cfg.Name)
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
	channel, ok := resolveManagedAgentChannel(cfg.ID)
	if !ok {
		return fmt.Errorf("%s channel is unavailable", cfg.ID)
	}
	_, _ = fmt.Fprintf(out, "Using channel: %s (default)\n", channel.Name)
	reuseChannelToken := managedAddReusesChannelToken(cfg.ID, channel.ID)
	token := ""
	tokenSource := ""
	if reuseChannelToken {
		token, tokenSource = resolveManagedChannelToken(channel.ID)
		if tokenSource != "" {
			_, _ = fmt.Fprintf(out, "Reused %s token from %s.\n", channel.Name, tokenSource)
		}
	}
	if tokenSource == "" {
		if !reuseChannelToken {
			_, _ = fmt.Fprintf(out, "Token reuse is disabled for %s to avoid shared bot conflicts.\n", cfg.Name)
		}
		token, err = promptInput(reader, out, channel.TokenLabel, true)
		if err != nil {
			return err
		}
	}
	pairedChatID, pairedChatIDSource := latestManagedPairedChatID(cfg.ID, channel.ID)
	if pairedChatIDSource != "" {
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
	result, err := prepareManagedAgentAddArtifacts(cfg.ID, instanceID, channel.ID, token, provider, envVars, pairedChatID)
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
	if err := daemonAgentActionWithProgress(out, cfg.ID, "install", quiet); err != nil {
		return err
	}
	if err := daemonAgentActionWithProgress(out, cfg.ID, "start", quiet); err != nil {
		return err
	}
	if strings.EqualFold(cfg.ID, "picoclaw") {
		if pairCode, _ := daemonExtractPairCodeFromLogs(cfg.ID); strings.TrimSpace(pairCode) != "" {
			_, _ = fmt.Fprintf(out, "PicoClaw pair code: %s\n", pairCode)
			_, _ = fmt.Fprintf(out, "Next: send `/pair %s` in your PicoClaw Telegram bot chat.\n", pairCode)
		} else {
			_, _ = fmt.Fprintln(out, "Pair code not detected yet. Open PicoClaw Telegram bot chat and follow `/start` -> `/pair` prompts.")
		}
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
		GatewayURL:   gatewayProbeBaseURL(),
		Workspace:    result.WorkspacePath,
		ConfigPath:   result.ConfigPath,
		RecordPath:   result.RecordPath,
		Channel:      result.ChannelID,
		Provider:     result.ProviderID,
		PairRequired: strings.TrimSpace(result.PairedChatID) == "",
		PairedChatID: result.PairedChatID,
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
	return strings.TrimSpace(instances[bestIdx].Provider)
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
	if quiet {
		return daemonAgentAction(agentID, action)
	}

	previousLogs, _ := daemonFetchAgentLogs(agentID, 1000)
	startedAt := time.Now()
	lastHeartbeat := startedAt

	done := make(chan error, 1)
	go func() {
		done <- daemonAgentAction(agentID, action)
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
	agentID = strings.TrimSpace(agentID)
	action = strings.TrimSpace(action)
	if agentID == "" || action == "" {
		return errors.New("agentID and action are required")
	}
	path := fmt.Sprintf("/api/v1/agents/%s/%s", neturl.PathEscape(agentID), neturl.PathEscape(action))
	_, status, err := daemonRequestWithTimeout(http.MethodPost, path, map[string]string{}, daemonActionTimeout(action))
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
	if channelID == "" {
		return nil, fmt.Errorf("%s channel is required", cfg.ID)
	}
	if channelToken == "" {
		return nil, fmt.Errorf("%s channel token is required", cfg.ID)
	}
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
	configPath := filepath.Join(home, cfg.ConfigDir, "config.json")
	recordPath := filepath.Join(home, ".carrier", "agents", instanceID+".json")

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
	if strings.EqualFold(provider.ID, "openai-codex") {
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

	modelItem := map[string]interface{}{
		"model_name": modelName,
		"model":      modelID,
	}
	providerItem := map[string]interface{}{
		"credential_ref": provider.ID,
	}
	token := pickProviderTokenForManaged(provider, envVars)
	if strings.EqualFold(provider.ID, "openai-codex") {
		modelItem["auth_method"] = "oauth"
		providerItem["auth_method"] = "oauth"
		if strings.EqualFold(cfg.ID, "picoclaw") {
			accountID := extractOpenAIAccountIDFromToken(token)
			if err := savePicoclawAuthCredential(home, "openai", token, accountID); err != nil {
				return nil, fmt.Errorf("write picoclaw auth store: %w", err)
			}
		}
	} else if token != "" {
		providerItem["api_key"] = token
	}

	allowFrom := []string{}
	if pairedChatID != "" {
		allowFrom = []string{pairedChatID}
	}

	payload := map[string]interface{}{
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
		"channels": map[string]interface{}{
			channelID: map[string]interface{}{
				"enabled":    true,
				"token":      channelToken,
				"allow_from": allowFrom,
			},
		},
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
	}, nil
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

func mapCarrierProviderToManagedProvider(providerID string) string {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai-codex":
		return "openai"
	case "openai-compatible", "vllm", "openai-v1":
		return "openai"
	default:
		return strings.TrimSpace(providerID)
	}
}

func pickProviderTokenForManaged(provider choiceOption, envVars map[string]string) string {
	if envVars == nil {
		return ""
	}
	if strings.EqualFold(provider.ID, "openai-codex") {
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
	_, _ = fmt.Fprintln(out, "Type channel id to enable chat onboarding (telegram), or press Enter for WebUI-only mode.")
	_, _ = fmt.Fprint(out, "Channel id [telegram/WebUI-only]: ")
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
		if provider.ID == "openai-codex" {
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

		if provider.ID == "openai-codex" {
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

	for _, opt := range options {
		if strings.EqualFold(opt.ID, input) {
			return opt, true
		}
		for _, alias := range opt.Aliases {
			if strings.EqualFold(alias, input) {
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
