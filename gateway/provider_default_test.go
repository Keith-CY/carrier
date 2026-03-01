package gateway

import "testing"

func TestBuildCarrierDefaultProviderInfo_NoConfiguredDefault(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", "/path/does/not/exist/config.v2.json")

	info := buildCarrierDefaultProviderInfo()
	if info["configured"] != false || info["available"] != false || info["reusable"] != false {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestBuildCarrierDefaultProviderInfo_ConfiguredButUnknownProvider(t *testing.T) {
	writeGatewayDefaultProviderConfig(t, "unknown-provider", "unknown/model", "UNKNOWN_API_KEY")

	info := buildCarrierDefaultProviderInfo()
	if info["configured"] != true {
		t.Fatalf("expected configured=true, got %+v", info)
	}
	if info["available"] != false {
		t.Fatalf("expected available=false, got %+v", info)
	}
	if info["reason"] != "default provider is not in gateway catalog" {
		t.Fatalf("unexpected reason: %+v", info)
	}
}

func TestBuildCarrierDefaultProviderInfo_AuthModeNoneIsReusable(t *testing.T) {
	writeGatewayDefaultProviderConfig(t, "ollama", "ollama/llama3", "")

	info := buildCarrierDefaultProviderInfo()
	if info["configured"] != true || info["available"] != true || info["reusable"] != true {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info["has_saved_credential"] != true || info["credential_backend"] != "none" {
		t.Fatalf("unexpected credential metadata: %+v", info)
	}

	p, ok := info["provider"].(*LLMProvider)
	if !ok || p == nil || p.ID != "ollama" {
		t.Fatalf("unexpected provider payload: %#v", info["provider"])
	}
}
