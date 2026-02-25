package gateway

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestListLocalSSHConfigHosts_MissingConfigReturnsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	hosts, err := listLocalSSHConfigHosts()
	if err != nil {
		t.Fatalf("listLocalSSHConfigHosts error: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected no hosts, got %v", hosts)
	}
}

func TestListLocalSSHConfigHosts_ParsesAliasesAndIncludes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(sshDir, "conf.d"), 0o700); err != nil {
		t.Fatalf("mkdir ssh config dir: %v", err)
	}

	rootConfig := `# root config
Host prod-eu-1 *.prod !prod-blocked
  HostName 10.0.0.1

Host jump-box qa-1
  User ubuntu

Include conf.d/*.conf
`
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(rootConfig), 0o600); err != nil {
		t.Fatalf("write root config: %v", err)
	}

	extraConfig := `Host stage-a
Host qa-* !qa-blocked
Host "quoted-host"
`
	if err := os.WriteFile(filepath.Join(sshDir, "conf.d", "extra.conf"), []byte(extraConfig), 0o600); err != nil {
		t.Fatalf("write include config: %v", err)
	}

	hosts, err := listLocalSSHConfigHosts()
	if err != nil {
		t.Fatalf("listLocalSSHConfigHosts error: %v", err)
	}

	want := []string{"jump-box", "prod-eu-1", "qa-1", "quoted-host", "stage-a"}
	if !slices.Equal(hosts, want) {
		t.Fatalf("unexpected hosts list: got=%v want=%v", hosts, want)
	}
}

func TestRemoteSSHConfigHostsEndpoint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir ssh dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host prod-a\nHost *.wild\n"), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

	mux := buildRemoteFeatureMux(t)
	rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/ssh-config-hosts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ssh config hosts status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	items, _ := payload["hosts"].([]interface{})
	if len(items) != 1 || items[0] != "prod-a" {
		t.Fatalf("unexpected hosts payload=%v", payload)
	}
}

func TestRemoteSSHConfigHostsEndpoint_WhenRemoteControlDisabled(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}, nil)

	rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/ssh-config-hosts", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}
