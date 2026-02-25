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

func TestDefaultMemoryRootUsesUserConfigDirWhenAvailable(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("CARRIER_MEMORY_ROOT", "")
	t.Setenv("HOME", "")

	configDir := filepath.Join(t.TempDir(), "cfg")
	userConfigDirFunc = func() (string, error) {
		return configDir, nil
	}

	root, err := defaultMemoryRoot()
	if err != nil {
		t.Fatalf("defaultMemoryRoot error: %v", err)
	}
	want := filepath.Join(configDir, "carrier", "memory")
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

func TestResolveDaemonHomeDirFallsBackToRootWhenUserEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific fallback")
	}

	resetMemoryRootResolvers(t)
	t.Setenv("HOME", "")
	t.Setenv("USER", "")

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

func TestResolveDaemonHomeDirUsesUserNameFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific fallback")
	}

	resetMemoryRootResolvers(t)
	t.Setenv("HOME", "")
	t.Setenv("USER", "alice")

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
	if home != "/home/alice" {
		t.Fatalf("home = %q, want /home/alice", home)
	}
}

func TestResolveDaemonHomeDirUsesHOMEEnv(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("HOME", "/tmp/carrier-home")

	userHomeDirFunc = func() (string, error) {
		return "", errors.New("should not be called when HOME is set")
	}
	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("should not be called when HOME is set")
	}

	home, err := resolveDaemonHomeDir()
	if err != nil {
		t.Fatalf("resolveDaemonHomeDir error: %v", err)
	}
	if home != "/tmp/carrier-home" {
		t.Fatalf("home = %q, want %q", home, "/tmp/carrier-home")
	}
}

func TestResolveDaemonHomeDirUsesUserHomeDir(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("HOME", "")
	t.Setenv("USER", "")

	userHomeDirFunc = func() (string, error) {
		return "/tmp/from-user-home", nil
	}
	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("should not be called when userHomeDir succeeds")
	}

	home, err := resolveDaemonHomeDir()
	if err != nil {
		t.Fatalf("resolveDaemonHomeDir error: %v", err)
	}
	if home != "/tmp/from-user-home" {
		t.Fatalf("home = %q, want %q", home, "/tmp/from-user-home")
	}
}

func TestResolveDaemonHomeDirUsesCurrentUserFallback(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("HOME", "")
	t.Setenv("USER", "")

	userHomeDirFunc = func() (string, error) {
		return "", errors.New("userHomeDir failed")
	}
	currentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: "/tmp/from-current-user"}, nil
	}

	home, err := resolveDaemonHomeDir()
	if err != nil {
		t.Fatalf("resolveDaemonHomeDir error: %v", err)
	}
	if home != "/tmp/from-current-user" {
		t.Fatalf("home = %q, want %q", home, "/tmp/from-current-user")
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

func TestDefaultLifecycleStatePathUsesUserConfigDirWhenAvailable(t *testing.T) {
	resetMemoryRootResolvers(t)
	t.Setenv("CARRIER_LIFECYCLE_STATE_FILE", "")
	t.Setenv("HOME", "")

	configDir := filepath.Join(t.TempDir(), "cfg")
	userConfigDirFunc = func() (string, error) {
		return configDir, nil
	}

	path, err := defaultLifecycleStatePath()
	if err != nil {
		t.Fatalf("defaultLifecycleStatePath error: %v", err)
	}
	want := filepath.Join(configDir, "carrier", "lifecycle-state.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
