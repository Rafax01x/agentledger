package redact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"agentledger/internal/config"
)

// CustomPattern is one caller-supplied redaction rule, mirroring
// config.Pattern without making redact depend on config's YAML/JSON
// decoding details.
type CustomPattern struct {
	Name  string
	Regex string
}

// Redactor applies the built-in secret rules plus custom patterns
// and exact .env secret values. It is the redaction engine used for
// a run once its project configuration is known; the package-level
// Redact function keeps the built-in rules only, for callers with
// no configuration.
type Redactor struct {
	custom     []rule
	envSecrets []string
}

// NewRedactor compiles custom patterns and keeps env secrets longer
// than 6 characters. An invalid custom regex is an error: a broken
// pattern must fail the run, not silently leave secrets unmasked.
func NewRedactor(custom []CustomPattern, envSecrets []string) (*Redactor, error) {
	r := &Redactor{}
	for _, p := range custom {
		if p.Name == "" || p.Regex == "" {
			return nil, fmt.Errorf("redact pattern requires both name and regex")
		}
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			return nil, fmt.Errorf("redact pattern %q: invalid regex: %w", p.Name, err)
		}
		r.custom = append(r.custom, rule{pattern: re, replacement: "<REDACTED:" + p.Name + ">"})
	}
	for _, s := range envSecrets {
		if len(s) > config.MinEnvSecretLen {
			r.envSecrets = append(r.envSecrets, s)
		}
	}
	// Longest first: a short value that is a substring of a longer
	// secret must not mask the longer one partially.
	sort.Slice(r.envSecrets, func(i, j int) bool { return len(r.envSecrets[i]) > len(r.envSecrets[j]) })
	return r, nil
}

// NewFromConfig builds a Redactor from a loaded project config. When
// cfg.RedactEnvFiles is true, values are read from root/.env at this
// moment; the file is never stored.
func NewFromConfig(cfg *config.Config, root string) (*Redactor, error) {
	if cfg == nil {
		cfg = config.Default()
	}
	var custom []CustomPattern
	for _, p := range cfg.RedactPatterns {
		custom = append(custom, CustomPattern{Name: p.Name, Regex: p.Regex})
	}
	var secrets []string
	if cfg.RedactEnvFiles {
		var err error
		secrets, err = config.LoadEnvSecrets(root)
		if err != nil {
			return nil, fmt.Errorf("read .env for redaction: %w", err)
		}
	}
	return NewRedactor(custom, secrets)
}

// Redact applies the built-in rules, then custom patterns, then
// exact .env secret values, in that order.
func (r *Redactor) Redact(s string) string {
	for _, rule := range rules {
		s = rule.pattern.ReplaceAllString(s, rule.replacement)
	}
	for _, rule := range r.custom {
		s = rule.pattern.ReplaceAllString(s, rule.replacement)
	}
	for _, secret := range r.envSecrets {
		s = strings.ReplaceAll(s, secret, "<REDACTED:ENV_SECRET>")
	}
	return s
}
