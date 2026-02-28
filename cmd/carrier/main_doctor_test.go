package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRunDoctorAllHealthy(t *testing.T) {
	writeDefaultProviderConfigForTest(t, "openai", "openai/gpt-5.2")

	origProbe := daemonHealthProbe
	origFetcher := doctorAgentStatusesFetcher
	t.Cleanup(func() {
		daemonHealthProbe = origProbe
		doctorAgentStatusesFetcher = origFetcher
	})
	daemonHealthProbe = func(string) bool { return true }
	doctorAgentStatusesFetcher = func() ([]doctorAgentStatus, error) {
		return []doctorAgentStatus{{ID: "openclaw", Runtime: "running", Health: "healthy"}}, nil
	}

	var out bytes.Buffer
	if err := runDoctor(&out, doctorCommandOptions{JSON: true}); err != nil {
		t.Fatalf("runDoctor error: %v", err)
	}

	var payload struct {
		Checks []doctorCheckResult `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode doctor json: %v", err)
	}
	assertDoctorCheckStatus(t, payload.Checks, "Daemon reachable", "ok")
	assertDoctorCheckStatus(t, payload.Checks, "Running agents healthy", "ok")
}

func TestRunDoctorDaemonDown(t *testing.T) {
	writeDefaultProviderConfigForTest(t, "openai", "openai/gpt-5.2")

	origProbe := daemonHealthProbe
	origFetcher := doctorAgentStatusesFetcher
	t.Cleanup(func() {
		daemonHealthProbe = origProbe
		doctorAgentStatusesFetcher = origFetcher
	})
	daemonHealthProbe = func(string) bool { return false }
	doctorAgentStatusesFetcher = func() ([]doctorAgentStatus, error) { return nil, nil }

	var out bytes.Buffer
	if err := runDoctor(&out, doctorCommandOptions{JSON: true}); err != nil {
		t.Fatalf("runDoctor error: %v", err)
	}

	var payload struct {
		Checks []doctorCheckResult `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode doctor json: %v", err)
	}
	assertDoctorCheckStatus(t, payload.Checks, "Daemon reachable", "fail")
}

func assertDoctorCheckStatus(t *testing.T, checks []doctorCheckResult, name, wantStatus string) {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			if check.Status != wantStatus {
				t.Fatalf("%s status = %q, want %q", name, check.Status, wantStatus)
			}
			return
		}
	}
	t.Fatalf("check %q missing", name)
}
