package gateway

import (
	"strings"
	"testing"
)

func TestOnboardStore_StartAndGet(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:123"

	if s.get(key) != nil {
		t.Error("expected nil before start")
	}

	sess := s.start(key)
	if sess == nil {
		t.Fatal("start returned nil")
	}
	if sess.Step != OnboardIdle {
		t.Errorf("expected idle, got %q", sess.Step)
	}
	if sess.EnvVars == nil {
		t.Error("EnvVars should be initialized")
	}
}

func TestOnboardStore_Update(t *testing.T) {
	s := NewOnboardStore()
	key := "discord:ch"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})
	sess := s.get(key)
	if sess.Step != OnboardAgentSelected {
		t.Errorf("expected agent_selected, got %q", sess.Step)
	}
	if sess.SelectedAgent != "openclaw" {
		t.Errorf("selectedAgent: %q", sess.SelectedAgent)
	}
}

func TestOnboardStore_Clear(t *testing.T) {
	s := NewOnboardStore()
	key := "feishu:chat"
	s.start(key)
	s.clear(key)
	if s.get(key) != nil {
		t.Error("expected nil after clear")
	}
}

func TestOnboardStore_HasActive(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:999"
	if s.hasActive(key) {
		t.Error("no session, should not be active")
	}
	s.start(key)
	if s.hasActive(key) {
		t.Error("idle step should not be active")
	}
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })
	if !s.hasActive(key) {
		t.Error("agent_selected step should be active")
	}
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardDone })
	if s.hasActive(key) {
		t.Error("done step should not be active")
	}
}

func TestOnboardCancel_NoSession(t *testing.T) {
	s := NewOnboardStore()
	resp := onboardCancel("req-1", "telegram:1", s)
	if resp.Result != "ok" {
		t.Errorf("cancel with no session should be ok: %+v", resp)
	}
	if !strings.Contains(resp.Message, "No active") {
		t.Errorf("message: %q", resp.Message)
	}
}

func TestOnboardCancel_ActiveSession(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })

	resp := onboardCancel("req-1", key, s)
	if resp.Result != "ok" {
		t.Errorf("cancel should be ok: %+v", resp)
	}
	if !strings.Contains(resp.Message, "cancelled") {
		t.Errorf("message: %q", resp.Message)
	}
	if s.get(key) != nil {
		t.Error("session should be cleared after cancel")
	}
}

func TestOnboardEnvInput_Done(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardEnvInput("req-1", key, "done", s)
	if resp.Result != "ok" {
		t.Errorf("done should be ok: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardEnvConfigured {
		t.Errorf("expected env_configured, got %q", sess.Step)
	}
}

func TestOnboardEnvInput_SetVar(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })

	resp := onboardEnvInput("req-1", key, "FOO=bar", s)
	if resp.Result != "ok" {
		t.Errorf("env set should be ok: %+v", resp)
	}
	sess := s.get(key)
	if sess.EnvVars["FOO"] != "bar" {
		t.Errorf("FOO should be 'bar', got %q", sess.EnvVars["FOO"])
	}
}

func TestOnboardEnvInput_InvalidFormat(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })

	resp := onboardEnvInput("req-1", key, "notakvpair", s)
	if resp.Result != "error" {
		t.Errorf("invalid env should be error: %+v", resp)
	}
}
