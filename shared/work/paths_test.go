package work

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCarrierPathsResolveDefaultRootsUsesHomeDirectory(t *testing.T) {
	t.Setenv("CARRIER_ROOT", "")
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	userHomeDirFunc = os.UserHomeDir
	t.Cleanup(func() { userHomeDirFunc = os.UserHomeDir })

	roots, err := ResolveRoots()
	if err != nil {
		t.Fatalf("ResolveRoots error: %v", err)
	}

	if roots.Root != filepath.Join(home, ".carrier") {
		t.Fatalf("root=%q", roots.Root)
	}
	if roots.App != filepath.Join(home, ".carrier", "app") {
		t.Fatalf("app=%q", roots.App)
	}
	if roots.Projects != filepath.Join(home, ".carrier", "projects") {
		t.Fatalf("projects=%q", roots.Projects)
	}
	if roots.Works != filepath.Join(home, ".carrier", "works") {
		t.Fatalf("works=%q", roots.Works)
	}
}

func TestCarrierPathsResolveRootOverrides(t *testing.T) {
	t.Setenv("CARRIER_ROOT", "/tmp/carrier-root")
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	roots, err := ResolveRoots()
	if err != nil {
		t.Fatalf("ResolveRoots error: %v", err)
	}

	if roots.Root != "/tmp/carrier-root" {
		t.Fatalf("root=%q", roots.Root)
	}
	if roots.App != "/tmp/carrier-root/app" {
		t.Fatalf("app=%q", roots.App)
	}
	if roots.Projects != "/tmp/carrier-root/projects" {
		t.Fatalf("projects=%q", roots.Projects)
	}
	if roots.Works != "/tmp/carrier-root/works" {
		t.Fatalf("works=%q", roots.Works)
	}
}

func TestCarrierPathsResolveExplicitOverrides(t *testing.T) {
	t.Setenv("CARRIER_ROOT", "/tmp/carrier-root")
	t.Setenv("CARRIER_APP_ROOT", "/tmp/app-root")
	t.Setenv("CARRIER_PROJECTS_ROOT", "/tmp/projects-root")
	t.Setenv("CARRIER_WORKS_ROOT", "/tmp/works-root")

	roots, err := ResolveRoots()
	if err != nil {
		t.Fatalf("ResolveRoots error: %v", err)
	}

	if roots.App != "/tmp/app-root" {
		t.Fatalf("app=%q", roots.App)
	}
	if roots.Projects != "/tmp/projects-root" {
		t.Fatalf("projects=%q", roots.Projects)
	}
	if roots.Works != "/tmp/works-root" {
		t.Fatalf("works=%q", roots.Works)
	}
}

func TestCarrierPathsResolveRootsHomeError(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("CARRIER_ROOT", "")
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")
	userHomeDirFunc = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() { userHomeDirFunc = os.UserHomeDir })

	_, err := ResolveRoots()
	if err == nil || err.Error() != "resolve home dir: home unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}
}
