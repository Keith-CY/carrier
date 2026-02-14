import { handleCommand, parseInput } from "./index";
import { InMemoryDaemonClient } from "./daemon/client";

const daemon = new InMemoryDaemonClient();
daemon.setRemoteDiagnosisState("openclaw", {
  needsRemoteDiagnosis: true,
  lastDiagnoseFile: "/tmp/openclaw-diagnose.zip",
});

const example = "telegram 123 req-1 /diagnose-consent openclaw yes";
const response = await handleCommand(parseInput(example), daemon);
console.log(JSON.stringify(response, null, 2));
