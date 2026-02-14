# Daemon Test Coverage Report

**Date:** 2026-02-14
**Total Coverage:** 76.1%

## Per-Package Coverage

```
carrier/daemon/cmd/agentd/main.go:12:			main				0.0%
carrier/daemon/internal/baseagent/policy.go:89:		IsRepairActionAllowlisted	100.0%
carrier/daemon/internal/baseagent/policy.go:104:	ClassifyRepairActionRisk	85.7%
carrier/daemon/internal/baseagent/policy.go:117:	ValidateRepairAction		100.0%
carrier/daemon/internal/baseagent/policy.go:128:	usesSudo			100.0%
carrier/daemon/internal/baseagent/policy.go:136:	touchesSystemDirectory		100.0%
carrier/daemon/internal/baseagent/types.go:26:		Analyze				0.0%
carrier/daemon/internal/catalog/catalog.go:24:		List				100.0%
carrier/daemon/internal/catalog/catalog.go:29:		ListByStatus			100.0%
carrier/daemon/internal/catalog/catalog.go:40:		DefaultEntries			100.0%
carrier/daemon/internal/catalog/catalog.go:82:		FindByID			75.0%
carrier/daemon/internal/catalog/catalog.go:92:		IsRunnable			100.0%
carrier/daemon/internal/catalog/catalog.go:97:		HasCapability			100.0%
carrier/daemon/internal/catalog/manifests.go:5:		OpenClawManifest		0.0%
carrier/daemon/internal/commandexec/runner.go:25:	NewShellRunner			0.0%
carrier/daemon/internal/commandexec/runner.go:29:	Run				88.9%
carrier/daemon/internal/commandexec/runner.go:48:	exitCode			83.3%
carrier/daemon/internal/lifecycle/service.go:58:	WithRunner			100.0%
carrier/daemon/internal/lifecycle/service.go:62:	WithRuntimeChecker		100.0%
carrier/daemon/internal/lifecycle/service.go:66:	WithDiagnoseDir			100.0%
carrier/daemon/internal/lifecycle/service.go:70:	WithLogLimit			0.0%
carrier/daemon/internal/lifecycle/service.go:82:	WithCrashLoopConfig		0.0%
carrier/daemon/internal/lifecycle/service.go:96:	WithNow				100.0%
carrier/daemon/internal/lifecycle/service.go:104:	WithAuditLogLimit		100.0%
carrier/daemon/internal/lifecycle/service.go:112:	WithIDGenerator			0.0%
carrier/daemon/internal/lifecycle/service.go:120:	WithHandoffRetention		0.0%
carrier/daemon/internal/lifecycle/service.go:128:	WithMemoryStore			0.0%
carrier/daemon/internal/lifecycle/service.go:162:	NewService			100.0%
carrier/daemon/internal/lifecycle/service.go:199:	RegisterManifest		91.7%
carrier/daemon/internal/lifecycle/service.go:236:	ListAgents			0.0%
carrier/daemon/internal/lifecycle/service.go:254:	Install				84.6%
carrier/daemon/internal/lifecycle/service.go:290:	Start				74.1%
carrier/daemon/internal/lifecycle/service.go:365:	Upgrade				82.1%
carrier/daemon/internal/lifecycle/service.go:451:	Stop				63.0%
carrier/daemon/internal/lifecycle/service.go:489:	Status				83.3%
carrier/daemon/internal/lifecycle/service.go:501:	Logs				66.7%
carrier/daemon/internal/lifecycle/service.go:525:	AuditBufferStatus		100.0%
carrier/daemon/internal/lifecycle/service.go:534:	AuditLogs			100.0%
carrier/daemon/internal/lifecycle/service.go:540:	DiagnosisHandoffs		87.5%
carrier/daemon/internal/lifecycle/service.go:554:	CleanupExpiredDiagnosisHandoffs	100.0%
carrier/daemon/internal/lifecycle/service.go:573:	Diagnose			77.3%
carrier/daemon/internal/lifecycle/service.go:606:	CreateRemoteDiagnosisHandoff	94.4%
carrier/daemon/internal/lifecycle/service.go:641:	HandleFailure			87.0%
carrier/daemon/internal/lifecycle/service.go:673:	updateStateOnInstallError	90.9%
carrier/daemon/internal/lifecycle/service.go:688:	updateStateOnStartError		94.4%
carrier/daemon/internal/lifecycle/service.go:717:	updateStateOnUpgradeError	88.9%
carrier/daemon/internal/lifecycle/service.go:730:	checkRuntimePrerequisites	100.0%
carrier/daemon/internal/lifecycle/service.go:737:	formatPreFlightFailures		0.0%
carrier/daemon/internal/lifecycle/service.go:751:	firstFailedCode			0.0%
carrier/daemon/internal/lifecycle/service.go:760:	formatUpgradeFailure		75.0%
carrier/daemon/internal/lifecycle/service.go:768:	nextPatchVersion		69.2%
carrier/daemon/internal/lifecycle/service.go:790:	validateRequiredEnv		100.0%
carrier/daemon/internal/lifecycle/service.go:804:	ensurePortsAvailable		88.9%
carrier/daemon/internal/lifecycle/service.go:819:	blockIfCrashLoopCoolingDown	100.0%
carrier/daemon/internal/lifecycle/service.go:850:	trimRestartHistory		50.0%
carrier/daemon/internal/lifecycle/service.go:867:	describePortOccupant		0.0%
carrier/daemon/internal/lifecycle/service.go:887:	findListeningSocketInode	0.0%
carrier/daemon/internal/lifecycle/service.go:897:	findListeningSocketInodeInFile	0.0%
carrier/daemon/internal/lifecycle/service.go:935:	findProcessBySocketInode	0.0%
carrier/daemon/internal/lifecycle/service.go:974:	MemoryStore			0.0%
carrier/daemon/internal/lifecycle/service.go:977:	autoMountMemories		22.2%
carrier/daemon/internal/lifecycle/service.go:995:	autoUnmountMemories		40.0%
carrier/daemon/internal/lifecycle/service.go:1006:	SetMemoryAttachments		0.0%
carrier/daemon/internal/lifecycle/service.go:1011:	GetMemoryAttachments		0.0%
carrier/daemon/internal/lifecycle/service.go:1015:	getManifestAndState		85.7%
carrier/daemon/internal/lifecycle/service.go:1027:	appendCommandLog		100.0%
carrier/daemon/internal/lifecycle/service.go:1038:	getMemoryAttachments		100.0%
carrier/daemon/internal/lifecycle/service.go:1045:	setMemoryAttachments		100.0%
carrier/daemon/internal/lifecycle/service.go:1051:	envVarKeys			75.0%
carrier/daemon/internal/lifecycle/service.go:1070:	createUpgradeBackup		81.8%
carrier/daemon/internal/lifecycle/service.go:1098:	appendLog			80.0%
carrier/daemon/internal/lifecycle/service.go:1114:	recordAudit			85.7%
carrier/daemon/internal/lifecycle/service.go:1136:	writeDiagnoseZip		75.9%
carrier/daemon/internal/lifecycle/service.go:1191:	addZipFile			66.7%
carrier/daemon/internal/manifest/load.go:13:		LoadFile			72.7%
carrier/daemon/internal/manifest/types.go:110:		Validate			95.2%
carrier/daemon/internal/manifest/types.go:145:		validateRuntime			90.9%
carrier/daemon/internal/manifest/types.go:177:		validateRequired		100.0%
carrier/daemon/internal/manifest/types.go:185:		validateMemory			75.0%
carrier/daemon/internal/manifest/types.go:210:		validateUpgrade			77.8%
carrier/daemon/internal/manifest/types.go:228:		validateEnv			75.0%
carrier/daemon/internal/manifest/types.go:243:		validateNetwork			100.0%
carrier/daemon/internal/manifest/types.go:260:		validateHealth			77.8%
carrier/daemon/internal/manifest/types.go:276:		validateCapabilities		100.0%
carrier/daemon/internal/memory/policy.go:25:		DefaultAccessMode		100.0%
carrier/daemon/internal/memory/policy.go:34:		CheckMount			92.3%
carrier/daemon/internal/memory/policy.go:70:		ResolveAccessMode		100.0%
carrier/daemon/internal/memory/store.go:23:		WithNow				100.0%
carrier/daemon/internal/memory/store.go:28:		NewStore			100.0%
carrier/daemon/internal/memory/store.go:40:		Create				100.0%
carrier/daemon/internal/memory/store.go:64:		Get				83.3%
carrier/daemon/internal/memory/store.go:75:		List				100.0%
carrier/daemon/internal/memory/store.go:86:		Mount				94.4%
carrier/daemon/internal/memory/store.go:126:		Unmount				89.5%
carrier/daemon/internal/memory/store.go:160:		Archive				90.9%
carrier/daemon/internal/memory/store.go:178:		MountsForAgent			100.0%
carrier/daemon/internal/memory/store.go:191:		UnmountAll			92.9%
carrier/daemon/internal/memory/store.go:214:		agentMounts			66.7%
carrier/daemon/internal/memory/types.go:19:		ValidTypes			0.0%
carrier/daemon/internal/memory/types.go:69:		ValidateTransition		85.7%
carrier/daemon/internal/redact/redact.go:34:		IsSensitiveKeyName		100.0%
carrier/daemon/internal/redact/redact.go:44:		RedactEnviron			91.7%
carrier/daemon/internal/redact/redact.go:63:		RedactText			100.0%
carrier/daemon/internal/redact/redact.go:69:		BuildMetadata			100.0%
carrier/daemon/internal/redact/redact.go:80:		MetadataJSON			100.0%
carrier/daemon/internal/redact/redact.go:85:		ArtifactChecksum		100.0%
carrier/daemon/internal/runtimecheck/checker.go:19:	LookPath			100.0%
carrier/daemon/internal/runtimecheck/checker.go:48:	Error				83.3%
carrier/daemon/internal/runtimecheck/checker.go:67:	NewHostChecker			0.0%
carrier/daemon/internal/runtimecheck/checker.go:75:	Check				100.0%
carrier/daemon/internal/runtimecheck/checker.go:84:	collectIssues			94.7%
carrier/daemon/internal/runtimecheck/checker.go:133:	hasTool				75.0%
carrier/daemon/internal/runtimecheck/checker.go:142:	PreFlight			100.0%
carrier/daemon/internal/runtimecheck/checker.go:146:	detectWSL			100.0%
carrier/daemon/internal/runtimecheck/preflight.go:50:	WithGetenv			100.0%
carrier/daemon/internal/runtimecheck/preflight.go:55:	WithListenTCP			100.0%
carrier/daemon/internal/runtimecheck/preflight.go:60:	WithCommandLookPath		100.0%
carrier/daemon/internal/runtimecheck/preflight.go:66:	RunPreFlight			100.0%
carrier/daemon/internal/runtimecheck/preflight.go:102:	checkRuntimePrereqs		75.0%
carrier/daemon/internal/runtimecheck/preflight.go:115:	checkEnvVars			100.0%
carrier/daemon/internal/runtimecheck/preflight.go:144:	checkPorts			86.7%
carrier/daemon/internal/runtimecheck/preflight.go:179:	checkCommandExists		87.5%
total:							(statements)			76.1%
```

## How to Regenerate

```bash
./scripts/coverage.sh
```
