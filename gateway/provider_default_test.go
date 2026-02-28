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

func TestBuildCarrierDefaultProviderInfo_OpenAICompatibleWithoutSavedCredentialNotReusable(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", t.TempDir()+"/credentials.json")
	writeGatewayDefaultProviderConfig(t, "openai-compatible", "openai-compatible/demo", "OPENAI_COMPATIBLE_API_KEY")

	info := buildCarrierDefaultProviderInfo()
	if info["configured"] != true || info["available"] != true || info["reusable"] != false {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info["has_saved_credential"] != false {
		t.Fatalf("unexpected credential metadata: %+v", info)
	}

	p, ok := info["provider"].(*LLMProvider)
	if !ok || p == nil || p.ID != "openai-compatible" {
		t.Fatalf("unexpected provider payload: %#v", info["provider"])
	}
}
