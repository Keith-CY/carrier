package baseagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type runtimeServiceFake struct {
	agents    []AgentState
	statuses  map[string]AgentState
	logLines  map[string][]string
	upgrades  map[string]UpgradeResult
	diagnoses map[string]string
	errs      map[string]error
}

func (f *runtimeServiceFake) ListAgents() []AgentState {
	return slices.Clone(f.agents)
}

func (f *runtimeServiceFake) Install(_ context.Context, agentID string) error {
	return f.err("install", agentID)
}

func (f *runtimeServiceFake) Uninstall(_ context.Context, agentID string) error {
	return f.err("uninstall", agentID)
}

func (f *runtimeServiceFake) Start(_ context.Context, agentID string) error {
	return f.err("start", agentID)
}

func (f *runtimeServiceFake) Stop(_ context.Context, agentID string) error {
	return f.err("stop", agentID)
}

func (f *runtimeServiceFake) Status(agentID string) (AgentState, error) {
	if err := f.err("status", agentID); err != nil {
		return AgentState{}, err
	}
	if s, ok := f.statuses[agentID]; ok {
		return s, nil
	}
	return AgentState{ID: agentID}, nil
}

func (f *runtimeServiceFake) Logs(agentID string, _ int) ([]string, error) {
	if err := f.err("logs", agentID); err != nil {
		return nil, err
	}
	return slices.Clone(f.logLines[agentID]), nil
}

func (f *runtimeServiceFake) Upgrade(_ context.Context, agentID string) (UpgradeResult, error) {
	if err := f.err("upgrade", agentID); err != nil {
		return UpgradeResult{}, err
	}
	if u, ok := f.upgrades[agentID]; ok {
		return u, nil
	}
	return UpgradeResult{AgentID: agentID, FromVersion: "unknown", ToVersion: "unknown"}, nil
}

func (f *runtimeServiceFake) Diagnose(agentID string) (string, error) {
	if err := f.err("diagnose", agentID); err != nil {
		return "", err
	}
	if d, ok := f.diagnoses[agentID]; ok {
		return d, nil
	}
	return "diag://" + agentID, nil
}

func (f *runtimeServiceFake) err(action, agentID string) error {
	if f == nil || f.errs == nil {
		return nil
	}
	if err, ok := f.errs[action+":"+agentID]; ok {
		return err
	}
	if err, ok := f.errs[action+":*"]; ok {
		return err
	}
	return nil
}

var errMemoryMissing = errors.New("memory entry not found")

type runtimeMemoryFake struct {
	entries map[string]MemoryEntry

	getErrByID    map[string]error
	createErrByID map[string]error
	createErrPref string
	createErr     error

	setAttachmentsErr      error
	setAttachmentsErrQueue []error
	prepareErrQueue        []error
	exportRef              string
	exportErr              error
	archiveErr             error

	prepareCalls   int
	createCalls    []string
	archiveCalls   []string
	attachCalls    [][]string
	attachCallSeen int
}

func newRuntimeMemoryFake() *runtimeMemoryFake {
	return &runtimeMemoryFake{
		entries:       map[string]MemoryEntry{},
		getErrByID:    map[string]error{},
		createErrByID: map[string]error{},
	}
}

func (m *runtimeMemoryFake) Get(id string) error {
	if err, ok := m.getErrByID[id]; ok {
		return err
	}
	if _, ok := m.entries[id]; ok {
		return nil
	}
	return errMemoryMissing
}

func (m *runtimeMemoryFake) Create(id, _ string, _ string, _ MemoryType, _ string) error {
	if m.createErr != nil && strings.HasPrefix(id, m.createErrPref) {
		return m.createErr
	}
	if err, ok := m.createErrByID[id]; ok {
		return err
	}
	m.entries[id] = MemoryEntry{ID: id}
	m.createCalls = append(m.createCalls, id)
	return nil
}

func (m *runtimeMemoryFake) List() []MemoryEntry {
	out := make([]MemoryEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		out = append(out, entry)
	}
	return out
}

func (m *runtimeMemoryFake) SetAttachmentsFromLinks(_ string, memoryIDs []string) error {
	clone := make([]string, len(memoryIDs))
	copy(clone, memoryIDs)
	m.attachCalls = append(m.attachCalls, clone)
	if m.attachCallSeen < len(m.setAttachmentsErrQueue) {
		err := m.setAttachmentsErrQueue[m.attachCallSeen]
		m.attachCallSeen++
		return err
	}
	m.attachCallSeen++
	return m.setAttachmentsErr
}

func (m *runtimeMemoryFake) PrepareAgentMemory(_ string) error {
	call := m.prepareCalls
	m.prepareCalls++
	if call < len(m.prepareErrQueue) && m.prepareErrQueue[call] != nil {
		return m.prepareErrQueue[call]
	}
	return nil
}

func (m *runtimeMemoryFake) ExportMemory(memoryID string, _ ExportOptions) (string, error) {
	if m.exportErr != nil {
		return "", m.exportErr
	}
	if m.exportRef == "" {
		m.exportRef = "backup://" + memoryID
	}
	return m.exportRef, m.exportErr
}

func (m *runtimeMemoryFake) Archive(memoryID string) error {
	if m.archiveErr != nil {
		return m.archiveErr
	}
	entry := m.entries[memoryID]
	entry.State = MemoryStateArchived
	m.entries[memoryID] = entry
	m.archiveCalls = append(m.archiveCalls, memoryID)
	return nil
}

func TestWantsListAgents(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "list agents", want: true},
		{input: "show agents", want: true},
		{input: "agents", want: true},
		{input: "install openclaw", want: false},
		{input: "add openclaw", want: false},
	}
	for _, tc := range tests {
		if got := wantsListAgents(tc.input); got != tc.want {
			t.Fatalf("wantsListAgents(%q)=%v want=%v", tc.input, got, tc.want)
		}
	}
}

func TestRenderAgentList(t *testing.T) {
	if got := renderAgentList(nil); got != "No agents are registered." {
		t.Fatalf("unexpected empty agent list output: %q", got)
	}

	got := renderAgentList([]AgentState{
		{ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok"},
		{ID: "zeroclaw", Install: "not_installed", Runtime: "stopped", Health: "unknown"},
	})
	if !strings.Contains(got, "Found 2 agents:") {
		t.Fatalf("missing header: %q", got)
	}
	if !strings.Contains(got, "- openclaw: install=installed runtime=running health=ok") {
		t.Fatalf("missing first entry: %q", got)
	}
}

func TestBaseAgentHelpers(t *testing.T) {
	help := baseAgentHelpText()
	if !strings.Contains(help, "list agents") || !strings.Contains(help, "diagnose <agent>") {
		t.Fatalf("unexpected help text: %q", help)
	}

	resp := withMemoryNote(ChatResponse{Message: "payload"}, "memory note", true, "backup://1")
	if !strings.HasPrefix(resp.Message, "memory note\npayload") {
		t.Fatalf("unexpected note prefix: %q", resp.Message)
	}
	if !resp.SelfHealed || resp.BackupRef != "backup://1" {
		t.Fatalf("unexpected heal metadata: %+v", resp)
	}

	noNote := withMemoryNote(ChatResponse{Message: "payload"}, "   ", false, "")
	if noNote.Message != "payload" {
		t.Fatalf("unexpected no-note message: %q", noNote.Message)
	}

	if got := explainLLMUnavailable(nil); got != "LLM chat is currently unavailable." {
		t.Fatalf("unexpected nil error message: %q", got)
	}
	longErr := errors.New(strings.Repeat("x", 320))
	if got := explainLLMUnavailable(longErr); !strings.HasPrefix(got, "LLM chat is currently unavailable (") || !strings.HasSuffix(got, "...).") {
		t.Fatalf("unexpected long error format: %q", got)
	}
}

func TestExecuteAgentActionAllBranches(t *testing.T) {
	svc := &runtimeServiceFake{
		statuses: map[string]AgentState{
			"openclaw": {ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok", RestartCount: 2},
		},
		logLines: map[string][]string{
			"openclaw": {"l1", "l2"},
			"idle":     {},
		},
		upgrades: map[string]UpgradeResult{
			"openclaw": {AgentID: "openclaw", FromVersion: "1.0.0", ToVersion: "1.1.0"},
		},
		diagnoses: map[string]string{
			"openclaw": "diag://openclaw",
		},
	}
	rt := NewRuntime(svc, nil)

	tests := []struct {
		action  string
		agentID string
		needle  string
	}{
		{action: "uninstall", agentID: "openclaw", needle: "Uninstalled openclaw."},
		{action: "start", agentID: "openclaw", needle: "Started openclaw."},
		{action: "stop", agentID: "openclaw", needle: "Stopped openclaw."},
		{action: "status", agentID: "openclaw", needle: "restarts=2"},
		{action: "logs", agentID: "openclaw", needle: "l1\nl2"},
		{action: "logs", agentID: "idle", needle: "No logs for idle."},
		{action: "upgrade", agentID: "openclaw", needle: "Upgraded openclaw from 1.0.0 to 1.1.0."},
		{action: "diagnose", agentID: "openclaw", needle: "diag://openclaw"},
	}
	for _, tc := range tests {
		resp, err := rt.executeAgentAction(context.Background(), tc.action, tc.agentID)
		if err != nil {
			t.Fatalf("%s failed unexpectedly: %v", tc.action, err)
		}
		if !strings.Contains(resp.Message, tc.needle) {
			t.Fatalf("%s response missing %q: %q", tc.action, tc.needle, resp.Message)
		}
	}

	svc.errs = map[string]error{"start:openclaw": errors.New("start failed")}
	if _, err := rt.executeAgentAction(context.Background(), "start", "openclaw"); err == nil {
		t.Fatal("expected start error")
	}

	svc.errs = map[string]error{
		"uninstall:openclaw": errors.New("uninstall failed"),
		"stop:openclaw":      errors.New("stop failed"),
		"status:openclaw":    errors.New("status failed"),
		"logs:openclaw":      errors.New("logs failed"),
		"upgrade:openclaw":   errors.New("upgrade failed"),
		"diagnose:openclaw":  errors.New("diagnose failed"),
	}
	for _, action := range []string{"uninstall", "stop", "status", "logs", "upgrade", "diagnose"} {
		if _, err := rt.executeAgentAction(context.Background(), action, "openclaw"); err == nil {
			t.Fatalf("expected %s error", action)
		}
	}
}

func TestPickActiveMemoryID(t *testing.T) {
	mem := newRuntimeMemoryFake()
	rt := NewRuntime(nil, mem)
	if got := rt.pickActiveMemoryID(); got != baseAgentActiveMemoryV1ID {
		t.Fatalf("expected default active memory id, got %q", got)
	}

	mem.entries["carrier.base.active.1"] = MemoryEntry{ID: "carrier.base.active.1"}
	mem.entries["carrier.base.active.2"] = MemoryEntry{ID: "carrier.base.active.2"}
	mem.entries["carrier.base.active.3"] = MemoryEntry{ID: "carrier.base.active.3", State: MemoryStateArchived}
	mem.entries["other.memory"] = MemoryEntry{ID: "other.memory"}
	if got := rt.pickActiveMemoryID(); got != "carrier.base.active.2" {
		t.Fatalf("expected latest non-archived active memory, got %q", got)
	}
}

func TestEnsureMemoryEntry(t *testing.T) {
	mem := newRuntimeMemoryFake()
	mem.entries["existing"] = MemoryEntry{ID: "existing"}
	rt := NewRuntime(nil, mem)

	if err := rt.ensureMemoryEntry("existing", "name", "v1", MemoryTypePublic, ""); err != nil {
		t.Fatalf("ensure existing memory should succeed: %v", err)
	}
	if len(mem.createCalls) != 0 {
		t.Fatalf("unexpected create call for existing memory: %+v", mem.createCalls)
	}

	if err := rt.ensureMemoryEntry("new-id", "name", "v1", MemoryTypePublic, ""); err != nil {
		t.Fatalf("ensure new memory should create entry: %v", err)
	}
	if len(mem.createCalls) != 1 || mem.createCalls[0] != "new-id" {
		t.Fatalf("unexpected create calls: %+v", mem.createCalls)
	}
}

func TestEnsureMemoryReady(t *testing.T) {
	rtNoMem := NewRuntime(nil, nil)
	note, healed, backupRef, err := rtNoMem.ensureMemoryReady()
	if err != nil || note != "" || healed || backupRef != "" {
		t.Fatalf("unexpected nil-memory result: note=%q healed=%v backup=%q err=%v", note, healed, backupRef, err)
	}

	mem := newRuntimeMemoryFake()
	rt := NewRuntime(nil, mem)
	note, healed, backupRef, err = rt.ensureMemoryReady()
	if err != nil {
		t.Fatalf("ensureMemoryReady init error: %v", err)
	}
	if note != "" || healed || backupRef != "" {
		t.Fatalf("unexpected init result: note=%q healed=%v backup=%q", note, healed, backupRef)
	}
	if !rt.initialized {
		t.Fatal("expected runtime initialized")
	}
	if _, ok := mem.entries[baseAgentPublicMemoryID]; !ok {
		t.Fatal("expected public memory entry")
	}
	if _, ok := mem.entries[rt.activeID]; !ok {
		t.Fatal("expected active memory entry")
	}
	if len(mem.attachCalls) == 0 {
		t.Fatal("expected attachment call")
	}

	beforeCreateCalls := len(mem.createCalls)
	note, healed, backupRef, err = rt.ensureMemoryReady()
	if err != nil {
		t.Fatalf("second ensureMemoryReady call error: %v", err)
	}
	if note != "" || healed || backupRef != "" {
		t.Fatalf("unexpected second-call result: note=%q healed=%v backup=%q", note, healed, backupRef)
	}
	if len(mem.createCalls) != beforeCreateCalls {
		t.Fatalf("expected no new create calls on second ensure, got %+v", mem.createCalls)
	}

	memRepair := newRuntimeMemoryFake()
	memRepair.entries[baseAgentPublicMemoryID] = MemoryEntry{ID: baseAgentPublicMemoryID}
	memRepair.entries[baseAgentActiveMemoryV1ID] = MemoryEntry{ID: baseAgentActiveMemoryV1ID}
	memRepair.prepareErrQueue = []error{errors.New("prepare failed"), nil}
	memRepair.exportRef = "backup://active-v1"
	rtRepair := NewRuntime(nil, memRepair)

	note, healed, backupRef, err = rtRepair.ensureMemoryReady()
	if err != nil {
		t.Fatalf("ensureMemoryReady self-heal error: %v", err)
	}
	if !healed || backupRef != "backup://active-v1" {
		t.Fatalf("unexpected heal result: healed=%v backup=%q", healed, backupRef)
	}
	if !strings.Contains(note, "self-heal completed") || !strings.Contains(note, "backup://active-v1") {
		t.Fatalf("unexpected heal note: %q", note)
	}
	if len(memRepair.archiveCalls) == 0 || memRepair.archiveCalls[0] != baseAgentActiveMemoryV1ID {
		t.Fatalf("expected archive call for previous active memory: %+v", memRepair.archiveCalls)
	}
	if !strings.HasPrefix(rtRepair.activeID, baseAgentActiveMemoryPrefix) {
		t.Fatalf("unexpected new active memory id: %q", rtRepair.activeID)
	}
}

func TestEnsureMemoryReadyErrorPaths(t *testing.T) {
	memCreateErr := newRuntimeMemoryFake()
	memCreateErr.createErrByID[baseAgentPublicMemoryID] = errors.New("create failed")
	rtCreateErr := NewRuntime(nil, memCreateErr)
	if _, _, _, err := rtCreateErr.ensureMemoryReady(); err == nil {
		t.Fatal("expected create failure")
	}

	memAttachErr := newRuntimeMemoryFake()
	memAttachErr.setAttachmentsErr = errors.New("attach failed")
	rtAttachErr := NewRuntime(nil, memAttachErr)
	if _, _, _, err := rtAttachErr.ensureMemoryReady(); err == nil {
		t.Fatal("expected attachment failure")
	}

	memActiveCreateErr := newRuntimeMemoryFake()
	memActiveCreateErr.createErrByID[baseAgentActiveMemoryV1ID] = errors.New("active create failed")
	rtActiveCreateErr := NewRuntime(nil, memActiveCreateErr)
	if _, _, _, err := rtActiveCreateErr.ensureMemoryReady(); err == nil {
		t.Fatal("expected active-memory create failure")
	}

	memPrepareErr := newRuntimeMemoryFake()
	memPrepareErr.entries[baseAgentPublicMemoryID] = MemoryEntry{ID: baseAgentPublicMemoryID}
	memPrepareErr.entries[baseAgentActiveMemoryV1ID] = MemoryEntry{ID: baseAgentActiveMemoryV1ID}
	memPrepareErr.prepareErrQueue = []error{errors.New("first prepare failed"), errors.New("second prepare failed")}
	rtPrepareErr := NewRuntime(nil, memPrepareErr)
	if _, _, _, err := rtPrepareErr.ensureMemoryReady(); err == nil {
		t.Fatal("expected self-heal second prepare failure")
	}

	memSecondAttachErr := newRuntimeMemoryFake()
	memSecondAttachErr.entries[baseAgentPublicMemoryID] = MemoryEntry{ID: baseAgentPublicMemoryID}
	memSecondAttachErr.entries[baseAgentActiveMemoryV1ID] = MemoryEntry{ID: baseAgentActiveMemoryV1ID}
	memSecondAttachErr.prepareErrQueue = []error{errors.New("first prepare failed"), nil}
	memSecondAttachErr.setAttachmentsErrQueue = []error{nil, errors.New("second attach failed")}
	rtSecondAttachErr := NewRuntime(nil, memSecondAttachErr)
	if _, _, _, err := rtSecondAttachErr.ensureMemoryReady(); err == nil {
		t.Fatal("expected second attachment failure during self-heal")
	}

	memNewActiveCreateErr := newRuntimeMemoryFake()
	memNewActiveCreateErr.entries[baseAgentPublicMemoryID] = MemoryEntry{ID: baseAgentPublicMemoryID}
	memNewActiveCreateErr.entries[baseAgentActiveMemoryV1ID] = MemoryEntry{ID: baseAgentActiveMemoryV1ID}
	memNewActiveCreateErr.prepareErrQueue = []error{errors.New("first prepare failed"), nil}
	memNewActiveCreateErr.createErrPref = baseAgentActiveMemoryPrefix
	memNewActiveCreateErr.createErr = errors.New("new active create failed")
	rtNewActiveCreateErr := NewRuntime(nil, memNewActiveCreateErr)
	if _, _, _, err := rtNewActiveCreateErr.ensureMemoryReady(); err == nil {
		t.Fatal("expected new active memory create failure")
	}

	memNoBackup := newRuntimeMemoryFake()
	memNoBackup.entries[baseAgentPublicMemoryID] = MemoryEntry{ID: baseAgentPublicMemoryID}
	memNoBackup.entries[baseAgentActiveMemoryV1ID] = MemoryEntry{ID: baseAgentActiveMemoryV1ID}
	memNoBackup.prepareErrQueue = []error{errors.New("first prepare failed"), nil}
	memNoBackup.exportErr = errors.New("export failed")
	rtNoBackup := NewRuntime(nil, memNoBackup)
	note, healed, backup, err := rtNoBackup.ensureMemoryReady()
	if err != nil {
		t.Fatalf("unexpected no-backup self-heal error: %v", err)
	}
	if !healed || backup != "" {
		t.Fatalf("unexpected no-backup result: healed=%v backup=%q", healed, backup)
	}
	if strings.Contains(note, "Backup:") {
		t.Fatalf("unexpected backup note when export failed: %q", note)
	}
}

func TestRuntimeChatBranches(t *testing.T) {
	svc := &runtimeServiceFake{
		agents: []AgentState{
			{ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok"},
		},
		statuses: map[string]AgentState{
			"openclaw": {ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok", RestartCount: 1},
		},
	}
	rt := NewRuntime(svc, nil)

	resp, err := rt.Chat(context.Background(), ChatRequest{Message: "   "})
	if err != nil || !strings.Contains(resp.Message, "Base agent manages local agents") {
		t.Fatalf("empty-message branch failed: resp=%+v err=%v", resp, err)
	}

	resp, err = rt.Chat(context.Background(), ChatRequest{Message: "list agents"})
	if err != nil || resp.Action != "list_agents" {
		t.Fatalf("list-agents branch failed: resp=%+v err=%v", resp, err)
	}

	resp, err = rt.Chat(context.Background(), ChatRequest{Message: "start openclaw"})
	if err != nil || resp.Action != "start" {
		t.Fatalf("action branch failed: resp=%+v err=%v", resp, err)
	}

	svc.errs = map[string]error{"stop:openclaw": errors.New("cannot stop")}
	resp, err = rt.Chat(context.Background(), ChatRequest{Message: "stop openclaw"})
	if err != nil || resp.Action != "stop" || !strings.Contains(resp.Message, "failed") {
		t.Fatalf("action-error branch failed: resp=%+v err=%v", resp, err)
	}

	svc.errs = nil
	resp, err = rt.Chat(context.Background(), ChatRequest{Message: "help"})
	if err != nil || resp.Action != "help" {
		t.Fatalf("help branch failed: resp=%+v err=%v", resp, err)
	}

	resp, err = rt.Chat(context.Background(), ChatRequest{Message: "can you check openclaw"})
	if err != nil || resp.Action != "status" || !strings.Contains(resp.Message, "openclaw") {
		t.Fatalf("known-agent status fallback failed: resp=%+v err=%v", resp, err)
	}
}

func TestRuntimeChatLLMPaths(t *testing.T) {
	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode llm payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"llm says hi"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	rt := NewRuntime(&runtimeServiceFake{}, nil)
	resp, err := rt.Chat(context.Background(), ChatRequest{Message: "free-form question"})
	if err != nil || resp.Action != "chat" || resp.Message != "llm says hi" {
		t.Fatalf("llm success branch failed: resp=%+v err=%v", resp, err)
	}

	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))
	resp, err = rt.Chat(context.Background(), ChatRequest{Message: "another free-form question"})
	if err != nil || resp.Action != "help" || !strings.Contains(resp.Message, "LLM chat is currently unavailable") {
		t.Fatalf("llm unavailable branch failed: resp=%+v err=%v", resp, err)
	}
}

func TestRuntimeChatMemoryFailureAddsSafeModeNote(t *testing.T) {
	mem := newRuntimeMemoryFake()
	mem.createErrByID[baseAgentPublicMemoryID] = errors.New("create failed")
	svc := &runtimeServiceFake{
		agents: []AgentState{{ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok"}},
	}
	rt := NewRuntime(svc, mem)
	resp, err := rt.Chat(context.Background(), ChatRequest{Message: "list agents"})
	if err != nil {
		t.Fatalf("chat with memory failure returned error: %v", err)
	}
	if !strings.Contains(resp.Message, "safe mode") {
		t.Fatalf("expected safe mode note in response: %q", resp.Message)
	}
}

func TestResolveConfiguredBaseAgentProviderID(t *testing.T) {
	writeDefaultModelConfig(t, "openai", "openai/gpt-5.2", "OPENAI_API_KEY")
	if got := resolveConfiguredBaseAgentProviderID(); got != "openai" {
		t.Fatalf("resolveConfiguredBaseAgentProviderID() = %q, want %q", got, "openai")
	}

	writeDefaultModelConfig(t, "unknown-provider", "unknown/model", "UNKNOWN_API_KEY")
	if got := resolveConfiguredBaseAgentProviderID(); got != "" {
		t.Fatalf("resolveConfiguredBaseAgentProviderID() = %q, want empty for unsupported provider", got)
	}

	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))
	if got := resolveConfiguredBaseAgentProviderID(); got != "" {
		t.Fatalf("resolveConfiguredBaseAgentProviderID() with missing config = %q, want empty", got)
	}
}

func TestRuntimeChannelAndProviderWrappers(t *testing.T) {
	var nilRuntime *Runtime
	if err := nilRuntime.RegisterExternalChannel("telegram", func(context.Context, OutboundEnvelope) error { return nil }); err == nil {
		t.Fatal("expected nil runtime channel registration error")
	}
	if err := nilRuntime.StartChannels(context.Background()); err == nil {
		t.Fatal("expected nil runtime start channels error")
	}
	if err := nilRuntime.StopChannels(context.Background()); err == nil {
		t.Fatal("expected nil runtime stop channels error")
	}
	if err := nilRuntime.RegisterProvider(providerFake{name: "x", out: "ok"}); err == nil {
		t.Fatal("expected nil runtime register provider error")
	}
	if err := nilRuntime.SetActiveProvider("x"); err == nil {
		t.Fatal("expected nil runtime set active provider error")
	}

	rt := NewRuntime(nil, nil)
	sent := ""
	if err := rt.RegisterExternalChannel("telegram", func(_ context.Context, msg OutboundEnvelope) error {
		sent = msg.Content
		return nil
	}); err != nil {
		t.Fatalf("RegisterExternalChannel: %v", err)
	}
	if err := rt.StartChannels(context.Background()); err != nil {
		t.Fatalf("StartChannels: %v", err)
	}
	ch, ok := rt.channels.GetChannel("telegram")
	if !ok {
		t.Fatal("expected registered telegram channel")
	}
	if err := ch.Send(context.Background(), OutboundEnvelope{Channel: "telegram", ChatID: "c1", Content: "ping"}); err != nil {
		t.Fatalf("channel send failed: %v", err)
	}
	if sent != "ping" {
		t.Fatalf("sent payload = %q, want %q", sent, "ping")
	}
	if err := rt.StopChannels(context.Background()); err != nil {
		t.Fatalf("StopChannels: %v", err)
	}

	if err := rt.RegisterProvider(providerFake{name: "custom-provider", out: "custom reply"}); err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}
	if err := rt.SetActiveProvider("custom-provider"); err != nil {
		t.Fatalf("SetActiveProvider: %v", err)
	}
	reply, err := rt.providers.Reply(context.Background(), ProviderRequest{UserMessage: "hello"})
	if err != nil {
		t.Fatalf("provider reply failed: %v", err)
	}
	if reply != "custom reply" {
		t.Fatalf("provider reply = %q, want %q", reply, "custom reply")
	}
}

func TestMustRegisterProviderPanicsOnNilManager(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic for nil provider manager")
		}
	}()
	mustRegisterProvider(nil, providerFake{name: "p", out: "ok"})
}
