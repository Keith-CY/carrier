package gateway

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
)

func buildOrchestratorEvidenceArchive(bundle OrchestratorEvidenceBundle, cfg *GatewayConfig) ([]byte, error) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	addJSON := func(name string, value interface{}) error {
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = writer.Write(append(data, '\n'))
		return err
	}

	if err := addJSON("bundle.json", bundle); err != nil {
		return nil, err
	}
	if err := addJSON("execution.json", bundle.Execution); err != nil {
		return nil, err
	}
	if err := addJSON("plan.json", bundle.Plan); err != nil {
		return nil, err
	}
	if err := addJSON("policy.json", bundle.Policy); err != nil {
		return nil, err
	}
	if err := addJSON("governance.json", bundle.Governance); err != nil {
		return nil, err
	}
	if err := addJSON("provider-attribution.json", bundle.ProviderAttribution); err != nil {
		return nil, err
	}
	if err := addJSON("authorization.json", bundle.Authorization); err != nil {
		return nil, err
	}
	if err := addJSON("worker-leases.json", bundle.WorkerLeases); err != nil {
		return nil, err
	}
	if err := addJSON("results.json", bundle.Results); err != nil {
		return nil, err
	}
	if err := addJSON("result-summary.json", bundle.ResultSummary); err != nil {
		return nil, err
	}
	if err := addJSON("artifact-manifest.json", bundle.ArtifactManifest); err != nil {
		return nil, err
	}
	if err := addJSON("media-outputs.json", bundle.MediaOutputs); err != nil {
		return nil, err
	}
	if bundle.WorkItemSnapshot != nil {
		if err := addJSON("work-item-snapshot.json", bundle.WorkItemSnapshot); err != nil {
			return nil, err
		}
	}
	if bundle.RunSnapshot != nil {
		if err := addJSON("run-snapshot.json", bundle.RunSnapshot); err != nil {
			return nil, err
		}
	}
	if bundle.WorkspaceManifest != nil {
		if err := addJSON("workspace-manifest.json", bundle.WorkspaceManifest); err != nil {
			return nil, err
		}
	}
	if len(bundle.PublishRecords) > 0 {
		if err := addJSON("publish-records.json", bundle.PublishRecords); err != nil {
			return nil, err
		}
	}
	if err := addJSON("audit.json", bundle.Audit); err != nil {
		return nil, err
	}

	usedNames := map[string]int{}
	for _, artifact := range bundle.ArtifactManifest {
		data, filename, _, err := loadExecutionArtifact(cfg, artifact)
		if err != nil {
			continue
		}
		entryName := evidenceArchiveArtifactEntryName(filename, artifact.ID, usedNames)
		writer, err := archive.Create(entryName)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func evidenceArchiveArtifactEntryName(filename, artifactID string, used map[string]int) string {
	base := filepath.Base(strings.TrimSpace(filename))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = strings.TrimSpace(artifactID)
	}
	if base == "" {
		base = "artifact"
	}
	entry := "artifacts/" + base
	if used[entry] == 0 {
		used[entry] = 1
		return entry
	}
	trimmedID := strings.TrimSpace(artifactID)
	if trimmedID == "" {
		trimmedID = "artifact"
	}
	entry = "artifacts/" + trimmedID + "-" + base
	if used[entry] == 0 {
		used[entry] = 1
		return entry
	}
	used[entry]++
	return "artifacts/" + trimmedID + "-" + strconv.Itoa(used[entry]) + "-" + base
}
