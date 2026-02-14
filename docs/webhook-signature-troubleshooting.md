# Webhook Signature Failure Troubleshooting

Common diagnosis steps when provider webhook signature verification fails.

## Common symptoms

- HTTP 401 or 403 from your webhook endpoint
- Signature mismatch errors in gateway logs
- Provider shows webhook delivery failure

---

## Telegram

1. **Verify bot token** — Confirm `TELEGRAM_BOT_TOKEN` matches the token from [@BotFather](https://core.telegram.org/bots#botfather).
2. **Check webhook URL** — Run: `curl https://api.telegram.org/bot<TOKEN>/getWebhookInfo` and verify the `url` field.
3. **Check secret token** — If using `secret_token` in `setWebhook`, ensure your server validates `X-Telegram-Bot-Api-Secret-Token` header with the same value.
4. **Clock drift** — Ensure server time is within ±60s of NTP. Run: `date -u` and compare.

Telegram docs: https://core.telegram.org/bots/api#setwebhook

---

## Discord

1. **Verify public key** — Confirm `DISCORD_PUBLIC_KEY` in your environment matches the "Public Key" from the Discord Developer Portal → Application → General Information.
2. **Validate signature implementation** — Discord uses Ed25519 signatures. Verify you are checking `X-Signature-Ed25519` and `X-Signature-Timestamp` headers against the raw request body.
3. **Check for body parsing issues** — Signature must be computed on the raw body bytes, not a re-serialized JSON string. Ensure no middleware modifies the body before verification.

Discord docs: https://discord.com/developers/docs/interactions/receiving-and-responding#security-and-authorization

---

## Feishu (Lark)

1. **Verify Encrypt Key** — Confirm `FEISHU_ENCRYPT_KEY` matches the value in Feishu Open Platform → Event Subscription settings.
2. **Verify Verification Token** — Confirm `FEISHU_VERIFICATION_TOKEN` matches the token shown in the same settings page.
3. **Check event version** — Feishu v1 and v2 events use different signature schemes. Ensure your handler matches the version configured in the console.
4. **Decrypt test** — For encrypted events, verify AES-256-CBC decryption with your encrypt key produces valid JSON.

Feishu docs: https://open.feishu.cn/document/ukTMukTMukTM/uYDNxYjL2QTM24iN0EjN/event-subscription-configure-/encrypt-key-encryption-configuration-case
