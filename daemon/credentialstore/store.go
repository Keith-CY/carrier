package credentialstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	errCredentialNotFound           = errors.New("credential not found")
	errCredentialBackendUnavailable = errors.New("credential backend unavailable")
)

var (
	userHomeDirFunc = os.UserHomeDir
	currentUserFunc = user.Current
)

func LoadProviderCredential(providerID string) (string, string, bool, error) {
	service := providerCredentialService(providerID)
	if value, err := loadCredentialFromKeychain(service); err == nil {
		return value, "macOS-keychain", true, nil
	} else if !errors.Is(err, errCredentialNotFound) && !errors.Is(err, errCredentialBackendUnavailable) {
		return "", "", false, err
	}

	value, err := loadCredentialFromFile(providerID)
	if err == nil {
		return value, "local-file", true, nil
	}
	if errors.Is(err, errCredentialNotFound) {
		return "", "", false, nil
	}
	return "", "", false, err
}

func SaveProviderCredential(providerID, value string) (string, error) {
	service := providerCredentialService(providerID)
	if err := saveCredentialToKeychain(service, value); err == nil {
		return "macOS-keychain", nil
	} else if !errors.Is(err, errCredentialBackendUnavailable) {
		return "", err
	}
	if err := saveCredentialToFile(providerID, value); err != nil {
		return "", err
	}
	return "local-file", nil
}

func providerCredentialService(providerID string) string {
	return "carrier.provider." + strings.TrimSpace(providerID)
}

func keychainAvailable() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CARRIER_DISABLE_KEYCHAIN")), "1") {
		return false
	}
	_, err := exec.LookPath("security")
	return err == nil
}

func loadCredentialFromKeychain(service string) (string, error) {
	if !keychainAvailable() {
		return "", errCredentialBackendUnavailable
	}
	cmd := exec.Command("security", "find-generic-password", "-a", "carrier", "-s", service, "-w")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(stderr.String()))
		if strings.Contains(msg, "could not be found") || strings.Contains(msg, "item not found") {
			return "", errCredentialNotFound
		}
		return "", fmt.Errorf("read keychain credential for %s", service)
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", errCredentialNotFound
	}
	return value, nil
}

func saveCredentialToKeychain(service, value string) error {
	if !keychainAvailable() {
		return errCredentialBackendUnavailable
	}
	cmd := exec.Command("security", "add-generic-password", "-U", "-a", "carrier", "-s", service, "-w", value)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("write keychain credential for %s", service)
	}
	return nil
}

func loadCredentialFromFile(providerID string) (string, error) {
	path, err := credentialStorePath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errCredentialNotFound
		}
		return "", fmt.Errorf("read credential file: %w", err)
	}
	var payload struct {
		Providers map[string]string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("parse credential file: %w", err)
	}
	if payload.Providers == nil {
		return "", errCredentialNotFound
	}
	value := strings.TrimSpace(payload.Providers[providerID])
	if value == "" {
		return "", errCredentialNotFound
	}
	return value, nil
}

func saveCredentialToFile(providerID, value string) error {
	path, err := credentialStorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}

	payload := struct {
		Providers map[string]string `json:"providers"`
	}{
		Providers: make(map[string]string),
	}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &payload)
		if payload.Providers == nil {
			payload.Providers = make(map[string]string)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read credential file: %w", err)
	}

	payload.Providers[providerID] = value
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credential file: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	return nil
}

func credentialStorePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_CREDENTIAL_STORE")); path != "" {
		return path, nil
	}
	home, err := resolveHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".carrier", "credentials.json"), nil
}

func resolveHomeDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home, nil
	}

	var userHomeErr error
	if home, err := userHomeDirFunc(); err == nil {
		trimmed := strings.TrimSpace(home)
		if trimmed != "" {
			return trimmed, nil
		}
	} else {
		userHomeErr = err
	}

	if current, err := currentUserFunc(); err == nil && current != nil {
		trimmed := strings.TrimSpace(current.HomeDir)
		if trimmed != "" {
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

	if userHomeErr != nil {
		return "", userHomeErr
	}
	return "", errors.New("home directory unavailable")
}
