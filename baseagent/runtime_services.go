package baseagent

import (
	"context"
	"fmt"
	"strings"
)

func (r *Runtime) ListInstalledSkills(ctx context.Context) []SkillDefinition {
	if r == nil || r.skillsLoader == nil {
		return nil
	}
	return r.skillsLoader.ListInstalledSkills(ctx)
}

func (r *Runtime) RecentSubagentJobs(ctx context.Context, limit int) []SubagentJob {
	if r == nil || r.subagentManager == nil {
		return nil
	}
	return r.subagentManager.RecentJobs(ctx, limit)
}

func (r *Runtime) RecentSessionStats(limit int) []SessionStats {
	if r == nil || r.sessions == nil {
		return nil
	}
	return r.sessions.ListStats(limit)
}

func (r *Runtime) SubagentJob(ctx context.Context, jobID string) (SubagentJob, error) {
	if r == nil || r.subagentManager == nil {
		return SubagentJob{}, fmt.Errorf("subagent manager is unavailable")
	}
	return r.subagentManager.Job(ctx, jobID)
}

func (r *Runtime) SearchSkills(ctx context.Context, query string) []SkillDefinition {
	if r == nil || r.skillsLoader == nil {
		return nil
	}
	return r.skillsLoader.SearchSkills(ctx, query)
}

func (r *Runtime) InstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
	if r == nil || r.skillsLoader == nil {
		return SkillDefinition{}, fmt.Errorf("skills loader is unavailable")
	}
	return r.skillsLoader.InstallSkill(ctx, name)
}

func (r *Runtime) ReinstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
	if r == nil || r.skillsLoader == nil {
		return SkillDefinition{}, fmt.Errorf("skills loader is unavailable")
	}
	return r.skillsLoader.ReinstallSkill(ctx, name)
}

func (r *Runtime) UpdateSkill(ctx context.Context, name, version string) (SkillDefinition, error) {
	if r == nil || r.skillsLoader == nil {
		return SkillDefinition{}, fmt.Errorf("skills loader is unavailable")
	}
	return r.skillsLoader.UpdateSkill(ctx, name, version)
}

func (r *Runtime) UninstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
	if r == nil || r.skillsLoader == nil {
		return SkillDefinition{}, fmt.Errorf("skills loader is unavailable")
	}
	return r.skillsLoader.UninstallSkill(ctx, name)
}

func (r *Runtime) StartMCP(ctx context.Context) error {
	if r == nil || r.mcpManager == nil {
		return nil
	}
	manager, ok := r.mcpManager.(interface{ Start(context.Context) error })
	if !ok {
		return nil
	}
	return manager.Start(ctx)
}

func (r *Runtime) StopMCP(ctx context.Context) error {
	if r == nil || r.mcpManager == nil {
		return nil
	}
	manager, ok := r.mcpManager.(interface{ Stop(context.Context) error })
	if !ok {
		return nil
	}
	return manager.Stop(ctx)
}

func (r *Runtime) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	if r == nil || r.mcpManager == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	if err := r.mcpManager.SetServerEnabled(ctx, name, enabled); err != nil {
		return err
	}
	if r.loop != nil {
		r.loop.SetExecutionTools(r.loop.executionTools, r.loop.maxToolIterations, r.effectiveStructuredToolPolicy(), r.mcpManager, r.subagentManager)
	}
	return nil
}

func (r *Runtime) SetMCPServerAttached(ctx context.Context, name string, attached bool) error {
	if r == nil || r.mcpManager == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	if err := r.mcpManager.SetServerAttached(ctx, name, attached); err != nil {
		return err
	}
	if r.loop != nil {
		r.loop.SetExecutionTools(r.loop.executionTools, r.loop.maxToolIterations, r.effectiveStructuredToolPolicy(), r.mcpManager, r.subagentManager)
	}
	return nil
}

func (r *Runtime) UpdateMCPServerConfig(ctx context.Context, name string, config string) error {
	if r == nil || r.mcpManager == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	if err := r.mcpManager.UpdateServerConfig(ctx, name, config); err != nil {
		return err
	}
	if r.loop != nil {
		r.loop.SetExecutionTools(r.loop.executionTools, r.loop.maxToolIterations, r.effectiveStructuredToolPolicy(), r.mcpManager, r.subagentManager)
	}
	return nil
}

func (r *Runtime) MCPServerDetail(_ context.Context, name string) (MCPServerCapability, error) {
	if r == nil || r.mcpManager == nil {
		return MCPServerCapability{}, fmt.Errorf("mcp manager is unavailable")
	}
	return r.mcpManager.ServerDetail(name)
}

func (r *Runtime) effectiveStructuredToolPolicy() StructuredToolPolicySpec {
	if r == nil {
		return StructuredToolPolicySpec{}
	}
	if r.structuredToolPolicyOverride != nil {
		return *r.structuredToolPolicyOverride
	}
	return ActiveBoundarySpec().StructuredToolPolicy
}

func (r *Runtime) ScheduleJob(ctx context.Context, job CronJob) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.Schedule(ctx, job)
}

func (r *Runtime) ListCronJobs(_ context.Context, sessionKey string) ([]CronJob, error) {
	if r == nil || r.cron == nil {
		return nil, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.List(sessionKey), nil
}

func (r *Runtime) CancelCronJob(_ context.Context, jobID string) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.Cancel(jobID)
}

func (r *Runtime) PauseCronJob(_ context.Context, jobID string) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.Pause(jobID)
}

func (r *Runtime) ResumeCronJob(_ context.Context, jobID string) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.Resume(jobID)
}

func (r *Runtime) RunCronJob(ctx context.Context, jobID string) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.RunNow(ctx, jobID)
}

func normalizedAttachmentOutputRole(attachment AttachmentRef) string {
	if role := strings.TrimSpace(attachment.OutputRole); role != "" {
		return role
	}
	return "generated"
}
