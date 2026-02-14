# Memory Import/Export Pipeline and Artifact Delivery

> **Issue:** #78 · **Status:** Plan · **Track:** B2 · **Depends on:** #77 (B1)

## Goals

- Design the import API that ingests memory packages, validates them, and persists artifacts.
- Design the export API that packages stored memories for download.
- Define the download-token lifecycle (creation, expiry, revocation).
- Specify rollback and duplicate-handling behavior.
- Provide an E2E test plan covering the full pipeline.

## Non-Goals

- Streaming/chunked uploads (Phase 2).
- Multi-node replication or CDN distribution.
- Package signing and trust chains (Phase 2).

---

## 1. Import API and Persistence Flow

### Endpoint

```
POST /api/v1/memory/import
Content-Type: multipart/form-data
```

### Request

| Part | Type | Required | Description |
|------|------|----------|-------------|
| `manifest` | `file` | ✅ | `memory.yaml` file |
| `artifact` | `file` | ✅ | Tar-gzipped artifact bundle |

### Flow

```
Client ──POST──▶ Gateway ──validate──▶ Validator (#77)
                                          │
                              ┌───────────┴───────────┐
                              │ invalid               │ valid
                              ▼                       ▼
                         400 + errors          Check duplicates
                                                      │
                                          ┌───────────┴───────────┐
                                          │ exists (same ver)     │ new
                                          ▼                       ▼
                                     409 Conflict           Persist to store
                                     (see §4)                     │
                                                                  ▼
                                                           201 Created
                                                           { id, version, status }
```

### Persistence Layout

```
/data/carrier/memory/
  └── <package-id>/
      └── <version>/
          ├── memory.yaml
          ├── artifact.tar.gz
          └── .meta.json        # internal: import timestamp, importer, checksum verified
```

### Response

```json
{
  "id": "sentiment-model-en",
  "version": "1.2.0",
  "status": "imported",
  "imported_at": "2026-02-14T08:30:00Z"
}
```

---

## 2. Export API and Packaging Flow

### Endpoint

```
POST /api/v1/memory/export
```

### Request Body

```json
{
  "id": "sentiment-model-en",
  "version": "1.2.0"     // optional; defaults to latest
}
```

### Flow

1. Look up package in store.
2. If not found → `404`.
3. Generate a time-limited download token.
4. Return download URL with embedded token.

### Response

```json
{
  "download_url": "/api/v1/memory/download?token=<token>",
  "expires_at": "2026-02-14T09:30:00Z",
  "package": {
    "id": "sentiment-model-en",
    "version": "1.2.0",
    "size": 52430848
  }
}
```

### Download Endpoint

```
GET /api/v1/memory/download?token=<token>
```

Returns the tar-gzipped artifact bundle with `memory.yaml` included. Content-Disposition header set for download.

---

## 3. Download-Token Lifecycle Policy

| Property | Value |
|----------|-------|
| Format | UUID v4 + HMAC signature |
| TTL | 1 hour (configurable) |
| Max uses | 1 (single-use by default, configurable up to 5) |
| Storage | In-memory map with TTL eviction; persisted to SQLite for crash recovery |
| Revocation | `DELETE /api/v1/memory/tokens/{token}` |
| Cleanup | Background goroutine sweeps expired tokens every 5 minutes |

### Token Object

```json
{
  "token": "550e8400-e29b-41d4-a716-446655440000.hmac_sig",
  "package_id": "sentiment-model-en",
  "package_version": "1.2.0",
  "created_at": "2026-02-14T08:30:00Z",
  "expires_at": "2026-02-14T09:30:00Z",
  "uses_remaining": 1,
  "revoked": false
}
```

---

## 4. Rollback and Re-Import Behavior for Duplicates

### Duplicate Detection

A package is considered a duplicate if `(id, version)` already exists in the store.

| Scenario | Behavior |
|----------|----------|
| Same `id` + `version`, same checksum | `200 OK` — idempotent, no-op |
| Same `id` + `version`, different checksum | `409 Conflict` with error message |
| Same `id`, new `version` | Normal import (new version) |

### Force Re-Import

```
POST /api/v1/memory/import?force=true
```

When `force=true` and a conflict exists:
1. Move existing version to `.backup/` with timestamp.
2. Import new version.
3. Return `200 OK` with `"status": "replaced"` and backup reference.

### Rollback

```
POST /api/v1/memory/rollback
{
  "id": "sentiment-model-en",
  "version": "1.2.0"
}
```

Restores the most recent backup for the given `(id, version)`. Returns `404` if no backup exists.

---

## 5. E2E Test Plan

| # | Test Case | Steps | Expected |
|---|-----------|-------|----------|
| 1 | Happy path import | POST valid manifest + artifact | 201, files persisted |
| 2 | Happy path export | Export existing package | 200 with download URL |
| 3 | Download with valid token | GET download URL | 200, tar.gz body |
| 4 | Download with expired token | Wait > TTL, GET | 401 |
| 5 | Download with used token | Use token twice | 2nd request → 401 |
| 6 | Import invalid manifest | Missing required field | 400 with validation errors |
| 7 | Import duplicate (same checksum) | Import same package twice | 200, idempotent |
| 8 | Import duplicate (diff checksum) | Same id+version, diff content | 409 |
| 9 | Force re-import | `?force=true` on conflict | 200, backup created |
| 10 | Rollback | Rollback after force re-import | Original restored |
| 11 | Export non-existent | Export unknown id | 404 |
| 12 | Token revocation | Revoke token, then download | 401 |
| 13 | Large artifact | Import 500MB+ package | 201, no timeout |
| 14 | Concurrent imports | 10 parallel imports of different packages | All succeed |

---

## Acceptance Criteria

1. Import endpoint validates manifest using #77 validator before persisting.
2. Export endpoint generates single-use, time-limited download tokens.
3. Duplicate handling follows the policy table above.
4. Force re-import creates a recoverable backup.
5. All 14 E2E test cases documented and at least 8 automated.

## Timeline Estimate

| Task | Estimate |
|------|----------|
| Import API implementation | 2 days |
| Export API + token lifecycle | 2 days |
| Rollback and duplicate logic | 1 day |
| E2E tests | 2 days |
| **Total** | **~7 days** |
