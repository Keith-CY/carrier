# Memory Package Specification and Validator

> **Issue:** #77 · **Status:** ⚠️ Superseded (diverged from implementation) · **Track:** B1
>
> **Note:** This is a historical planning document.
> For current behavior, use [`docs/current-architecture.md`](../current-architecture.md), [`daemon/internal/memory/types.go`](../../daemon/internal/memory/types.go), [`daemon/internal/memory/store.go`](../../daemon/internal/memory/store.go), and [`daemon/internal/memory/policy.go`](../../daemon/internal/memory/policy.go).

## Goals

- Define a `memory.yaml` schema that every memory package must conform to.
- Build a validator that produces clear, actionable error messages.
- Provide developer documentation and examples for creating valid packages.
- Ensure the schema is versioned and extensible.

## Non-Goals

- Runtime loading/execution of memory packages (covered by #78).
- Binary artifact storage format (this covers metadata only).
- GUI for package creation.

---

## 1. Schema Design

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | ✅ | Unique identifier, lowercase alphanumeric + hyphens, 3-128 chars. Pattern: `^[a-z0-9][a-z0-9-]{1,126}[a-z0-9]$` |
| `version` | `string` | ✅ | Semantic version (`major.minor.patch`). Pattern: `^\d+\.\d+\.\d+$` |
| `type` | `string` | ✅ | Package type. One of: `model`, `dataset`, `config`, `plugin` |
| `owner` | `string` | ✅ | GitHub handle or org identifier of the package owner |
| `checksum` | `string` | ✅ | SHA-256 hex digest of the artifact payload |
| `name` | `string` | ❌ | Human-readable display name (defaults to `id`) |
| `description` | `string` | ❌ | Short description, max 500 chars |
| `license` | `string` | ❌ | SPDX license identifier |
| `created_at` | `string` | ❌ | ISO 8601 timestamp of creation |
| `tags` | `string[]` | ❌ | Freeform tags for discovery, max 20 |
| `dependencies` | `Dependency[]` | ❌ | Other memory packages this depends on |
| `artifacts` | `Artifact[]` | ✅ | List of files included in the package (≥1) |
| `metadata` | `object` | ❌ | Arbitrary key-value pairs for extensibility |

### Dependency Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | ✅ | Dependency package ID |
| `version` | `string` | ✅ | SemVer constraint (e.g. `>=1.0.0`, `^2.1.0`) |

### Artifact Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | `string` | ✅ | Relative path within the package |
| `size` | `integer` | ✅ | File size in bytes |
| `checksum` | `string` | ❌ | Per-file SHA-256 (recommended for packages with multiple artifacts) |

---

## 2. Example `memory.yaml`

```yaml
id: sentiment-model-en
version: 1.2.0
type: model
owner: dev01lay2
checksum: a1b2c3d4e5f6...  # SHA-256 of the full artifact bundle
name: English Sentiment Model
description: Pre-trained sentiment analysis model for English text.
license: MIT
created_at: "2026-02-14T08:00:00Z"
tags:
  - nlp
  - sentiment
  - english
dependencies:
  - id: tokenizer-base
    version: ">=1.0.0"
artifacts:
  - path: model/weights.bin
    size: 52428800
    checksum: f7e8d9c0b1a2...
  - path: model/config.json
    size: 1024
metadata:
  framework: pytorch
  accuracy: "0.94"
```

---

## 3. JSON Schema

A formal JSON Schema is provided at [`docs/plans/memory-schema.json`](memory-schema.json) for tooling integration (IDE validation, CI checks).

---

## 4. Validator Design

### Interface

```go
// Package memvalidator provides memory.yaml validation.
package memvalidator

type ValidationError struct {
    Field   string // e.g. "version", "artifacts[0].path"
    Message string // human-readable, actionable
    Code    string // machine-readable error code
}

type ValidationResult struct {
    Valid  bool
    Errors []ValidationError
}

// Validate parses and validates a memory.yaml byte slice.
func Validate(data []byte) ValidationResult
```

### Error Codes

| Code | Description | Example Message |
|------|-------------|-----------------|
| `MISSING_FIELD` | Required field is absent | `Field "id" is required but missing` |
| `INVALID_FORMAT` | Field value doesn't match expected pattern | `Field "version" must be semver (got "v1.2")` |
| `INVALID_TYPE` | Field type is wrong | `Field "type" must be one of: model, dataset, config, plugin` |
| `CHECKSUM_MISMATCH` | Computed checksum doesn't match declared | `Checksum mismatch for artifact "model/weights.bin"` |
| `VERSION_CONFLICT` | Dependency version constraint unsatisfiable | `Dependency "tokenizer-base" version ">=2.0.0" conflicts with available "1.3.0"` |
| `DUPLICATE_ARTIFACT` | Same path listed twice | `Duplicate artifact path "model/config.json"` |
| `ID_FORMAT` | ID doesn't match pattern | `Field "id" must be lowercase alphanumeric with hyphens, 3-128 chars` |
| `TOO_MANY_TAGS` | Tags array exceeds limit | `Maximum 20 tags allowed, got 25` |

### Validation Order

1. YAML syntax check
2. Required fields presence
3. Field format validation (patterns, types, ranges)
4. Cross-field validation (checksum verification, dependency resolution)
5. Warnings (missing optional but recommended fields)

---

## 5. Test Plan

| Test Case | Input | Expected |
|-----------|-------|----------|
| Valid minimal package | id + version + type + owner + checksum + 1 artifact | `Valid: true` |
| Valid full package | All fields populated | `Valid: true` |
| Missing required field (`id`) | No `id` field | `MISSING_FIELD` on `id` |
| Missing required field (`version`) | No `version` | `MISSING_FIELD` on `version` |
| Invalid semver | `version: "1.2"` | `INVALID_FORMAT` on `version` |
| Invalid type | `type: "unknown"` | `INVALID_TYPE` on `type` |
| Invalid ID format | `id: "UPPER_CASE"` | `ID_FORMAT` on `id` |
| Checksum mismatch | Wrong checksum value | `CHECKSUM_MISMATCH` |
| Duplicate artifact paths | Two artifacts with same path | `DUPLICATE_ARTIFACT` |
| Too many tags | 25 tags | `TOO_MANY_TAGS` |
| Version conflict in deps | Unsatisfiable constraint | `VERSION_CONFLICT` |
| Empty artifacts array | `artifacts: []` | `MISSING_FIELD` on `artifacts` |
| Malformed YAML | Broken YAML syntax | Parse error with line number |

---

## 6. Developer Documentation

### Creating a Memory Package

1. Create a `memory.yaml` in your package root.
2. Fill in required fields: `id`, `version`, `type`, `owner`, `checksum`, `artifacts`.
3. Compute the checksum: `sha256sum <artifact-bundle> | awk '{print $1}'`
4. Validate: `carrier memory validate memory.yaml`
5. Import: `carrier memory import ./my-package/`

### Common Mistakes

- **Forgetting checksum**: Always compute after final build, not before.
- **Using uppercase in ID**: IDs are strictly lowercase.
- **Version without patch**: Use `1.0.0`, not `1.0`.

---

## Acceptance Criteria

1. JSON Schema file merged and usable by IDEs / CI.
2. Validator package compiles and passes all test cases listed above.
3. Error messages are actionable (tell the user what to fix, not just what's wrong).
4. Developer docs with at least 2 complete examples merged.

## Timeline Estimate

| Task | Estimate |
|------|----------|
| Schema design and JSON Schema file | 1 day |
| Validator implementation | 2 days |
| Tests | 1 day |
| Developer docs | 0.5 day |
| **Total** | **~4.5 days** |
