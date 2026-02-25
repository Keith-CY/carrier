package credentialstore

import (
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProviderCredentialServiceTrimsID(t *testing.T) {
	got := providerCredentialService("  openai  ")
	if got != "carrier.provider.openai" {
		t.Fatalf("providerCredentialService() = %q", got)
	}
}

func TestKeychainHelpersReturnBackendUnavailableWhenDisabled(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	if keychainAvailable() {
		t.Fatal("expected keychain to be unavailable when explicitly disabled")
	}

	if _, err := loadCredentialFromKeychain("carrier.provider.openai"); !errors.Is(err, errCredentialBackendUnavailable) {
		t.Fatalf("loadCredentialFromKeychain error = %v", err)
	}
	if err := saveCredentialToKeychain("carrier.provider.openai", "token"); !errors.Is(err, errCredentialBackendUnavailable) {
		t.Fatalf("saveCredentialToKeychain error = %v", err)
	}
}

func TestLoadProviderCredentialReturnsParseErrorFromFile(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)

	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o600); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}

	_, _, ok, err := LoadProviderCredential("openai")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if ok {
		t.Fatal("expected no credential on parse error")
	}
	if !strings.Contains(err.Error(), "parse credential file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadCredentialFromFileBranches(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)

	if err := os.WriteFile(storePath, []byte(`{"providers":null}`), 0o600); err != nil {
		t.Fatalf("write providers=null file: %v", err)
	}
	if _, err := loadCredentialFromFile("openai"); !errors.Is(err, errCredentialNotFound) {
		t.Fatalf("expected credential not found for nil providers, got %v", err)
	}

	if err := os.WriteFile(storePath, []byte(`{"providers":{"openai":"   "}}`), 0o600); err != nil {
		t.Fatalf("write empty token file: %v", err)
	}
	if _, err := loadCredentialFromFile("openai"); !errors.Is(err, errCredentialNotFound) {
		t.Fatalf("expected credential not found for blank value, got %v", err)
	}
}

func TestSaveCredentialToFileMergesExistingProviders(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)

	if err := os.WriteFile(storePath, []byte(`{"providers":{"anthropic":"a-token"}}`), 0o600); err != nil {
		t.Fatalf("seed credential file: %v", err)
	}
	if err := saveCredentialToFile("openai", "o-token"); err != nil {
		t.Fatalf("saveCredentialToFile: %v", err)
	}

	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	var payload struct {
		Providers map[string]string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse credential file: %v", err)
	}
	if payload.Providers["anthropic"] != "a-token" || payload.Providers["openai"] != "o-token" {
		t.Fatalf("unexpected provider payload: %#v", payload.Providers)
	}
}

func TestSaveProviderCredentialReturnsFileReadError(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	dir := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", dir) // directory path forces read error in file backend

	backend, err := SaveProviderCredential("openai", "token")
	if err == nil {
		t.Fatal("expected file backend error")
	}
	if backend != "" {
		t.Fatalf("expected empty backend on error, got %q", backend)
	}
	if !strings.Contains(err.Error(), "read credential file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCredentialStorePathUsesTrimmedEnvOverride(t *testing.T) {
	tmpPath := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", "  "+tmpPath+"  ")

	got, err := credentialStorePath()
	if err != nil {
		t.Fatalf("credentialStorePath: %v", err)
	}
	if got != tmpPath {
		t.Fatalf("path = %q, want %q", got, tmpPath)
	}
}

func TestResolveHomeDirBranches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific fallback assertions")
	}

	t.Run("uses HOME directly", func(t *testing.T) {
		t.Setenv("HOME", "/tmp/home-from-env")
		resetHomeResolvers(t)
		got, err := resolveHomeDir()
		if err != nil {
			t.Fatalf("resolveHomeDir: %v", err)
		}
		if got != "/tmp/home-from-env" {
			t.Fatalf("home = %q", got)
		}
	})

	t.Run("uses trimmed os.UserHomeDir", func(t *testing.T) {
		t.Setenv("HOME", "")
		resetHomeResolvers(t)
		userHomeDirFunc = func() (string, error) { return "  /tmp/user-home  ", nil }
		currentUserFunc = func() (*user.User, error) {
			t.Fatal("currentUserFunc should not be called when UserHomeDir succeeds")
			return nil, nil
		}

		got, err := resolveHomeDir()
		if err != nil {
			t.Fatalf("resolveHomeDir: %v", err)
		}
		if got != "/tmp/user-home" {
			t.Fatalf("home = %q", got)
		}
	})

	t.Run("falls back to /home/<USER>", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USER", "alice")
		resetHomeResolvers(t)
		userHomeDirFunc = func() (string, error) { return "", errors.New("home unavailable") }
		currentUserFunc = func() (*user.User, error) { return nil, errors.New("lookup failed") }

		got, err := resolveHomeDir()
		if err != nil {
			t.Fatalf("resolveHomeDir: %v", err)
		}
		if got != "/home/alice" {
			t.Fatalf("home = %q", got)
		}
	})

	t.Run("falls back to /root when USER is empty", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USER", "")
		resetHomeResolvers(t)
		userHomeDirFunc = func() (string, error) { return "", errors.New("home unavailable") }
		currentUserFunc = func() (*user.User, error) { return nil, errors.New("lookup failed") }

		got, err := resolveHomeDir()
		if err != nil {
			t.Fatalf("resolveHomeDir: %v", err)
		}
		if got != "/root" {
			t.Fatalf("home = %q", got)
		}
	})
}

func TestKeychainCommandBranchesWithFakeSecurity(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain command behavior only applies on macOS")
	}

	fakeDir := t.TempDir()
	scriptPath := filepath.Join(fakeDir, "security")
	script := `#!/bin/sh
case "$SECURITY_BEHAVIOR" in
  find_ok)
    printf "%s\n" "$SECURITY_VALUE"
    exit 0
    ;;
  find_not_found)
    printf "SecKeychainSearchCopyNext: The specified item could not be found in the keychain.\n" 1>&2
    exit 44
    ;;
  find_fail)
    printf "unexpected keychain failure\n" 1>&2
    exit 2
    ;;
  add_ok)
    exit 0
    ;;
  add_fail)
    printf "write denied\n" 1>&2
    exit 3
    ;;
esac
exit 5
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake security script: %v", err)
	}
	t.Setenv("PATH", fakeDir+":"+os.Getenv("PATH"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "")

	if !keychainAvailable() {
		t.Fatal("expected keychainAvailable() to detect fake security binary")
	}

	t.Run("load success", func(t *testing.T) {
		t.Setenv("SECURITY_BEHAVIOR", "find_ok")
		t.Setenv("SECURITY_VALUE", "  sk-test-123  ")
		value, err := loadCredentialFromKeychain("carrier.provider.openai")
		if err != nil {
			t.Fatalf("loadCredentialFromKeychain: %v", err)
		}
		if value != "sk-test-123" {
			t.Fatalf("value = %q, want sk-test-123", value)
		}
	})

	t.Run("load not found", func(t *testing.T) {
		t.Setenv("SECURITY_BEHAVIOR", "find_not_found")
		if _, err := loadCredentialFromKeychain("carrier.provider.openai"); !errors.Is(err, errCredentialNotFound) {
			t.Fatalf("expected errCredentialNotFound, got %v", err)
		}
	})

	t.Run("load command failure", func(t *testing.T) {
		t.Setenv("SECURITY_BEHAVIOR", "find_fail")
		if _, err := loadCredentialFromKeychain("carrier.provider.openai"); err == nil {
			t.Fatal("expected read keychain error")
		} else if !strings.Contains(err.Error(), "read keychain credential for carrier.provider.openai") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("save success", func(t *testing.T) {
		t.Setenv("SECURITY_BEHAVIOR", "add_ok")
		if err := saveCredentialToKeychain("carrier.provider.openai", "token-value"); err != nil {
			t.Fatalf("saveCredentialToKeychain: %v", err)
		}
	})

	t.Run("save command failure", func(t *testing.T) {
		t.Setenv("SECURITY_BEHAVIOR", "add_fail")
		if err := saveCredentialToKeychain("carrier.provider.openai", "token-value"); err == nil {
			t.Fatal("expected write keychain error")
		} else if !strings.Contains(err.Error(), "write keychain credential for carrier.provider.openai") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestProviderCredentialPrefersKeychainWhenAvailable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain command behavior only applies on macOS")
	}

	fakeDir := t.TempDir()
	scriptPath := filepath.Join(fakeDir, "security")
	script := `#!/bin/sh
if [ "$1" = "find-generic-password" ]; then
  printf "sk-keychain\n"
  exit 0
fi
if [ "$1" = "add-generic-password" ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake security script: %v", err)
	}
	t.Setenv("PATH", fakeDir+":"+os.Getenv("PATH"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(t.TempDir(), "credentials.json"))

	value, backend, ok, err := LoadProviderCredential("openai")
	if err != nil {
		t.Fatalf("LoadProviderCredential: %v", err)
	}
	if !ok || value != "sk-keychain" || backend != "macOS-keychain" {
		t.Fatalf("unexpected keychain load result: value=%q backend=%q ok=%v", value, backend, ok)
	}

	saveBackend, err := SaveProviderCredential("openai", "token-value")
	if err != nil {
		t.Fatalf("SaveProviderCredential: %v", err)
	}
	if saveBackend != "macOS-keychain" {
		t.Fatalf("save backend = %q, want macOS-keychain", saveBackend)
	}
}
