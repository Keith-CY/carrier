package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"carrier/shared/integration"
)

func syncIntegrationUsageProofsByOrchestratorExecution(orchestrator OrchestratorExecution) error {
	exec, found, err := loadIntegrationExecutionByOrchestratorID(orchestrator.ID)
	if err != nil || !found {
		return err
	}
	return syncIntegrationUsageProofsForExecution(exec, orchestrator)
}

func syncIntegrationUsageProofsForExecution(exec integration.Execution, orchestrator OrchestratorExecution) error {
	governance := hydrateProviderGovernanceUsage(orchestrator)
	if len(governance.ProviderResolutions) == 0 {
		return nil
	}
	for _, resolution := range governance.ProviderResolutions {
		if resolution.EstimatedTotalTokens <= 0 && resolution.EstimatedCostUSD <= 0 {
			continue
		}
		proof := buildIntegrationUsageProof(exec.ID, resolution)
		if _, err := upsertIntegrationUsageProof(proof); err != nil {
			return err
		}
	}
	return nil
}

func buildIntegrationUsageProof(executionID string, resolution ProviderGovernanceResolution) integration.UsageProof {
	amountCents := int64(math.Round(max(resolution.EstimatedCostUSD, 0) * 100))
	stableKey := strings.Join([]string{
		strings.TrimSpace(executionID),
		strings.TrimSpace(resolution.HostID),
		strings.ToLower(strings.TrimSpace(resolution.AgentID)),
		strings.TrimSpace(resolution.Provider),
		strings.TrimSpace(resolution.Model),
	}, "|")
	payload := map[string]interface{}{
		"hostId":                strings.TrimSpace(resolution.HostID),
		"agentId":               strings.ToLower(strings.TrimSpace(resolution.AgentID)),
		"provider":              strings.TrimSpace(resolution.Provider),
		"model":                 strings.TrimSpace(resolution.Model),
		"estimatedInputTokens":  resolution.EstimatedInputTokens,
		"estimatedOutputTokens": resolution.EstimatedOutputTokens,
		"estimatedTotalTokens":  resolution.EstimatedTotalTokens,
		"estimatedCostUsd":      resolution.EstimatedCostUSD,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	providerPart := sanitizeIntegrationUsagePart(resolution.Provider)
	modelPart := sanitizeIntegrationUsagePart(resolution.Model)
	return integration.UsageProof{
		ID:          "proof_" + shortIntegrationUsageHash(stableKey),
		ExecutionID: executionID,
		ProofRef:    fmt.Sprintf("usage://executions/%s/providers/%s/%s", executionID, providerPart, modelPart),
		MeterRef:    fmt.Sprintf("provider:%s:model:%s", providerPart, modelPart),
		UsageKind:   "provider_cost_estimate",
		AmountCents: amountCents,
		Digest:      hex.EncodeToString(sum[:]),
	}
}

func sanitizeIntegrationUsagePart(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "/", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func shortIntegrationUsageHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:8])
}
