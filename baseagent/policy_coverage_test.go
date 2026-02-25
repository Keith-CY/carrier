package baseagent

import "testing"

func TestClassifyRepairActionRiskPreservesExplicitHighRisk(t *testing.T) {
	got := ClassifyRepairActionRisk(RepairAction{
		Command:    "npm install",
		TargetPath: "./",
		RiskLevel:  RiskHigh,
	})
	if got != RiskHigh {
		t.Fatalf("expected explicit high risk to be preserved, got %q", got)
	}
}
