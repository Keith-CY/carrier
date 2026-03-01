package baseagent

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestBoundarySpecValidationErrorBranchesAndHelpers(t *testing.T) {
	valid := fallbackBoundarySpec()

	cases := []struct {
		name string
		mut  func(*BoundarySpec)
		want string
	}{
		{name: "schema", mut: func(s *BoundarySpec) { s.SchemaVersion = "v0" }, want: "schema_version"},
		{name: "assistant role", mut: func(s *BoundarySpec) { s.AssistantRole = "  " }, want: "assistant_role"},
		{name: "in scope", mut: func(s *BoundarySpec) { s.InScope = nil }, want: "in_scope"},
		{name: "out of scope", mut: func(s *BoundarySpec) { s.OutOfScope = nil }, want: "out_of_scope"},
		{name: "boundary sources", mut: func(s *BoundarySpec) { s.BoundarySources = nil }, want: "boundary_sources"},
		{name: "design principles", mut: func(s *BoundarySpec) { s.DesignPrinciples = nil }, want: "design_principles"},
		{name: "chat install", mut: func(s *BoundarySpec) { s.CommandPolicies.ChatInstall = "bad" }, want: "chat_install"},
		{name: "chat onboard", mut: func(s *BoundarySpec) { s.CommandPolicies.ChatOnboard = "bad" }, want: "chat_onboard"},
		{name: "workflow empty", mut: func(s *BoundarySpec) { s.WorkflowPolicies = nil }, want: "workflow_policies"},
		{
			name: "workflow empty key",
			mut: func(s *BoundarySpec) {
				s.WorkflowPolicies = map[string]WorkflowPolicy{
					"": {Enabled: true, MaxAttempts: 1},
				}
			},
			want: "empty key",
		},
		{
			name: "workflow attempts",
			mut: func(s *BoundarySpec) {
				s.WorkflowPolicies = map[string]WorkflowPolicy{
					"wf": {Enabled: true, MaxAttempts: 0},
				}
			},
			want: "max_attempts",
		},
		{name: "repair rounds", mut: func(s *BoundarySpec) { s.RepairPolicy.MaxAutoRepairRounds = 0 }, want: "max_auto_repair_rounds"},
		{name: "repair paths", mut: func(s *BoundarySpec) { s.RepairPolicy.HighRiskPathPrefixes = nil }, want: "high_risk_path_prefixes"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spec := valid
			tc.mut(&spec)
			err := ValidateBoundarySpec(spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected validation error containing %q, got %v", tc.want, err)
			}
		})
	}

	if _, err := ParseBoundarySpec([]byte(`{`)); err == nil || !strings.Contains(err.Error(), "parse boundary spec") {
		t.Fatalf("expected parse error, got %v", err)
	}

	if got := (BoundarySpec{}).RepairRoundBudget(); got != defaultInstallAutoRepairRoundBudget {
		t.Fatalf("unexpected default repair round budget: %d", got)
	}
	if got := prefixLines(nil); len(got) != 1 || got[0] != "- (none)" {
		t.Fatalf("unexpected prefix lines for nil: %v", got)
	}
	if got := prefixLines([]string{"", "  "}); len(got) != 1 || got[0] != "- (none)" {
		t.Fatalf("unexpected prefix lines for blanks: %v", got)
	}
}

func TestActiveBoundarySpecFallbackPath(t *testing.T) {
	origRaw := embeddedBoundarySpecRaw
	origOnce := activeBoundarySpecOnce
	origSpec := activeBoundarySpec
	origErr := activeBoundarySpecErr
	defer func() {
		embeddedBoundarySpecRaw = origRaw
		activeBoundarySpecOnce = origOnce
		activeBoundarySpec = origSpec
		activeBoundarySpecErr = origErr
	}()

	embeddedBoundarySpecRaw = []byte(`{"schema_version":"bad"}`)
	activeBoundarySpecOnce = sync.Once{}
	activeBoundarySpec = BoundarySpec{}
	activeBoundarySpecErr = nil

	spec := ActiveBoundarySpec()
	if strings.TrimSpace(spec.SchemaVersion) != boundarySpecSchemaV1 {
		t.Fatalf("expected fallback boundary spec, got schema %q", spec.SchemaVersion)
	}
}

func TestMustRegisterToolAndProviderPanicBranches(t *testing.T) {
	t.Run("mustRegisterTool nil registry", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for nil registry")
			}
		}()
		mustRegisterTool(nil, ToolSpec{Name: "x", Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false }, Run: func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil }})
	})

	t.Run("mustRegisterTool invalid tool", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for invalid tool registration")
			}
		}()
		mustRegisterTool(NewToolRegistry(), ToolSpec{Name: "bad"})
	})

	t.Run("mustRegisterProvider nil manager", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for nil provider manager")
			}
		}()
		mustRegisterProvider(nil, providerFake{name: "x", out: "x"})
	})

	t.Run("mustRegisterProvider invalid provider", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for invalid provider registration")
			}
		}()
		mustRegisterProvider(NewProviderManager(nil), providerFake{name: " ", out: "x"})
	})
}

func TestToolRegistryAndChannelManagerValidationBranches(t *testing.T) {
	var nilReg *ToolRegistry
	if err := nilReg.RegisterTool(ToolSpec{}); err == nil {
		t.Fatal("expected nil registry RegisterTool error")
	}
	if _, err := nilReg.ExecuteTool(context.Background(), "x", nil); err == nil {
		t.Fatal("expected nil registry ExecuteTool error")
	}
	if summary := nilReg.RenderToolSummary(); summary != "No tools are registered." {
		t.Fatalf("unexpected nil registry summary: %q", summary)
	}

	reg := NewToolRegistry()
	if err := reg.RegisterTool(ToolSpec{Name: "x"}); err == nil {
		t.Fatal("expected missing matcher error")
	}
	if err := reg.RegisterTool(ToolSpec{Name: "x", Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false }}); err == nil {
		t.Fatal("expected missing runner error")
	}
	if _, err := reg.ExecuteTool(context.Background(), "missing", nil); err == nil {
		t.Fatal("expected missing tool error")
	}

	manager := NewChannelManager(nil)
	if manager.bus == nil {
		t.Fatal("expected NewChannelManager(nil) to initialize default bus")
	}
	if err := manager.RegisterChannel("   ", NewTelegramChannel(func(context.Context, OutboundEnvelope) error { return nil })); err == nil {
		t.Fatal("expected empty channel name error")
	}
	if err := manager.RegisterChannel("telegram", nil); err == nil {
		t.Fatal("expected nil channel error")
	}
}

func TestSessionManagerNilAndListLimitBranches(t *testing.T) {
	var nilSM *SessionManager
	nilSM.AddMessage("k", "user", "x")
	if got := nilSM.History("k"); got != nil {
		t.Fatalf("expected nil history from nil session manager, got %v", got)
	}
	if got := nilSM.Summary("k"); got != "" {
		t.Fatalf("expected empty summary from nil session manager, got %q", got)
	}
	nilSM.SetSummary("k", "v")
	if got := nilSM.ListStats(1); got != nil {
		t.Fatalf("expected nil list stats from nil session manager, got %v", got)
	}

	sm := NewSessionManager(2)
	sm.AddMessage("a", "user", "1")
	sm.AddMessage("b", "user", "1")
	stats := sm.ListStats(0)
	if len(stats) != 2 {
		t.Fatalf("expected unbounded ListStats with limit=0, got %d", len(stats))
	}
}

func TestMatchAgentActionInvocationBranches(t *testing.T) {
	call, ok := matchAgentActionInvocation("/status openclaw")
	if !ok {
		t.Fatal("expected slash command to match")
	}
	if call.Args["action"] != "status" || call.Args["agent_id"] != "openclaw" {
		t.Fatalf("unexpected slash match args: %+v", call.Args)
	}

	call, ok = matchAgentActionInvocation("please diagnose openclaw asap")
	if !ok {
		t.Fatal("expected embedded command to match")
	}
	if call.Args["action"] != "diagnose" || call.Args["agent_id"] != "openclaw" {
		t.Fatalf("unexpected embedded match args: %+v", call.Args)
	}

	if _, ok := matchAgentActionInvocation("hello world"); ok {
		t.Fatal("expected unrelated text not to match")
	}
}

func TestRenderToolSummaryDescriptionBranches(t *testing.T) {
	reg := NewToolRegistry()
	_ = reg.RegisterTool(ToolSpec{
		Name:        "plain",
		Description: "",
		Match:       func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	_ = reg.RegisterTool(ToolSpec{
		Name:        "described",
		Description: "with desc",
		Match:       func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})

	summary := reg.RenderToolSummary()
	if !strings.Contains(summary, "- plain") || !strings.Contains(summary, "- described: with desc") {
		t.Fatalf("unexpected tool summary: %q", summary)
	}
}

func TestWantsListAgentsBranches(t *testing.T) {
	positives := []string{
		"/agents status",
		"/list agents",
		"show agents",
		"what agents are running",
	}
	for _, in := range positives {
		if !wantsListAgents(in) {
			t.Fatalf("expected wantsListAgents true for %q", in)
		}
	}

	negatives := []string{
		"hello world",
		"status openclaw",
		"install agents",
		"add agents",
	}
	for _, in := range negatives {
		if wantsListAgents(in) {
			t.Fatalf("expected wantsListAgents false for %q", in)
		}
	}
}
