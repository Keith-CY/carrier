package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

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
		if err := parseYAMLManifest(trimmed, &m); err != nil {
			return PackageManifest{}, err
		}
	}

	if err := validateManifest(&m); err != nil {
		return PackageManifest{}, err
	}
	return m, nil
}

func parseYAMLManifest(data []byte, m *PackageManifest) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	section := ""
	collectionIdx := -1
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		raw := strings.TrimRight(scanner.Text(), " \t\r")
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			collectionIdx = -1
			if strings.HasSuffix(line, ":") {
				key := strings.TrimSpace(strings.TrimSuffix(line, ":"))
				switch key {
				case "provenance", "collections", "mount":
					section = key
					continue
				default:
					return fmt.Errorf("%w: unsupported section %q on line %d", ErrManifestInvalid, key, lineNo)
				}
			}

			key, val, ok := splitYAMLKV(line)
			if !ok {
				return fmt.Errorf("%w: malformed line %d", ErrManifestInvalid, lineNo)
			}
			section = ""
			setManifestTopLevel(m, key, val)
			continue
		}

		switch section {
		case "provenance":
			key, val, ok := splitYAMLKV(line)
			if !ok {
				return fmt.Errorf("%w: malformed provenance line %d", ErrManifestInvalid, lineNo)
			}
			switch key {
			case "source":
				m.Provenance.Source = val
			case "uri":
				m.Provenance.URI = val
			case "digest":
				m.Provenance.Digest = val
			}
		case "mount":
			key, val, ok := splitYAMLKV(line)
			if !ok {
				return fmt.Errorf("%w: malformed mount line %d", ErrManifestInvalid, lineNo)
			}
			switch key {
			case "default_mode":
				m.Mount.DefaultMode = AccessMode(val)
			case "default_slot":
				m.Mount.DefaultSlot = val
			}
		case "collections":
			if strings.HasPrefix(line, "- ") {
				collectionIdx++
				m.Collections = append(m.Collections, CollectionSpec{})
				rest := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if rest == "" {
					continue
				}
				key, val, ok := splitYAMLKV(rest)
				if !ok {
					return fmt.Errorf("%w: malformed collections line %d", ErrManifestInvalid, lineNo)
				}
				setCollectionField(&m.Collections[collectionIdx], key, val)
				continue
			}
			if collectionIdx < 0 {
				return fmt.Errorf("%w: malformed collections block before first list item on line %d", ErrManifestInvalid, lineNo)
			}
			key, val, ok := splitYAMLKV(line)
			if !ok {
				return fmt.Errorf("%w: malformed collections line %d", ErrManifestInvalid, lineNo)
			}
			setCollectionField(&m.Collections[collectionIdx], key, val)
		default:
			return fmt.Errorf("%w: unexpected indented line %d", ErrManifestInvalid, lineNo)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: scan manifest: %v", ErrManifestInvalid, err)
	}
	return nil
}

func splitYAMLKV(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	val := parseYAMLScalar(parts[1])
	if key == "" {
		return "", "", false
	}
	return key, val, true
}

func parseYAMLScalar(raw string) string {
	v := strings.TrimSpace(raw)
	if idx := strings.Index(v, " #"); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
	}
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func setManifestTopLevel(m *PackageManifest, key, val string) {
	switch key {
	case "schema_version":
		m.SchemaVersion = val
	case "id":
		m.ID = val
	case "name":
		m.Name = val
	case "version":
		m.Version = val
	case "region":
		m.Region = normalizeRegion(val)
	case "type":
		m.Kind = val
	case "publisher":
		m.Publisher = val
	}
}

func setCollectionField(c *CollectionSpec, key, val string) {
	switch key {
	case "id":
		c.ID = val
	case "path":
		c.Path = val
	case "sensitivity":
		c.Sensitivity = val
	case "default_mount":
		c.DefaultMount = val
	}
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
	if strings.TrimSpace(m.Provenance.Source) == "" || strings.TrimSpace(m.Provenance.URI) == "" || strings.TrimSpace(m.Provenance.Digest) == "" {
		return fmt.Errorf("%w: provenance.source, provenance.uri, and provenance.digest are required", ErrManifestInvalid)
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
	if m.Region == TypePublic {
		m.Mount.DefaultMode = AccessReadOnly
	}
	if m.Region == TypePerAgent {
		m.Mount.DefaultMode = AccessReadWrite
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
