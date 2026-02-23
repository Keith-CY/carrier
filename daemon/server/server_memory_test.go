package server

import (
	"errors"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func resetMemoryRootResolvers(t *testing.T) {
	t.Helper()
	origUserConfigDir := userConfigDirFunc
	origUserHomeDir := userHomeDirFunc
	origCurrentUser := currentUserFunc
	t.Cleanup(func() {
		userConfigDirFunc = origUserConfigDir
		userHomeDirFunc = origUserHomeDir
		currentUserFunc = origCurrentUser
	})
}

func TestDefaultMemoryRootUsesConfiguredOverride(t *testing.T) {
	resetMemoryRootResolvers(t)
	custom := filepath.Join(t.TempDir(), "memory-root")
	t.Setenv("CARRIER_MEMORY_ROOT", custom)

	root, err := defaultMemoryRoot()
	if err != nil {
		t.Fatalf("defaultMemoryRoot error: %v", err)
	}
	if root != custom {
		t.Fatalf("root = %q, want %q", root, custom)
	}
}

func TestDefaultMemoryRootFallsBackToHomeConfigDir(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("CARRIER_MEMORY_ROOT", "")
	t.Setenv("HOME", "")

	userConfigDirFunc = func() (string, error) {
		return "", errors.New("neither $XDG_CONFIG_HOME nor $HOME are defined")
	}
	home := filepath.Join(t.TempDir(), "home")
	currentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: home}, nil
	}
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}

	root, err := defaultMemoryRoot()
	if err != nil {
		t.Fatalf("defaultMemoryRoot error: %v", err)
	}
	want := filepath.Join(home, ".config", "carrier", "memory")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveDaemonHomeDirFallsBackToRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific fallback")
	}

	resetMemoryRootResolvers(t)
	t.Setenv("HOME", "")
	t.Setenv("USER", "root")

	userHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}
	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("lookup user failed")
	}

	home, err := resolveDaemonHomeDir()
	if err != nil {
		t.Fatalf("resolveDaemonHomeDir error: %v", err)
	}
	if home != "/root" {
		t.Fatalf("home = %q, want /root", home)
	}
}

func TestDefaultLifecycleStatePathUsesConfiguredOverride(t *testing.T) {
	resetMemoryRootResolvers(t)
	custom := filepath.Join(t.TempDir(), "state", "lifecycle.json")
	t.Setenv("CARRIER_LIFECYCLE_STATE_FILE", custom)

	path, err := defaultLifecycleStatePath()
	if err != nil {
		t.Fatalf("defaultLifecycleStatePath error: %v", err)
	}
	if path != custom {
		t.Fatalf("path = %q, want %q", path, custom)
	}
}

func TestDefaultLifecycleStatePathFallsBackToHomeConfigDir(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("CARRIER_LIFECYCLE_STATE_FILE", "")
	t.Setenv("HOME", "")

	userConfigDirFunc = func() (string, error) {
		return "", errors.New("neither $XDG_CONFIG_HOME nor $HOME are defined")
	}
	home := filepath.Join(t.TempDir(), "home")
	currentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: home}, nil
	}
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}

	path, err := defaultLifecycleStatePath()
	if err != nil {
		t.Fatalf("defaultLifecycleStatePath error: %v", err)
	}
	want := filepath.Join(home, ".config", "carrier", "lifecycle-state.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
