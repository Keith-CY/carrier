# Gateway

Carrier gateway runtime lives in this top-level Go module.

Key responsibilities:
- command ingress (`/command`)
- provider webhooks (`/webhook/*`)
- session/rate-limit/download-token handling
- WebUI API surface (`/api/v1/*`)

Run tests:
```bash
cd gateway
go test ./...
```
