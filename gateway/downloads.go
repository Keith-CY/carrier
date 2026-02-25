package gateway

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	defaultDownloadTTL      = 5 * time.Minute
	downloadCleanupInterval = 1 * time.Minute
)

// DownloadToken is an issued download token.
type DownloadToken struct {
	Token     string
	FileRef   string
	ExpiresAt time.Time
	SingleUse bool
}

type downloadRecord struct {
	DownloadToken
	consumedAt *time.Time
}

// DownloadStore manages download tokens.
type DownloadStore struct {
	mu           sync.Mutex
	tokens       map[string]*downloadRecord
	now          func() time.Time
	artifactRoot string
	stopCleanup  chan struct{}
}

// NewDownloadStore creates a new download store. artifactRoot restricts file serving.
func NewDownloadStore(artifactRoot string, now func() time.Time) *DownloadStore {
	if now == nil {
		now = time.Now
	}
	return &DownloadStore{
		tokens:       make(map[string]*downloadRecord),
		now:          now,
		artifactRoot: artifactRoot,
		stopCleanup:  make(chan struct{}),
	}
}

// StartPeriodicCleanup begins background expiry.
func (d *DownloadStore) StartPeriodicCleanup() *DownloadStore {
	go func() {
		ticker := time.NewTicker(downloadCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.Cleanup()
			case <-d.stopCleanup:
				return
			}
		}
	}()
	return d
}

// Stop halts background cleanup.
func (d *DownloadStore) Stop() {
	close(d.stopCleanup)
}

// Issue creates a new download token for fileRef with the given TTL.
func (d *DownloadStore) Issue(fileRef string, ttl time.Duration, singleUse bool) *DownloadToken {
	if ttl <= 0 {
		ttl = defaultDownloadTTL
	}
	token := "dl-" + uuid.New().String()
	rec := &downloadRecord{
		DownloadToken: DownloadToken{
			Token:     token,
			FileRef:   fileRef,
			ExpiresAt: d.now().Add(ttl),
			SingleUse: singleUse,
		},
	}
	d.mu.Lock()
	d.tokens[token] = rec
	d.mu.Unlock()
	t := rec.DownloadToken
	return &t
}

// Consume validates and (for single-use) marks a token as consumed.
// Returns nil if token is invalid or expired.
func (d *DownloadStore) Consume(token string) *DownloadToken {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec := d.tokens[token]
	if rec == nil {
		return nil
	}
	if d.now().After(rec.ExpiresAt) {
		delete(d.tokens, token)
		return nil
	}
	if rec.SingleUse && rec.consumedAt != nil {
		return nil
	}
	if rec.SingleUse {
		now := d.now()
		rec.consumedAt = &now
	}
	t := rec.DownloadToken
	return &t
}

// FinalizeConsumed removes a single-use token after the file has been served.
func (d *DownloadStore) FinalizeConsumed(token string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec := d.tokens[token]
	if rec == nil || !rec.SingleUse || rec.consumedAt == nil {
		return
	}
	delete(d.tokens, token)
}

// Cleanup removes expired/consumed tokens. Returns count removed.
func (d *DownloadStore) Cleanup() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	removed := 0
	now := d.now()
	for token, rec := range d.tokens {
		expired := now.After(rec.ExpiresAt)
		consumed := rec.SingleUse && rec.consumedAt != nil
		if expired || consumed {
			delete(d.tokens, token)
			removed++
		}
	}
	return removed
}

// Size returns the number of live tokens.
func (d *DownloadStore) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tokens)
}

// ToDownloadURL builds the download URL for a token.
func (d *DownloadStore) ToDownloadURL(tok *DownloadToken) string {
	base := filepath.Base(tok.FileRef)
	if strings.TrimSpace(base) == "" || base == "." || base == "/" {
		base = "artifact.zip"
	}
	return fmt.Sprintf("/downloads/%s/%s", tok.Token, url.PathEscape(base))
}

// ParseDownloadPath parses /downloads/{token}/{filename} → token, filename.
func ParseDownloadPath(path string) (token, filename string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "downloads" {
		return "", "", false
	}
	token = parts[1]
	if token == "" {
		return "", "", false
	}
	decoded, err := url.PathUnescape(parts[2])
	if err != nil || decoded == "" {
		return "", "", false
	}
	return token, decoded, true
}

// ExpectedFileName returns the base name of a fileRef.
func ExpectedFileName(fileRef string) string {
	base := filepath.Base(fileRef)
	if base == "" || base == "." {
		return "artifact.bin"
	}
	return base
}

// BuildContentDisposition builds a Content-Disposition header value.
func BuildContentDisposition(filename string) string {
	hasNonASCII := false
	for _, r := range filename {
		if r > unicode.MaxASCII {
			hasNonASCII = true
			break
		}
	}
	if hasNonASCII {
		asciiFallback := strings.Map(func(r rune) rune {
			if r > unicode.MaxASCII {
				return '_'
			}
			return r
		}, filename)
		escaped := strings.ReplaceAll(asciiFallback, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		encoded := url.PathEscape(filename)
		return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, escaped, encoded)
	}
	escaped := strings.ReplaceAll(filename, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`attachment; filename="%s"`, escaped)
}

// ValidateArtifactRoot validates that the artifact root is safe and returns the resolved path.
func ValidateArtifactRoot(root string) (string, error) {
	resolved, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve artifact root: %w", err)
	}

	if resolved == "/" {
		return "", fmt.Errorf("[security] ARTIFACT_ROOT cannot be system root: %s", resolved)
	}

	blockedBelow := []string{"/etc", "/usr", "/var", "/bin", "/sbin", "/boot", "/sys", "/proc"}
	blockedExact := []string{"/root"}

	sep := string(os.PathSeparator)
	for _, d := range blockedBelow {
		if resolved == d || strings.HasPrefix(resolved, d+sep) {
			return "", fmt.Errorf("[security] ARTIFACT_ROOT cannot be in system directory: %s", resolved)
		}
	}
	for _, d := range blockedExact {
		if resolved == d {
			return "", fmt.Errorf("[security] ARTIFACT_ROOT cannot be in system directory: %s", resolved)
		}
	}
	return resolved, nil
}

// IsPathUnderRoot returns true if filePath is under (or equal to) root.
func IsPathUnderRoot(filePath, root string) bool {
	sep := string(os.PathSeparator)
	rootWithSep := root
	if !strings.HasSuffix(rootWithSep, sep) {
		rootWithSep += sep
	}
	return filePath == root || strings.HasPrefix(filePath, rootWithSep)
}
