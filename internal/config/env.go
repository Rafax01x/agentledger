package config

import (
	"os"
	"path/filepath"
	"strings"
)

// MinEnvSecretLen is the minimum length of a .env value that gets
// masked. Short values (flags, ports, single words) are too generic
// to mask safely: masking them would corrupt ordinary output.
const MinEnvSecretLen = 6

// LoadEnvSecrets reads KEY=VALUE pairs from root/.env and returns the
// values longer than MinEnvSecretLen characters. The file itself is
// never copied or stored: only the in-memory values are returned, for
// masking. A missing .env file yields no secrets and no error.
func LoadEnvSecrets(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseEnvValues(string(data)), nil
}

// ParseEnvValues extracts secret-length values from .env text:
// "export KEY=value" lines, comments and blank lines are skipped,
// surrounding quotes are stripped, and values of 6 characters or
// fewer are dropped.
func ParseEnvValues(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		value = strings.TrimSpace(value)
		// Strip an inline comment only when the value is unquoted:
		// a "#" inside quotes may be part of the secret.
		if !strings.HasPrefix(value, "\"") && !strings.HasPrefix(value, "'") {
			if idx := strings.Index(value, " #"); idx >= 0 {
				value = strings.TrimSpace(value[:idx])
			}
		}
		value = unquote(value)
		if len(value) > MinEnvSecretLen {
			out = append(out, value)
		}
	}
	return out
}
