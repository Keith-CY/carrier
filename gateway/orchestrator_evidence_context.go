package gateway

import (
	"carrier/shared/work"
	"os"
	"sort"
	"strings"
)

func buildOrchestratorEvidenceProviderAttribution(execution OrchestratorExecution) OrchestratorEvidenceProviderAttribution {
	triggerLabel, _ := executionTriggerAttributionKey(execution)
	out := OrchestratorEvidenceProviderAttribution{
		Team:        strings.TrimSpace(execution.Team),
		Project:     strings.TrimSpace(execution.Project),
		Environment: strings.TrimSpace(execution.Environment),
		TemplateID:  strings.TrimSpace(execution.TemplateID),
		Trigger:     strings.TrimSpace(triggerLabel),
		Initiator:   strings.TrimSpace(execution.Initiator),
	}
	if len(execution.Governance.ProviderResolutions) == 0 {
		return out
	}

	providerAggregates := map[string]*orchestratorProviderAggregateSnapshot{}
	providerLatencyTotals := map[string]int64{}
	providerLatencyCounts := map[string]int64{}
	modelAggregates := map[string]*orchestratorProviderModelAggregateSnapshot{}
	modelLatencyTotals := map[string]int64{}
	modelLatencyCounts := map[string]int64{}
	var totalEstimatedCostUSD float64

	for _, resolution := range execution.Governance.ProviderResolutions {
		provider := strings.ToLower(strings.TrimSpace(resolution.Provider))
		if provider == "" {
			continue
		}
		aggregate := providerAggregates[provider]
		if aggregate == nil {
			aggregate = &orchestratorProviderAggregateSnapshot{Provider: provider}
			providerAggregates[provider] = aggregate
		}
		aggregate.Successes += resolution.SuccessfulTasks
		aggregate.Failures += resolution.FailedTasks
		aggregate.EstimatedCostUSD += resolution.EstimatedCostUSD
		totalEstimatedCostUSD += resolution.EstimatedCostUSD

		taskCount := int64(resolution.SuccessfulTasks + resolution.FailedTasks)
		if resolution.AvgLatencyMs > 0 && taskCount > 0 {
			providerLatencyTotals[provider] += resolution.AvgLatencyMs * taskCount
			providerLatencyCounts[provider] += taskCount
		}

		model := strings.TrimSpace(resolution.Model)
		if model == "" {
			continue
		}
		modelKey := provider + "\x00" + strings.ToLower(model)
		modelAggregate := modelAggregates[modelKey]
		if modelAggregate == nil {
			modelAggregate = &orchestratorProviderModelAggregateSnapshot{
				Provider: provider,
				Model:    model,
			}
			modelAggregates[modelKey] = modelAggregate
		}
		modelAggregate.Successes += resolution.SuccessfulTasks
		modelAggregate.Failures += resolution.FailedTasks
		modelAggregate.EstimatedCostUSD += resolution.EstimatedCostUSD
		if resolution.AvgLatencyMs > 0 && taskCount > 0 {
			modelLatencyTotals[modelKey] += resolution.AvgLatencyMs * taskCount
			modelLatencyCounts[modelKey] += taskCount
		}
	}

	for provider, aggregate := range providerAggregates {
		if count := providerLatencyCounts[provider]; count > 0 {
			aggregate.AvgLatencyMs = providerLatencyTotals[provider] / count
		}
		aggregate.EstimatedCostUSD = roundProviderAggregateCost(aggregate.EstimatedCostUSD)
		out.Providers = append(out.Providers, *aggregate)
	}
	sort.SliceStable(out.Providers, func(i, j int) bool {
		if out.Providers[i].EstimatedCostUSD != out.Providers[j].EstimatedCostUSD {
			return out.Providers[i].EstimatedCostUSD > out.Providers[j].EstimatedCostUSD
		}
		return out.Providers[i].Provider < out.Providers[j].Provider
	})

	for modelKey, aggregate := range modelAggregates {
		if count := modelLatencyCounts[modelKey]; count > 0 {
			aggregate.AvgLatencyMs = modelLatencyTotals[modelKey] / count
		}
		aggregate.EstimatedCostUSD = roundProviderAggregateCost(aggregate.EstimatedCostUSD)
		out.Models = append(out.Models, *aggregate)
	}
	sort.SliceStable(out.Models, func(i, j int) bool {
		if out.Models[i].EstimatedCostUSD != out.Models[j].EstimatedCostUSD {
			return out.Models[i].EstimatedCostUSD > out.Models[j].EstimatedCostUSD
		}
		if out.Models[i].Provider != out.Models[j].Provider {
			return out.Models[i].Provider < out.Models[j].Provider
		}
		return out.Models[i].Model < out.Models[j].Model
	})

	out.TotalEstimatedCostUSD = roundProviderAggregateCost(totalEstimatedCostUSD)
	return out
}

func buildOrchestratorEvidenceMediaOutputs(execution OrchestratorExecution) []OrchestratorEvidenceMediaOutput {
	renderMode := strings.TrimSpace(execution.Outcome.RenderMode)
	out := make([]OrchestratorEvidenceMediaOutput, 0, len(execution.Outcome.Artifacts))
	for _, artifact := range execution.Outcome.Artifacts {
		mediaType := strings.TrimSpace(artifact.MediaType)
		outputRole := strings.TrimSpace(artifact.OutputRole)
		if mediaType == "" && outputRole != "generated" {
			continue
		}
		deliveryRef := strings.TrimSpace(firstString(artifact.DownloadURL, artifact.ExternalID, artifact.Path))
		out = append(out, OrchestratorEvidenceMediaOutput{
			ArtifactID:     strings.TrimSpace(artifact.ID),
			AttachmentID:   strings.TrimSpace(artifact.AttachmentID),
			Name:           strings.TrimSpace(artifact.Name),
			Kind:           strings.TrimSpace(artifact.Kind),
			OutputRole:     outputRole,
			MediaType:      mediaType,
			RenderMode:     renderMode,
			Transport:      resolveEvidenceMediaTransport(artifact),
			DeliveryMethod: resolveEvidenceMediaDeliveryMethod(artifact),
			DeliveryKind:   evidenceMediaDeliveryKind(artifact),
			DeliveryRef:    deliveryRef,
			PreviewText:    resolveEvidenceMediaPreviewText(artifact),
			Source:         strings.TrimSpace(artifact.Source),
		})
	}
	return out
}

func buildOrchestratorEvidenceWorkContext(execution OrchestratorExecution) (*work.WorkItem, *work.Run, *OrchestratorEvidenceWorkspaceManifest, []workGitHubPublishRecord, error) {
	workContext := normalizeOrchestratorExecutionWorkContext(execution.Work)
	if execution.Mode != OrchestratorExecutionModeWork && workContext.ProjectID == "" && workContext.WorkItemID == "" && workContext.RunID == "" && workContext.WorkspaceID == "" {
		return nil, nil, nil, nil, nil
	}

	var runSnapshot *work.Run
	runID := strings.TrimSpace(workContext.RunID)
	if runID != "" {
		run, ok, err := getWorkRun(runID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if ok {
			runCopy := run
			runSnapshot = &runCopy
		}
	}

	workItemID := strings.TrimSpace(workContext.WorkItemID)
	if workItemID == "" && runSnapshot != nil {
		workItemID = strings.TrimSpace(runSnapshot.WorkItemID)
	}
	var workItemSnapshot *work.WorkItem
	if workItemID != "" {
		item, ok, err := getWorkItem(workItemID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if ok {
			itemCopy := item
			workItemSnapshot = &itemCopy
		}
	}

	var workspaceManifest *OrchestratorEvidenceWorkspaceManifest
	workspacePath := strings.TrimSpace(workContext.WorkspacePath)
	if workspacePath == "" && runSnapshot != nil {
		workspacePath = strings.TrimSpace(runSnapshot.WorkspacePath)
	}
	if workContext.WorkspaceID != "" || workspacePath != "" || runSnapshot != nil {
		manifest := &OrchestratorEvidenceWorkspaceManifest{
			WorkspaceID:        strings.TrimSpace(firstString(workContext.WorkspaceID, valueFromRun(runSnapshot, func(r work.Run) string { return r.WorkspaceID }))),
			WorkspacePath:      workspacePath,
			Backend:            strings.TrimSpace(firstString(workContext.Backend, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.Backend) }))),
			WorkflowDigest:     strings.TrimSpace(firstString(workContext.WorkflowDigest, valueFromRun(runSnapshot, func(r work.Run) string { return r.WorkflowDigest }))),
			Phase:              strings.TrimSpace(firstString(workContext.Phase, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.Phase) }))),
			LeaseOwner:         valueFromRun(runSnapshot, func(r work.Run) string { return r.LeaseOwner }),
			LeaseExpiresAt:     valueFromRun(runSnapshot, func(r work.Run) string { return r.LeaseExpiresAt }),
			VerificationStatus: strings.TrimSpace(firstString(workContext.VerificationStatus, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.VerificationStatus) }))),
			PublishStatus:      strings.TrimSpace(firstString(workContext.PublishStatus, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.PublishStatus) }))),
		}
		if manifest.WorkspacePath != "" {
			if _, err := os.Stat(manifest.WorkspacePath); err == nil {
				manifest.Exists = true
			} else if err != nil && !os.IsNotExist(err) {
				return nil, nil, nil, nil, err
			}
		}
		workspaceManifest = manifest
	}

	projectID := strings.TrimSpace(workContext.ProjectID)
	if projectID == "" && runSnapshot != nil {
		projectID = strings.TrimSpace(runSnapshot.ProjectID)
	}
	publishRecords, err := listGitHubPublishRecords(projectID, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return workItemSnapshot, runSnapshot, workspaceManifest, publishRecords, nil
}

func valueFromRun(run *work.Run, selector func(work.Run) string) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(selector(*run))
}

func evidenceMediaDeliveryKind(artifact OrchestratorArtifact) string {
	mediaType := strings.ToLower(strings.TrimSpace(artifact.MediaType))
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return "image"
	case strings.HasPrefix(mediaType, "audio/ogg") || strings.Contains(mediaType, "opus") || kind == "voice":
		return "voice"
	case strings.HasPrefix(mediaType, "audio/") || kind == "audio":
		return "audio"
	case strings.HasPrefix(mediaType, "video/") || kind == "video":
		return "video"
	case mediaType != "" || kind == "document" || kind == "file":
		return "document"
	default:
		return "unknown"
	}
}

func resolveEvidenceMediaTransport(artifact OrchestratorArtifact) string {
	return strings.TrimSpace(firstString(artifact.Transport, artifact.Source))
}

func resolveEvidenceMediaDeliveryMethod(artifact OrchestratorArtifact) string {
	if method := strings.TrimSpace(artifact.DeliveryMethod); method != "" {
		return method
	}
	kind := evidenceMediaDeliveryKind(artifact)
	switch strings.TrimSpace(resolveEvidenceMediaTransport(artifact)) {
	case "telegram":
		method, _ := telegramMediaMethodAndField(kind)
		if method != "" {
			return method
		}
	}
	if kind == "unknown" {
		return ""
	}
	return kind
}

func resolveEvidenceMediaPreviewText(artifact OrchestratorArtifact) string {
	if preview := strings.TrimSpace(artifact.PreviewText); preview != "" {
		return preview
	}
	name := strings.TrimSpace(artifact.Name)
	if name == "" {
		return ""
	}
	switch evidenceMediaDeliveryKind(artifact) {
	case "image":
		return "Image: " + name
	case "audio", "voice":
		return "Audio: " + name
	case "video":
		return "Video: " + name
	default:
		return "Attachment: " + name
	}
}
