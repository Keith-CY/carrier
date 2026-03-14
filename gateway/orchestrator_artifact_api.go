package gateway

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func handleOrchestratorExecutionArtifacts(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, execution OrchestratorExecution, parts []string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	artifacts := execution.Outcome.Artifacts
	if len(parts) == 2 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"artifacts": artifacts,
		})
		return
	}
	if len(parts) != 3 {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "artifact not found"))
		return
	}
	artifactID := strings.TrimSpace(parts[2])
	if artifactID == "" {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "artifact not found"))
		return
	}
	artifact, ok := findExecutionArtifact(artifacts, artifactID)
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "artifact not found"))
		return
	}
	if err := serveExecutionArtifact(w, cfg, artifact); err != nil {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "artifact file was not found"))
		return
	}
}

func findExecutionArtifact(artifacts []OrchestratorArtifact, artifactID string) (OrchestratorArtifact, bool) {
	id := strings.TrimSpace(artifactID)
	for _, artifact := range artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.ID), id) {
			return artifact, true
		}
	}
	return OrchestratorArtifact{}, false
}

func serveExecutionArtifact(w http.ResponseWriter, cfg *GatewayConfig, artifact OrchestratorArtifact) error {
	data, filename, contentType, err := loadExecutionArtifact(cfg, artifact)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", BuildContentDisposition(filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	return nil
}

func loadExecutionArtifact(cfg *GatewayConfig, artifact OrchestratorArtifact) ([]byte, string, string, error) {
	path := strings.TrimSpace(artifact.Path)
	if path == "" {
		return nil, "", "", os.ErrNotExist
	}
	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		return nil, "", "", err
	}
	if cfg != nil && strings.TrimSpace(cfg.ArtifactRoot) != "" {
		resolvedRoot, rootErr := ValidateArtifactRoot(cfg.ArtifactRoot)
		if rootErr != nil || !IsPathUnderRoot(resolvedPath, resolvedRoot) {
			return nil, "", "", os.ErrNotExist
		}
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, "", "", err
	}
	contentType := strings.TrimSpace(artifact.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	filename := strings.TrimSpace(artifact.Name)
	if filename == "" {
		filename = filepath.Base(resolvedPath)
	}
	return data, filename, contentType, nil
}
