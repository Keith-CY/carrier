package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const remoteKeyUploadMaxBytes = 256 * 1024

var remoteKeyRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{7,127}$`)

type remoteUploadedKey struct {
	ID          string `json:"id"`
	KeyRef      string `json:"keyRef"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	SizeBytes   int    `json:"sizeBytes"`
	CreatedAt   string `json:"createdAt"`
}

func remoteKeyDirPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_REMOTE_KEY_DIR")); custom != "" {
		return filepath.Clean(custom), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for remote key dir: %w", err)
	}
	return filepath.Join(home, ".carrier", "keys"), nil
}

func resolveRemoteKeyPath(keyRef string) (string, error) {
	ref := strings.TrimSpace(keyRef)
	if !remoteKeyRefPattern.MatchString(ref) {
		return "", fmt.Errorf("keyRef format is invalid")
	}
	dir, err := remoteKeyDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ref+".pem"), nil
}

func saveUploadedRemoteKey(filename string, raw []byte) (*remoteUploadedKey, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("PEM file is empty")
	}
	if len(raw) > remoteKeyUploadMaxBytes {
		return nil, fmt.Errorf("PEM file exceeds %d bytes", remoteKeyUploadMaxBytes)
	}
	if err := validatePEMPrivateKey(raw); err != nil {
		return nil, err
	}

	dir, err := remoteKeyDirPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create remote key dir: %w", err)
	}

	ref := strings.ReplaceAll(uuid.NewString(), "-", "")
	path := filepath.Join(dir, ref+".pem")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, fmt.Errorf("write PEM file: %w", err)
	}

	name := strings.TrimSpace(filepath.Base(filename))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "uploaded.pem"
	}
	now := nowTimestamp()
	return &remoteUploadedKey{
		ID:          ref,
		KeyRef:      ref,
		Name:        name,
		Fingerprint: pemFingerprint(raw),
		SizeBytes:   len(raw),
		CreatedAt:   now,
	}, nil
}

func validatePEMPrivateKey(raw []byte) error {
	rest := raw
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		if strings.Contains(strings.ToUpper(strings.TrimSpace(block.Type)), "PRIVATE KEY") {
			return nil
		}
		rest = next
	}
	return fmt.Errorf("uploaded file must contain a PEM private key block")
}

func pemFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
