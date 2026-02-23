# Carrier CLI reference

This page summarizes the implemented CLI command surface and the recommended usage flow.

## Command surface

- `carrier`
  - Bootstrap command.
  - If no onboard config exists, it runs `carrier onboard`.
  - If already onboarded, it ensures daemon and gateway are running and then exits.
- `carrier onboard`
  - Interactive onboarding flow (TUI).
- `carrier onboard --webui`
  - Browser/WebUI onboarding flow.
- `carrier add <agent_id>`
  - Add/install agent via TUI flow.
  - Managed agents (`openclaw`, `picoclaw`, `zeroclaw`) use managed-agent setup and instance tracking.
  - Non-managed agent IDs run direct daemon install + start.
- `carrier add <agent_id> --webui`
  - Browser/WebUI add flow.
- `carrier list`
  - Lists running managed agent instances.
- `carrier stop`
  - Stops background services started by Carrier: daemon and gateway.
- `carrier stop <instance_id>`
  - Stops one managed agent instance.
- `carrier uninstall <instance_id>`
  - Uninstalls and removes a managed agent instance.
- `carrier daemon`
  - Starts Carrier daemon in the foreground.
- `carrier gateway`
  - Starts Carrier gateway in the foreground.

## Recommended flow (bootstrap-first)

1. `carrier`
2. `carrier onboard` (Telegram-first TUI flow)
3. `carrier add <agent_id>`

Use `--webui` when interactive terminal access is unavailable or when onboarding needs a browser-assisted path.

## TUI vs WebUI vs chat commands

- Use TUI when:
  - You are running from terminal and can complete setup interactively.
  - Provider setup is comfortable in terminal.
- Use WebUI when:
  - You are doing Telegram-less onboarding (Discord/Feishu) or using managed agents from a browser flow.
  - Manual setup of provider credentials is needed outside the TUI path.
- Use chat commands when:
  - You are already paired in chat and need quick operational commands.
  - You are not relying on primary CLI onboarding/install flow.

`/install` and `/onboard` chat commands still parse at gateway level, but onboarding/install actions in chat mode are intentionally blocked (`E_INSTALL_GUI_ONLY` / `E_ONBOARD_GUI_ONLY`) to avoid credential handling in chat.

## Concise example flows

- Local CLI bootstrap + managed TUI add

```bash
carrier
carrier onboard
carrier add openclaw
carrier list
```

- Browser/WebUI onboarding + managed add (Discord/Feishu recommended)

```bash
carrier onboard --webui
carrier add openclaw --webui
carrier list
```

- Direct add flow for non-managed agents

```bash
carrier add <agent_id>
carrier stop
# For managed-instance flows:
carrier stop <instance_id>
carrier uninstall <instance_id>
```

## Daemon and gateway helpers

- `carrier stop` only manages background carrier services and does not remove instance metadata.
- `carrier stop <instance_id>` and `carrier uninstall <instance_id>` require a managed instance ID from `carrier list`.
- `carrier daemon` and `carrier gateway` are foreground commands and keep process running until interrupted.
