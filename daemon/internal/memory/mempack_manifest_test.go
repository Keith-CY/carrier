package memory

import (
	"strings"
	"testing"
)

func TestParseManifestYAMLSupportsQuotedColonValues(t *testing.T) {
	manifest := strings.TrimSpace(`
schema_version: "1"
id: style-kit
name: "Team: Style"
version: 1.2.3
region: public
type: mixed
publisher: acme
provenance:
  source: market
  uri: "https://example.com/a:b?x=1"
  digest: sha256:0000000000000000000000000000000000000000000000000000000000000000
collections:
  - id: prompts
    path: content/prompts
    sensitivity: medium
    default_mount: "/prompts"
mount:
  default_mode: ro
  default_slot: default
`) + "\n"

	parsed, err := parseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if parsed.Name != "Team: Style" {
		t.Fatalf("expected quoted colon value to parse, got %q", parsed.Name)
	}
	if parsed.Provenance.URI != "https://example.com/a:b?x=1" {
		t.Fatalf("expected uri to parse, got %q", parsed.Provenance.URI)
	}
}

func TestParseManifestYAMLSupportsTrailingComments(t *testing.T) {
	manifest := strings.TrimSpace(`
schema_version: "1" # comment
id: style-kit # comment
name: Team Style # comment
version: 1.2.3 # comment
region: shared
type: mixed
publisher: acme
provenance:
  source: market
  uri: https://example.com/team-style # trailing comment
  digest: sha256:0000000000000000000000000000000000000000000000000000000000000000
collections:
  - id: prompts
    path: content/prompts
    sensitivity: medium
    default_mount: /prompts
mount:
  default_mode: ro
  default_slot: default
`) + "\n"

	parsed, err := parseManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if parsed.ID != "style-kit" {
		t.Fatalf("expected id parsed with trailing comments, got %q", parsed.ID)
	}
}

func TestValidateManifestPublicRequiresReadOnlyDefaultMode(t *testing.T) {
	manifest := PackageManifest{
		SchemaVersion: "1",
		ID:            "style-kit",
		Name:          "Team Style",
		Version:       "1.0.0",
		Region:        TypePublic,
		Kind:          "mixed",
		Provenance: Provenance{
			Source: "market",
			URI:    "https://example.com",
			Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		Collections: []CollectionSpec{{ID: "prompts", Path: "content/prompts"}},
		Mount:       MountDefaults{DefaultMode: AccessReadWrite, DefaultSlot: "default"},
	}

	if err := validateManifest(&manifest); err == nil {
		t.Fatal("expected validation error when public default mode is rw")
	}
}

func TestValidateManifestPrivateRequiresReadWriteDefaultMode(t *testing.T) {
	manifest := PackageManifest{
		SchemaVersion: "1",
		ID:            "style-kit",
		Name:          "Team Style",
		Version:       "1.0.0",
		Region:        TypePerAgent,
		Kind:          "mixed",
		Provenance: Provenance{
			Source: "market",
			URI:    "https://example.com",
			Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		Collections: []CollectionSpec{{ID: "prompts", Path: "content/prompts"}},
		Mount:       MountDefaults{DefaultMode: AccessReadOnly, DefaultSlot: "default"},
	}

	if err := validateManifest(&manifest); err == nil {
		t.Fatal("expected validation error when private default mode is ro")
	}
}
