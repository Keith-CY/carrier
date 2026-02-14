package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

const RedactedValue = "***REDACTED***"

var sensitiveKeyParts = []string{
	"API_KEY",
	"SECRET",
	"TOKEN",
	"PASSWORD",
	"CREDENTIAL",
}

var sensitiveTextPattern = regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API_KEY|SECRET|TOKEN|PASSWORD|CREDENTIAL)[A-Z0-9_]*)\b(\s*[:=]\s*"?)([^,\s"'\n]+)"?`)

// urlCredentialPattern matches credentials embedded in URLs like postgres://user:pass@host
var urlCredentialPattern = regexp.MustCompile(`(://[^:/@\s]+):([^@\s]+)@`)

type ArtifactMetadata struct {
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	SHA256    string    `json:"sha256"`
}

func IsSensitiveKeyName(key string) bool {
	upper := strings.ToUpper(key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

func RedactEnviron(environ []string) map[string]string {
	redacted := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, found := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if IsSensitiveKeyName(key) {
			redacted[key] = RedactedValue
		} else if found {
			redacted[key] = RedactText(value)
		} else {
			redacted[key] = ""
		}
	}
	return redacted
}

func RedactText(input string) string {
	result := sensitiveTextPattern.ReplaceAllString(input, "${1}${2}"+RedactedValue)
	result = urlCredentialPattern.ReplaceAllString(result, "${1}:"+RedactedValue+"@")
	return result
}

func BuildMetadata(createdAt time.Time, expiration time.Duration, artifacts map[string][]byte) ArtifactMetadata {
	if expiration <= 0 {
		expiration = 24 * time.Hour
	}
	return ArtifactMetadata{
		CreatedAt: createdAt.UTC(),
		ExpiresAt: createdAt.UTC().Add(expiration),
		SHA256:    ArtifactChecksum(artifacts),
	}
}

func MetadataJSON(createdAt time.Time, expiration time.Duration, artifacts map[string][]byte) ([]byte, error) {
	metadata := BuildMetadata(createdAt, expiration, artifacts)
	return json.MarshalIndent(metadata, "", "  ")
}

func ArtifactChecksum(artifacts map[string][]byte) string {
	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	sort.Strings(names)

	hash := sha256.New()
	for _, name := range names {
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(artifacts[name])
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}
