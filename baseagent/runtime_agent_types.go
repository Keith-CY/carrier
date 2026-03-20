package baseagent

type AgentState struct {
	ID           string
	Install      string
	Runtime      string
	Health       string
	RestartCount int
}

type UpgradeResult struct {
	AgentID     string
	FromVersion string
	ToVersion   string
}
