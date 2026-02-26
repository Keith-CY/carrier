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

func TestSplitSSHConfigDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		line  string
		key   string
		value string
		ok    bool
	}{
		{name: "whitespace separator", line: "Host prod-a", key: "Host", value: "prod-a", ok: true},
		{name: "tab separator", line: "Host\tprod-a", key: "Host", value: "prod-a", ok: true},
		{name: "equals separator", line: "Host=prod-a", key: "Host", value: "prod-a", ok: true},
		{name: "space then equals separator", line: "Host = prod-a", key: "Host", value: "prod-a", ok: true},
		{name: "space then tight equals separator", line: "Host =prod-a", key: "Host", value: "prod-a", ok: true},
		{name: "tight then space equals separator", line: "Host= prod-a", key: "Host", value: "prod-a", ok: true},
		{name: "value may contain equals", line: "ProxyCommand ssh -W %h:%p jump=host", key: "ProxyCommand", value: "ssh -W %h:%p jump=host", ok: true},
		{name: "missing separator", line: "Host", ok: false},
		{name: "missing value", line: "Host =", ok: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			key, value, ok := splitSSHConfigDirective(tc.line)
			if ok != tc.ok {
				t.Fatalf("ok mismatch: got=%v want=%v", ok, tc.ok)
			}
			if key != tc.key {
				t.Fatalf("key mismatch: got=%q want=%q", key, tc.key)
			}
			if value != tc.value {
				t.Fatalf("value mismatch: got=%q want=%q", value, tc.value)
			}
		})
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
