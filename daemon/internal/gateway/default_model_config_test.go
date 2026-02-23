package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeGatewayDefaultProviderConfig(t *testing.T, providerID, modelID, envVar string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)
	modelName := providerID + "-default"
	payload := map[string]interface{}{
		"config_version": 2,
		"default_model":  modelName,
		"model_list": []map[string]string{
			{
				"model_name":  modelName,
				"model":       modelID,
				"provider_id": providerID,
				"env_var":     envVar,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal config payload: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config payload: %v", err)
	}
	return path
}
