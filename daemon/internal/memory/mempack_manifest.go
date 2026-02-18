package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
var sha256ManifestDigestPattern = regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`)

func parseManifest(data []byte) (PackageManifest, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return PackageManifest{}, fmt.Errorf("%w: empty manifest", ErrManifestInvalid)
	}

	var m PackageManifest
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if err := json.Unmarshal(trimmed, &m); err != nil {
			return PackageManifest{}, fmt.Errorf("%w: parse json manifest: %v", ErrManifestInvalid, err)
		}
	} else {
		if err := yaml.Unmarshal(trimmed, &m); err != nil {
			return PackageManifest{}, fmt.Errorf("%w: parse yaml manifest: %v", ErrManifestInvalid, err)
		}
	}

	if err := validateManifest(&m); err != nil {
		return PackageManifest{}, err
	}
	return m, nil
}

func validateManifest(m *PackageManifest) error {
	m.Region = normalizeRegion(string(m.Region))

	if strings.TrimSpace(m.SchemaVersion) == "" {
		return fmt.Errorf("%w: schema_version is required", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("%w: version is required", ErrManifestInvalid)
	}
	if !semverPattern.MatchString(m.Version) {
		return fmt.Errorf("%w: version must be semver, got %q", ErrManifestInvalid, m.Version)
	}
	if m.Region == "" {
		return fmt.Errorf("%w: region is required", ErrManifestInvalid)
	}
	switch m.Region {
	case TypePerAgent, TypeShared, TypePublic:
	default:
		return fmt.Errorf("%w: unsupported region %q", ErrManifestInvalid, m.Region)
	}
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("%w: type is required", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Provenance.Source) == "" {
		return fmt.Errorf("%w: provenance.source is required", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Provenance.URI) == "" {
		return fmt.Errorf("%w: provenance.uri is required", ErrManifestInvalid)
	}
	digest := strings.TrimSpace(m.Provenance.Digest)
	if digest == "" {
		return fmt.Errorf("%w: provenance.digest is required", ErrManifestInvalid)
	}
	if !sha256ManifestDigestPattern.MatchString(digest) {
		return fmt.Errorf("%w: provenance.digest must be sha256:<64 hex>, got %q", ErrManifestInvalid, m.Provenance.Digest)
	}
	if len(m.Collections) == 0 {
		return fmt.Errorf("%w: collections must not be empty", ErrManifestInvalid)
	}
	seenCollections := make(map[string]struct{}, len(m.Collections))
	for _, c := range m.Collections {
		if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Path) == "" {
			return fmt.Errorf("%w: each collection requires id and path", ErrManifestInvalid)
		}
		if _, ok := seenCollections[c.ID]; ok {
			return fmt.Errorf("%w: duplicate collection id %q", ErrManifestInvalid, c.ID)
		}
		seenCollections[c.ID] = struct{}{}
	}

	if m.Mount.DefaultMode == "" {
		m.Mount.DefaultMode = Policy{}.DefaultAccessMode(m.Region)
	}
	if m.Mount.DefaultMode != AccessReadOnly && m.Mount.DefaultMode != AccessReadWrite {
		return fmt.Errorf("%w: mount.default_mode must be ro or rw", ErrManifestInvalid)
	}
	if strings.TrimSpace(m.Mount.DefaultSlot) == "" {
		m.Mount.DefaultSlot = "default"
	}
	if m.Region == TypePublic && m.Mount.DefaultMode != AccessReadOnly {
		return fmt.Errorf("%w: public region requires mount.default_mode=ro", ErrManifestInvalid)
	}
	if m.Region == TypePerAgent && m.Mount.DefaultMode != AccessReadWrite {
		return fmt.Errorf("%w: private/per-agent region requires mount.default_mode=rw", ErrManifestInvalid)
	}

	return nil
}

func normalizeRegion(region string) Type {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "private", "per_agent", "per-agent":
		return TypePerAgent
	case "shared":
		return TypeShared
	case "public":
		return TypePublic
	default:
		return Type(region)
	}
}
