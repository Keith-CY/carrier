package gateway

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxSSHConfigIncludeDepth = 8

func listLocalSSHConfigHosts() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	configPath := filepath.Join(home, ".ssh", "config")
	hosts := map[string]struct{}{}
	seenFiles := map[string]struct{}{}
	if err := collectSSHConfigHostsFromFile(configPath, 0, seenFiles, hosts); err != nil {
		return nil, err
	}
	list := make([]string, 0, len(hosts))
	for host := range hosts {
		list = append(list, host)
	}
	sort.Strings(list)
	return list, nil
}

func collectSSHConfigHostsFromFile(path string, depth int, seenFiles, hosts map[string]struct{}) error {
	if depth > maxSSHConfigIncludeDepth {
		return nil
	}

	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "." || cleaned == "" {
		return nil
	}
	if abs, err := filepath.Abs(cleaned); err == nil {
		cleaned = abs
	}

	if _, seen := seenFiles[cleaned]; seen {
		return nil
	}
	seenFiles[cleaned] = struct{}{}

	file, err := os.Open(cleaned)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open ssh config %s: %w", cleaned, err)
	}
	defer file.Close()

	baseDir := filepath.Dir(cleaned)
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(stripSSHConfigComment(scanner.Text()))
		if line == "" {
			continue
		}
		key, value, ok := splitSSHConfigDirective(line)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "host":
			addSSHConfigHostPatterns(hosts, parseSSHConfigFields(value))
		case "include":
			includes, includeErr := resolveSSHConfigIncludes(baseDir, parseSSHConfigFields(value))
			if includeErr != nil {
				return fmt.Errorf("resolve include in %s:%d: %w", cleaned, lineNo, includeErr)
			}
			for _, includePath := range includes {
				if recurseErr := collectSSHConfigHostsFromFile(includePath, depth+1, seenFiles, hosts); recurseErr != nil {
					return recurseErr
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan ssh config %s: %w", cleaned, err)
	}
	return nil
}

func stripSSHConfigComment(line string) string {
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	for idx, r := range line {
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			if !inSingleQuote {
				escaped = true
			}
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return line[:idx]
			}
		}
	}
	return line
}

func splitSSHConfigDirective(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	idx := strings.IndexAny(trimmed, " \t")
	if idx <= 0 {
		return "", "", false
	}
	key := trimmed[:idx]
	value := strings.TrimSpace(trimmed[idx+1:])
	if value == "" {
		return "", "", false
	}
	return key, value, true
}

func parseSSHConfigFields(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	fields := make([]string, 0, 4)
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	flush := func() {
		token := strings.TrimSpace(current.String())
		if token != "" {
			fields = append(fields, token)
		}
		current.Reset()
	}

	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		switch r {
		case '\\':
			if inSingleQuote {
				current.WriteRune(r)
			} else {
				escaped = true
			}
		case '\'':
			if inDoubleQuote {
				current.WriteRune(r)
			} else {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if inSingleQuote {
				current.WriteRune(r)
			} else {
				inDoubleQuote = !inDoubleQuote
			}
		case ' ', '\t':
			if inSingleQuote || inDoubleQuote {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	flush()
	return fields
}

func addSSHConfigHostPatterns(hosts map[string]struct{}, patterns []string) {
	for _, raw := range patterns {
		pattern := strings.TrimSpace(strings.Trim(raw, `"'`))
		if !isConcreteSSHHostPattern(pattern) {
			continue
		}
		hosts[pattern] = struct{}{}
	}
}

func isConcreteSSHHostPattern(pattern string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "!") {
		return false
	}
	if strings.ContainsAny(pattern, "*?[]") {
		return false
	}
	return true
}

func resolveSSHConfigIncludes(baseDir string, patterns []string) ([]string, error) {
	out := make([]string, 0, len(patterns))
	seen := map[string]struct{}{}
	appendPath := func(path string) {
		cleaned := filepath.Clean(path)
		if cleaned == "." || cleaned == "" {
			return
		}
		if abs, err := filepath.Abs(cleaned); err == nil {
			cleaned = abs
		}
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}

	for _, rawPattern := range patterns {
		expanded := expandSSHConfigPath(rawPattern, baseDir)
		if expanded == "" {
			continue
		}
		if hasGlobMeta(expanded) {
			matches, err := filepath.Glob(expanded)
			if err != nil {
				return nil, err
			}
			sort.Strings(matches)
			for _, match := range matches {
				info, statErr := os.Stat(match)
				if statErr != nil || info.IsDir() {
					continue
				}
				appendPath(match)
			}
			continue
		}
		info, err := os.Stat(expanded)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		appendPath(expanded)
	}
	return out, nil
}

func expandSSHConfigPath(path, baseDir string) string {
	trimmed := strings.TrimSpace(strings.Trim(path, `"'`))
	if trimmed == "" {
		return ""
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			if trimmed == "~" {
				trimmed = home
			} else {
				trimmed = filepath.Join(home, trimmed[2:])
			}
		}
	}
	if !filepath.IsAbs(trimmed) {
		trimmed = filepath.Join(baseDir, trimmed)
	}
	return filepath.Clean(trimmed)
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
