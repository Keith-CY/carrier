package baseagent

import (
	"errors"
	"path/filepath"
	"strings"
)

type RiskLevel string

const (
	RiskLow  RiskLevel = "low"
	RiskHigh RiskLevel = "high"
)

type RepairAction struct {
	Command    string
	TargetPath string
	RiskLevel  RiskLevel
}

var (
	ErrRepairActionNotAllowed = errors.New("repair action is not in allowlist")
	ErrRepairActionNeedsConfirmation = errors.New("high-risk repair action requires confirmation")
)

type repairActionRule struct {
	name  string
	match func(RepairAction) bool
}

var allowlistedRepairActionRules = []repairActionRule{
	{
		name: "restart service",
		match: func(a RepairAction) bool {
			cmd := strings.TrimSpace(a.Command)
			if strings.HasPrefix(cmd, "systemctl restart ") {
				serviceName := strings.TrimPrefix(cmd, "systemctl restart ")
				return len(serviceName) > 0 && !strings.ContainsAny(serviceName, " ;|&`$()<>\n")
			}
			parts := strings.Fields(cmd)
			return len(parts) == 3 && parts[0] == "service" && parts[2] == "restart" && !strings.ContainsAny(parts[1], ";|&`$()<>\n")
		},
	},
	{
		name: "clear cache",
		match: func(a RepairAction) bool {
			cmd := strings.TrimSpace(a.Command)
			switch cmd {
			case "npm cache clean --force", "pip cache purge":
				return true
			default:
				return false
			}
		},
	},
	{
		name: "reinstall deps",
		match: func(a RepairAction) bool {
			cmd := strings.TrimSpace(a.Command)
			switch cmd {
			case "npm install", "yarn install", "pnpm install", "pip install -r requirements.txt", "go mod download":
				return true
			default:
				return false
			}
		},
	},
}

var systemDirectories = []string{
	"/bin",
	"/boot",
	"/dev",
	"/etc",
	"/lib",
	"/lib64",
	"/opt",
	"/proc",
	"/root",
	"/run",
	"/sbin",
	"/srv",
	"/sys",
	"/usr",
	"/var",
}

func IsRepairActionAllowlisted(action RepairAction) bool {
	// Also check with sudo stripped, so "sudo systemctl restart x" matches "systemctl restart x"
	stripped := action
	cmd := strings.TrimSpace(action.Command)
	if strings.HasPrefix(cmd, "sudo ") {
		stripped.Command = strings.TrimPrefix(cmd, "sudo ")
	}
	for _, rule := range allowlistedRepairActionRules {
		if rule.match(action) || rule.match(stripped) {
			return true
		}
	}
	return false
}

func ClassifyRepairActionRisk(action RepairAction) RiskLevel {
	if action.RiskLevel == RiskHigh {
		return RiskHigh
	}
	if usesSudo(action.Command) {
		return RiskHigh
	}
	if touchesSystemDirectory(action.TargetPath) {
		return RiskHigh
	}
	return RiskLow
}

func ValidateRepairAction(action RepairAction, confirmed bool) (RepairAction, error) {
	action.RiskLevel = ClassifyRepairActionRisk(action)
	if !IsRepairActionAllowlisted(action) {
		return action, ErrRepairActionNotAllowed
	}
	if action.RiskLevel == RiskHigh && !confirmed {
		return action, ErrRepairActionNeedsConfirmation
	}
	return action, nil
}

func usesSudo(command string) bool {
	parts := strings.Fields(command)
	if len(parts) > 0 && (parts[0] == "sudo" || filepath.Base(parts[0]) == "sudo") {
		return true
	}
	return false
}

func touchesSystemDirectory(targetPath string) bool {
	if strings.TrimSpace(targetPath) == "" {
		return false
	}
	cleanPath := filepath.Clean(targetPath)
	for _, dir := range systemDirectories {
		if cleanPath == dir || strings.HasPrefix(cleanPath, dir+"/") {
			return true
		}
	}
	return false
}
