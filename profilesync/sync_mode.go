package profilesync

import (
	"fmt"
	"strings"
)

type SyncMode string

const (
	SyncModeAlwaysPush       SyncMode = "always_push"
	SyncModePullValidatePush SyncMode = "pull_validate_push"
	SyncModeManual           SyncMode = "manual"
)

func NormalizeSyncMode(raw string) SyncMode {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return SyncModeAlwaysPush
	}
	return SyncMode(mode)
}

func ValidateSyncMode(mode SyncMode) error {
	switch NormalizeSyncMode(string(mode)) {
	case SyncModeAlwaysPush, SyncModePullValidatePush, SyncModeManual:
		return nil
	default:
		return fmt.Errorf("sync mode must be one of always_push, pull_validate_push, manual")
	}
}
