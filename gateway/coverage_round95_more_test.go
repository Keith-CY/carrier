package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type round95Stringer struct {
	value string
}

func (s round95Stringer) String() string { return s.value }

func TestParseOpenClawInstancesAndCanonicalHelpersRound95(t *testing.T) {
	parsed := parseOpenClawInstances("host-1", "log prefix [{\"id\":\"main\",\"runtimeState\":\"running\",\"health\":\"healthy\"},{\"agentId\":\"zeroclaw\",\"status\":\"stopped\"}] tail")
	if len(parsed) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(parsed))
	}
	if parsed[0].ID != "host-1:main" || parsed[0].RuntimeState != "running" || parsed[0].Health != "healthy" {
		t.Fatalf("unexpected first instance: %+v", parsed[0])
	}
	if parsed[1].ID != "host-1:zeroclaw" || parsed[1].RuntimeState != "stopped" || parsed[1].Health != "unknown" {
		t.Fatalf("unexpected second instance: %+v", parsed[1])
	}
	if got := parseOpenClawInstances("host-1", "not-json"); got != nil {
		t.Fatalf("expected nil parse result for invalid payload, got %+v", got)
	}
	fallback := parseOpenClawInstances("host-1", "[{}]")
	if len(fallback) != 1 || fallback[0].ID != "host-1:main" || fallback[0].RuntimeState != "unknown" || fallback[0].Health != "unknown" {
		t.Fatalf("unexpected fallback parse result: %+v", fallback)
	}

	if got := openClawRawFromCanonical(nil); got != "" {
		t.Fatalf("expected empty openclaw raw from nil payload, got %q", got)
	}
	if got := openClawRawFromCanonical(map[string]interface{}{"raw_json5": " {x:1} "}); got != "{x:1}" {
		t.Fatalf("unexpected openclaw raw: %q", got)
	}
	if got := zeroClawRawFromCanonical(nil); got != "" {
		t.Fatalf("expected empty zeroclaw raw from nil payload, got %q", got)
	}
	if got := zeroClawRawFromCanonical(map[string]interface{}{"raw_toml": " api_key = \"k\" "}); got != "api_key = \"k\"" {
		t.Fatalf("unexpected zeroclaw raw: %q", got)
	}
}

func TestExtractJSONObjectOrArrayAndAnyToStringRound95(t *testing.T) {
	if got := extractJSONObjectOrArray(`{"a":1}`); got != `{"a":1}` {
		t.Fatalf("unexpected object extraction: %q", got)
	}
	if got := extractJSONObjectOrArray("prefix [1,2,3] suffix"); got != `[1,2,3]` {
		t.Fatalf("unexpected array extraction: %q", got)
	}
	if got := extractJSONObjectOrArray("prefix {\"a\":1} trailing"); got != `{"a":1}` {
		t.Fatalf("unexpected trimmed object extraction: %q", got)
	}
	if got := extractJSONObjectOrArray("garbage"); got != "" {
		t.Fatalf("expected empty extraction for garbage, got %q", got)
	}

	if got := anyToString("x"); got != "x" {
		t.Fatalf("unexpected anyToString string result: %q", got)
	}
	if got := anyToString(round95Stringer{value: "s"}); got != "s" {
		t.Fatalf("unexpected anyToString stringer result: %q", got)
	}
	if got := anyToString(float64(12)); got != "12" {
		t.Fatalf("unexpected anyToString float-int result: %q", got)
	}
	if got := anyToString(float64(12.5)); got != "12.5" {
		t.Fatalf("unexpected anyToString float result: %q", got)
	}
	if got := anyToString(7); got != "7" {
		t.Fatalf("unexpected anyToString int result: %q", got)
	}
	if got := anyToString(int64(8)); got != "8" {
		t.Fatalf("unexpected anyToString int64 result: %q", got)
	}
	if got := anyToString(json.Number("9")); got != "9" {
		t.Fatalf("unexpected anyToString json.Number result: %q", got)
	}
	if got := anyToString(struct{}{}); got != "" {
		t.Fatalf("expected empty anyToString default branch, got %q", got)
	}
}

func TestWriteSSEEventAndChunkStringRound95(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeSSEEvent(rec, map[string]interface{}{"type": "delta", "content": "hello"}); err != nil {
		t.Fatalf("writeSSEEvent success error: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, `"type":"delta"`) {
		t.Fatalf("unexpected SSE body: %q", body)
	}
	if err := writeSSEEvent(rec, map[string]interface{}{"bad": func() {}}); err == nil {
		t.Fatal("expected writeSSEEvent marshal error for function payload")
	}

	if got := chunkString("abc", 0); !reflect.DeepEqual(got, []string{"abc"}) {
		t.Fatalf("unexpected chunkString size<=0 result: %v", got)
	}
	if got := chunkString("abc", 10); !reflect.DeepEqual(got, []string{"abc"}) {
		t.Fatalf("unexpected chunkString short input result: %v", got)
	}
	if got := chunkString("你好abcd", 2); !reflect.DeepEqual(got, []string{"你好", "ab", "cd"}) {
		t.Fatalf("unexpected chunkString multi-chunk result: %v", got)
	}
}

func TestRemoteStoreAndOnboardConfigHelpersRound95(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "remote-control.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)

	path, err := remoteControlStorePath()
	if err != nil {
		t.Fatalf("remoteControlStorePath error: %v", err)
	}
	if filepath.Clean(path) != filepath.Clean(storePath) {
		t.Fatalf("unexpected remote control store path: %q", path)
	}

	state := &remoteControlState{
		Hosts:    []RemoteHost{},
		Profiles: []ProviderProfile{{ID: "p1", Provider: "openai", Model: "gpt-5", Enabled: true}, {ID: "p2", Provider: "openai", Model: "gpt-4", Enabled: true}},
		Bindings: []ProviderBinding{{ID: "b1", ProfileID: "p1", TargetType: "host", TargetID: "h1", SyncMode: providerBindingSyncModeAlwaysPush}},
	}
	if err := saveRemoteControlState(storePath, state); err != nil {
		t.Fatalf("saveRemoteControlState error: %v", err)
	}

	profiles, err := listProviderProfiles()
	if err != nil || len(profiles) != 2 {
		t.Fatalf("unexpected listProviderProfiles result profiles=%v err=%v", profiles, err)
	}
	if deleted, err := deleteProviderProfile("missing"); err != nil || deleted {
		t.Fatalf("expected missing profile delete to be false,nil; deleted=%v err=%v", deleted, err)
	}
	if deleted, err := deleteProviderProfile("p1"); err != nil || !deleted {
		t.Fatalf("expected existing profile delete to be true,nil; deleted=%v err=%v", deleted, err)
	}
	profiles, err = listProviderProfiles()
	if err != nil || len(profiles) != 1 || profiles[0].ID != "p2" {
		t.Fatalf("unexpected profiles after delete: %v err=%v", profiles, err)
	}
	bindings, err := listProviderBindings()
	if err != nil || len(bindings) != 0 {
		t.Fatalf("unexpected bindings after profile delete: %v err=%v", bindings, err)
	}

	t.Setenv("CARRIER_CONFIG", "/tmp/cfg-primary.json")
	t.Setenv("CARRIER_ONBOARD_CONFIG", "/tmp/cfg-secondary.json")
	if got, err := onboardConfigPath(); err != nil || got != "/tmp/cfg-primary.json" {
		t.Fatalf("expected CARRIER_CONFIG to win, got=%q err=%v", got, err)
	}
	t.Setenv("CARRIER_CONFIG", "")
	if got, err := onboardConfigPath(); err != nil || got != "/tmp/cfg-secondary.json" {
		t.Fatalf("expected CARRIER_ONBOARD_CONFIG fallback, got=%q err=%v", got, err)
	}

	if err := applyOnboardConfigEnvironment(nil); err != nil {
		t.Fatalf("applyOnboardConfigEnvironment nil cfg error: %v", err)
	}
	cfg := &onboardConfigFile{
		Channels: []onboardConfigChannel{
			{ID: "telegram", BotToken: "tg-token", WebhookSecret: "tg-secret", TransportMode: "polling"},
			{ID: "discord", BotToken: "dc-token", WebhookSecret: "dc-public"},
		},
		ModelList: []onboardConfigModel{{ProviderID: "openai", EnvVar: "", CredentialRef: ""}},
	}
	if err := applyOnboardConfigEnvironment(cfg); err != nil {
		t.Fatalf("applyOnboardConfigEnvironment error: %v", err)
	}
	if got := strings.TrimSpace(strings.ToLower(anyToString(strings.TrimSpace(strings.ToLower("localhost"))))); got != "localhost" {
		t.Fatalf("unexpected sanity string path: %q", got)
	}
	if token := strings.TrimSpace(strings.ToLower(anyToString(strings.TrimSpace(strings.ToLower(""))))); token != "" {
		t.Fatalf("unexpected empty token normalization: %q", token)
	}
	if got := isLocalhostOrIP(""); !got {
		t.Fatalf("expected empty host treated as local")
	}
	if got := isLocalhostOrIP("localhost"); !got {
		t.Fatalf("expected localhost treated as local")
	}
	if got := isLocalhostOrIP("127.0.0.1"); !got {
		t.Fatalf("expected IPv4 localhost treated as local")
	}
	if got := isLocalhostOrIP("::1"); !got {
		t.Fatalf("expected IPv6 localhost treated as local")
	}
	if got := isLocalhostOrIP("example.com"); got {
		t.Fatalf("expected non-local host not treated as local")
	}
}
