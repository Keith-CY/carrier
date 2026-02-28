package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type upstreamCompatLock struct {
	SchemaVersion int                            `json:"schema_version"`
	UpdatedAt     string                         `json:"updated_at,omitempty"`
	Agents        map[string]upstreamAgentCompat `json:"agents"`
}

type upstreamAgentCompat struct {
	Repository          string                 `json:"repository"`
	RecommendedVersion  string                 `json:"recommended_version"`
	SupportedRenderers  []upstreamRendererSpec `json:"supported_renderers"`
	TrackedFiles        []upstreamTrackedFile  `json:"tracked_files,omitempty"`
	ExpectedFingerprint string                 `json:"expected_fingerprint,omitempty"`
}

type upstreamRendererSpec struct {
	ID           string `json:"id"`
	VersionRange string `json:"version_range"`
	ConfigFormat string `json:"config_format"`
	ConfigPath   string `json:"config_path"`
}

type upstreamTrackedFile struct {
	Path string `json:"path"`
	SHA  string `json:"sha"`
}

type managedRendererSelection struct {
	AgentID             string
	RendererID          string
	ConfigFormat        string
	ConfigPath          string
	AgentVersion        string
	VersionSource       string
	Repository          string
	ExpectedFingerprint string
	VersionRange        string
}

type semVer struct {
	Major int
	Minor int
	Patch int
}

var (
	semVerTextPattern  = regexp.MustCompile(`(?i)\bv?(\d+)\.(\d+)\.(\d+)\b`)
	semVerExactPattern = regexp.MustCompile(`(?i)^v?(\d+)\.(\d+)\.(\d+)$`)
	// rangeTokenPattern matches version range tokens like ">=1.2.3", "<2.0.0", or bare "1.2.3".
	// Note: a bare version (no operator prefix) is treated as exact equality ("="),
	// not as ">=" (npm-style). This is intentional for deterministic managed config rendering.
	rangeTokenPattern = regexp.MustCompile(`^(>=|<=|>|<|=)?v?(\d+\.\d+\.\d+)$`)

	managedCompatLockOnce sync.Once
	managedCompatLock     upstreamCompatLock
)

func defaultUpstreamCompatLock() upstreamCompatLock {
	return upstreamCompatLock{
		SchemaVersion: 1,
		Agents: map[string]upstreamAgentCompat{
			"openclaw": {
				Repository:         "openclaw/openclaw",
				RecommendedVersion: "1.0.0",
				SupportedRenderers: []upstreamRendererSpec{
					{ID: "openclaw.json.v1", VersionRange: ">=0.0.0", ConfigFormat: "json", ConfigPath: "~/.openclaw/openclaw.json"},
				},
			},
			"picoclaw": {
				Repository:         "sipeed/picoclaw",
				RecommendedVersion: "0.1.2",
				SupportedRenderers: []upstreamRendererSpec{
					{ID: "picoclaw.json.v1", VersionRange: ">=0.1.0 <1.0.0", ConfigFormat: "json", ConfigPath: "~/.picoclaw/config.json"},
				},
			},
			"zeroclaw": {
				Repository:         "zeroclaw-labs/zeroclaw",
				RecommendedVersion: "0.1.7",
				SupportedRenderers: []upstreamRendererSpec{
					{ID: "zeroclaw.toml.v1", VersionRange: ">=0.1.0 <1.0.0", ConfigFormat: "toml", ConfigPath: "~/.zeroclaw/config.toml"},
				},
			},
		},
	}
}

func loadManagedCompatLock() upstreamCompatLock {
	managedCompatLockOnce.Do(func() {
		lock := defaultUpstreamCompatLock()
		if fromDisk, err := readUpstreamCompatLockFromDisk(); err == nil && fromDisk != nil {
			if normalized, ok := normalizeUpstreamCompatLock(*fromDisk); ok {
				for agentID, cfg := range normalized.Agents {
					lock.Agents[agentID] = cfg
				}
				lock.SchemaVersion = normalized.SchemaVersion
				if strings.TrimSpace(normalized.UpdatedAt) != "" {
					lock.UpdatedAt = strings.TrimSpace(normalized.UpdatedAt)
				}
				managedCompatLock = lock
				return
			}
		}
		managedCompatLock = lock
	})
	return managedCompatLock
}

func readUpstreamCompatLockFromDisk() (*upstreamCompatLock, error) {
	for _, candidate := range upstreamCompatLockCandidates() {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		raw, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		var lock upstreamCompatLock
		if err := json.Unmarshal(raw, &lock); err != nil {
			continue
		}
		return &lock, nil
	}
	return nil, fmt.Errorf("compat lock not found")
}

func normalizeUpstreamCompatLock(lock upstreamCompatLock) (upstreamCompatLock, bool) {
	if lock.SchemaVersion <= 0 {
		lock.SchemaVersion = 1
	}
	if lock.Agents == nil {
		return upstreamCompatLock{}, false
	}
	normalized := upstreamCompatLock{
		SchemaVersion: lock.SchemaVersion,
		UpdatedAt:     strings.TrimSpace(lock.UpdatedAt),
		Agents:        map[string]upstreamAgentCompat{},
	}
	for rawAgentID, cfg := range lock.Agents {
		agentID := strings.ToLower(strings.TrimSpace(rawAgentID))
		if agentID == "" {
			continue
		}
		if strings.TrimSpace(cfg.Repository) == "" || len(cfg.SupportedRenderers) == 0 {
			continue
		}
		for i := range cfg.SupportedRenderers {
			r := &cfg.SupportedRenderers[i]
			r.ID = strings.TrimSpace(r.ID)
			r.VersionRange = strings.TrimSpace(r.VersionRange)
			r.ConfigFormat = strings.ToLower(strings.TrimSpace(r.ConfigFormat))
			r.ConfigPath = strings.TrimSpace(r.ConfigPath)
			if r.ID == "" || r.VersionRange == "" || r.ConfigFormat == "" {
				return upstreamCompatLock{}, false
			}
		}
		normalized.Agents[agentID] = cfg
	}
	if len(normalized.Agents) == 0 {
		return upstreamCompatLock{}, false
	}
	return normalized, true
}

func upstreamCompatLockCandidates() []string {
	candidates := []string{}
	if custom := strings.TrimSpace(os.Getenv("CARRIER_UPSTREAM_LOCK_PATH")); custom != "" {
		candidates = append(candidates, custom)
	}
	candidates = append(candidates,
		filepath.Join("compat", "upstreams.lock.json"),
		filepath.Join("..", "compat", "upstreams.lock.json"),
	)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "compat", "upstreams.lock.json"),
			filepath.Join(exeDir, "..", "compat", "upstreams.lock.json"),
		)
	}
	return candidates
}

func resolveManagedRenderer(agentID string) (managedRendererSelection, error) {
	trimmedAgentID := strings.ToLower(strings.TrimSpace(agentID))
	lock := loadManagedCompatLock()
	compat, ok := lock.Agents[trimmedAgentID]
	if !ok {
		return managedRendererSelection{}, fmt.Errorf("no compatibility policy found for %s", trimmedAgentID)
	}
	version, source, err := resolveManagedAgentVersion(trimmedAgentID, compat.RecommendedVersion)
	if err != nil {
		return managedRendererSelection{}, err
	}

	for _, renderer := range compat.SupportedRenderers {
		match, rangeErr := semverSatisfiesRange(version, renderer.VersionRange)
		if rangeErr != nil {
			continue
		}
		if !match {
			continue
		}
		return managedRendererSelection{
			AgentID:             trimmedAgentID,
			RendererID:          renderer.ID,
			ConfigFormat:        renderer.ConfigFormat,
			ConfigPath:          renderer.ConfigPath,
			AgentVersion:        version,
			VersionSource:       source,
			Repository:          compat.Repository,
			ExpectedFingerprint: compat.ExpectedFingerprint,
			VersionRange:        renderer.VersionRange,
		}, nil
	}

	ranges := make([]string, 0, len(compat.SupportedRenderers))
	for _, renderer := range compat.SupportedRenderers {
		ranges = append(ranges, renderer.VersionRange)
	}
	return managedRendererSelection{}, fmt.Errorf(
		"unsupported %s version %s (%s), supported ranges: %s",
		trimmedAgentID,
		version,
		source,
		strings.Join(ranges, ", "),
	)
}

func resolveManagedAgentVersion(agentID, recommended string) (string, string, error) {
	envKey := fmt.Sprintf("CARRIER_MANAGED_%s_VERSION", strings.ToUpper(strings.ReplaceAll(agentID, "-", "_")))
	if override := strings.TrimSpace(os.Getenv(envKey)); override != "" {
		if parsed, ok := parseSemVerFromText(override); ok {
			return formatSemVer(parsed), "env:" + envKey, nil
		}
		return "", "", fmt.Errorf("invalid version override %s=%q", envKey, override)
	}

	if detected, ok := detectManagedAgentBinaryVersion(agentID); ok {
		return detected, "binary:" + agentID, nil
	}

	if parsed, ok := parseSemVerFromText(recommended); ok {
		return formatSemVer(parsed), "lock:recommended_version", nil
	}
	return "", "", fmt.Errorf("unable to resolve version for %s", agentID)
}

func detectManagedAgentBinaryVersion(agentID string) (string, bool) {
	binaryPath, err := exec.LookPath(agentID)
	if err != nil {
		return "", false
	}
	if version, ok := detectVersionWithCommand(binaryPath, "--version"); ok {
		return version, true
	}
	if version, ok := detectVersionWithCommand(binaryPath, "version"); ok {
		return version, true
	}
	return "", false
}

func detectVersionWithCommand(binaryPath string, arg string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, arg)
	out, err := cmd.CombinedOutput()
	if len(out) == 0 && err != nil {
		return "", false
	}
	if parsed, ok := parseSemVerFromText(string(out)); ok {
		return formatSemVer(parsed), true
	}
	return "", false
}

func parseSemVerFromText(raw string) (semVer, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return semVer{}, false
	}
	if exact := semVerExactPattern.FindStringSubmatch(trimmed); len(exact) == 4 {
		return parseSemVerParts(exact[1], exact[2], exact[3])
	}
	matches := semVerTextPattern.FindStringSubmatch(trimmed)
	if len(matches) != 4 {
		return semVer{}, false
	}
	return parseSemVerParts(matches[1], matches[2], matches[3])
}

func parseSemVerParts(major, minor, patch string) (semVer, bool) {
	maj, err := strconv.Atoi(major)
	if err != nil {
		return semVer{}, false
	}
	min, err := strconv.Atoi(minor)
	if err != nil {
		return semVer{}, false
	}
	pat, err := strconv.Atoi(patch)
	if err != nil {
		return semVer{}, false
	}
	return semVer{Major: maj, Minor: min, Patch: pat}, true
}

func formatSemVer(v semVer) string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func compareSemVer(a, b semVer) int {
	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}
	return 0
}

func semverSatisfiesRange(version, rangeExpr string) (bool, error) {
	v, ok := parseSemVerFromText(version)
	if !ok {
		return false, fmt.Errorf("invalid version %q", version)
	}
	raw := strings.TrimSpace(strings.ReplaceAll(rangeExpr, ",", " "))
	if raw == "" {
		return true, nil
	}
	for _, token := range strings.Fields(raw) {
		matches := rangeTokenPattern.FindStringSubmatch(strings.TrimSpace(token))
		if len(matches) != 3 {
			return false, fmt.Errorf("invalid range token %q", token)
		}
		op := matches[1]
		if op == "" {
			op = "="
		}
		required, ok := parseSemVerFromText(matches[2])
		if !ok {
			return false, fmt.Errorf("invalid range version %q", matches[2])
		}
		cmp := compareSemVer(v, required)
		if !evaluateSemVerConstraint(cmp, op) {
			return false, nil
		}
	}
	return true, nil
}

func evaluateSemVerConstraint(cmp int, op string) bool {
	switch op {
	case "=":
		return cmp == 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	default:
		return false
	}
}

func resetManagedCompatLockForTests() {
	managedCompatLockOnce = sync.Once{}
	managedCompatLock = upstreamCompatLock{}
}
