# Gateway

Carrier gateway runtime lives in this top-level Go module.

Key responsibilities:
- command ingress (`/command`)
- provider webhooks (`/webhook/*`)
- session/rate-limit/download-token handling
- WebUI API surface (`/api/v1/*`)
- canonical channel registry and inbound routing normalization
- unified auth/status APIs for provider and channel setup state

Run tests:
```bash
cd gateway
go test ./...
```

Useful endpoints:
- `GET /api/v1/auth/providers`: redacted provider auth status (`configured`, `reusable`, saved credential metadata)
- `GET /api/v1/channels`: canonical channel capabilities plus redacted setup/configuration state

Current supported channel descriptors:
- `telegram`
- `discord`
- `feishu`
- `webui`

Channel traffic is normalized into one shared routing layer before command/chat dispatch. Transport-specific handlers still own signature verification and response formatting at the edge.
