# Provider Setup and Secrets Operations Runbook

This runbook covers end-to-end setup for all three gateway providers: Telegram, Discord, and Feishu.

## Prerequisites

- Carrier daemon and gateway are deployed and accessible.
- You have admin/owner access for the messaging platforms.
- `gh` CLI or equivalent for secrets management (if using GitHub Actions for CI).

---

## 1. Telegram Setup

### 1.1 Create a Bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather).
2. Send `/newbot` and follow the prompts to name your bot.
3. Copy the **HTTP API token** (format: `123456:ABC-DEF...`).

### 1.2 Configure Webhook

```bash
curl -X POST "https://api.telegram.org/bot<TOKEN>/setWebhook" \
  -d "url=https://<YOUR_GATEWAY_HOST>/webhook/telegram" \
  -d "secret_token=<YOUR_WEBHOOK_SECRET>"
```

Gateway-side verification should validate `X-Telegram-Bot-Api-Secret-Token` against `CARRIER_TELEGRAM_WEBHOOK_SECRET`.

### 1.3 Store Secrets

Set the following environment variables for the gateway:

| Variable | Description | Example |
|----------|-------------|---------|
| `CARRIER_TELEGRAM_BOT_TOKEN` | Bot API token from BotFather | `123456:ABC-DEF1234ghIkl-zyx57W2v` |
| `CARRIER_TELEGRAM_WEBHOOK_SECRET` | Optional webhook secret for request validation | `my-random-secret-string` |

```bash
# Local deployment
export CARRIER_TELEGRAM_BOT_TOKEN="<token>"
export CARRIER_TELEGRAM_WEBHOOK_SECRET="<secret>"

# GitHub Actions (for CI)
gh secret set CARRIER_TELEGRAM_BOT_TOKEN --body "<token>"
```

---

## 2. Discord Setup

### 2.1 Create an Application

1. Go to [Discord Developer Portal](https://discord.com/developers/applications).
2. Click **New Application** → name it → **Create**.
3. Under **Bot** → **Add Bot** → copy the **Bot Token**.
4. Under **General Information** → copy the **Application ID** and **Public Key**.

### 2.2 Configure Interactions Endpoint

1. In the Discord Developer Portal, go to **General Information**.
2. Set **Interactions Endpoint URL** to: `https://<YOUR_GATEWAY_HOST>/webhook/discord`
3. Discord will send a verification ping — the gateway must respond correctly.

Gateway-side verification should validate `X-Signature-Ed25519` and `X-Signature-Timestamp` against `CARRIER_DISCORD_PUBLIC_KEY`.

### 2.3 Invite Bot to Server

```
https://discord.com/api/oauth2/authorize?client_id=<APP_ID>&permissions=2048&scope=bot%20applications.commands
```

### 2.4 Store Secrets

| Variable | Description |
|----------|-------------|
| `CARRIER_DISCORD_BOT_TOKEN` | Bot token from Developer Portal |
| `CARRIER_DISCORD_PUBLIC_KEY` | Public key for interaction verification |
| `CARRIER_DISCORD_APP_ID` | Application ID |

```bash
export CARRIER_DISCORD_BOT_TOKEN="<token>"
export CARRIER_DISCORD_PUBLIC_KEY="<public-key>"
export CARRIER_DISCORD_APP_ID="<app-id>"
```

---

## 3. Feishu (Lark) Setup

### 3.1 Create an Application

1. Go to [Feishu Open Platform](https://open.feishu.cn/app).
2. Click **Create Custom App**.
3. Under **Credentials & Basic Info**, copy the **App ID** and **App Secret**.

### 3.2 Configure Event Subscription

1. In the app settings, go to **Event Subscriptions**.
2. Set **Request URL** to: `https://<YOUR_GATEWAY_HOST>/webhook/feishu`
3. Copy the **Verification Token** and **Encrypt Key** (if encryption is enabled).

### 3.3 Add Bot Capability

1. Under **App Features**, enable **Bot**.
2. Publish the app version and request approval from your tenant admin.

### 3.4 Store Secrets

| Variable | Description |
|----------|-------------|
| `CARRIER_FEISHU_APP_ID` | App ID from Open Platform |
| `CARRIER_FEISHU_APP_SECRET` | App Secret |
| `CARRIER_FEISHU_VERIFICATION_TOKEN` | Event verification token |
| `CARRIER_FEISHU_ENCRYPT_KEY` | Event encryption key (optional) |

```bash
export CARRIER_FEISHU_APP_ID="<app-id>"
export CARRIER_FEISHU_APP_SECRET="<app-secret>"
export CARRIER_FEISHU_VERIFICATION_TOKEN="<token>"
```

---

## 4. Secrets Management Best Practices

### Local Development

Use a `.env` file (git-ignored) for local secrets:

```bash
# .env (DO NOT COMMIT)
CARRIER_TELEGRAM_BOT_TOKEN=...
CARRIER_DISCORD_BOT_TOKEN=...
CARRIER_FEISHU_APP_SECRET=...
```

### Production

- Use your platform's secret manager (e.g., AWS Secrets Manager, Vault, GitHub Actions secrets).
- Rotate tokens periodically.
- Use the minimum required bot permissions.
- Monitor for token leaks in logs (the gateway redacts known secret env vars in diagnostic output).

### Verification

After configuring each provider, verify the webhook is active:

```bash
# Telegram: check webhook info
curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"

# Discord: the Interactions Endpoint URL shows a green checkmark in the portal

# Feishu: the Event Subscription page shows "Verified" status
```

---

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| Telegram webhook not receiving | URL unreachable from Telegram servers | Ensure gateway is publicly accessible; check firewall rules |
| Discord verification fails | Gateway not handling the ping interaction type | Ensure webhook route responds to type 1 (PING) with type 1 (PONG) |
| Feishu events not arriving | App not published or bot not added to group | Publish app version and add bot to target group |
| Secret rotation breaks delivery | Old token still cached | Restart gateway after updating env vars |
