package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Pattern is one custom redaction rule: any text matching Regex is
// replaced with <REDACTED:NAME>.
type Pattern struct {
	Name  string `json:"name"`
	Regex string `json:"regex"`
}

// Config is the project configuration for one repository.
type Config struct {
	// IgnorePaths are glob patterns of files excluded from recorded
	// snapshots/diffs (and therefore from undo). Patterns are matched
	// against the repository-relative path and against the file's base
	// name, so "*.log" ignores log files at any depth. A trailing "/"
	// pattern ("vendor/") ignores everything under that directory.
	IgnorePaths []string `json:"ignore_paths"`
	// RedactPatterns are custom regex redaction rules applied in
	// addition to the built-in secret shapes.
	RedactPatterns []Pattern `json:"redact_patterns"`
	// RedactEnvFiles, when true (the default), reads values from a
	// local .env file in the repository root and masks any exact
	// value longer than 6 characters in persisted output and diffs.
	// The .env file itself is never stored.
	RedactEnvFiles bool `json:"redact_env_files"`
}

// Default returns the configuration used when no config file exists.
func Default() *Config {
	return &Config{RedactEnvFiles: true}
}

// Load reads .agentledger.yaml (preferred) or .agentledger.json from
// root. A missing file is not an error: Default is returned. A file
// that exists but cannot be parsed is an error, so a typo in the
// config fails loudly instead of silently recording unredacted data.
func Load(root string) (*Config, error) {
	yamlPath := filepath.Join(root, ".agentledger.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", yamlPath, err)
		}
		cfg, err := parseYAML(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", yamlPath, err)
		}
		return cfg, nil
	}
	jsonPath := filepath.Join(root, ".agentledger.json")
	if _, err := os.Stat(jsonPath); err == nil {
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", jsonPath, err)
		}
		var raw struct {
			IgnorePaths    []string  `json:"ignore_paths"`
			RedactPatterns []Pattern `json:"redact_patterns"`
			RedactEnvFiles *bool     `json:"redact_env_files"`
		}
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", jsonPath, err)
		}
		cfg := Default()
		cfg.IgnorePaths = raw.IgnorePaths
		cfg.RedactPatterns = raw.RedactPatterns
		if raw.RedactEnvFiles != nil {
			cfg.RedactEnvFiles = *raw.RedactEnvFiles
		}
		return cfg, nil
	}
	return Default(), nil
}

// MatchIgnore reports whether repo-relative p is excluded by any
// ignore pattern.
func (c *Config) MatchIgnore(p string) bool {
	p = filepath.ToSlash(p)
	base := path.Base(p)
	for _, pattern := range c.IgnorePaths {
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/") {
			if strings.HasPrefix(p, pattern) {
				return true
			}
			continue
		}
		if globMatch(pattern, p) || globMatch(pattern, base) {
			return true
		}
	}
	return false
}

// globMatch matches a glob against s, where "*" matches any run of
// characters except "/", "**" matches any run including "/", and
// "?" matches one character except "/".
func globMatch(pattern, s string) bool {
	re, err := globToRegexp(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i += 2
			} else {
				b.WriteString("[^/]*")
				i++
			}
		case '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// unquote strips one layer of matching single or double quotes.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseYAML parses the supported subset described in doc.go.
func parseYAML(data []byte) (*Config, error) {
	cfg := Default()
	section := "" // "", "ignore", "patterns"
	var current *Pattern
	flush := func() {
		if current != nil && (current.Name != "" || current.Regex != "") {
			cfg.RedactPatterns = append(cfg.RedactPatterns, *current)
		}
		current = nil
	}
	for lineno, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			flush()
			key, value, ok := strings.Cut(trimmed, ":")
			if !ok {
				return nil, fmt.Errorf("line %d: expected key: value", lineno+1)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "ignore_paths":
				if value != "" {
					return nil, fmt.Errorf("line %d: ignore_paths must use one \"- item\" line per pattern, not an inline value", lineno+1)
				}
				section = "ignore"
			case "redact_patterns":
				if value != "" {
					return nil, fmt.Errorf("line %d: redact_patterns must use one \"- name:/regex:\" item per pattern, not an inline value", lineno+1)
				}
				section = "patterns"
			case "redact_env_files":
				section = ""
				switch strings.ToLower(value) {
				case "true":
					cfg.RedactEnvFiles = true
				case "false":
					cfg.RedactEnvFiles = false
				default:
					return nil, fmt.Errorf("line %d: redact_env_files must be true or false", lineno+1)
				}
			default:
				// Unknown keys fail loudly: a typo like
				// "ignor_paths:" must never silently disable
				// ignoring or redaction. Forward compatibility
				// is less important here than failing visibly.
				return nil, fmt.Errorf("line %d: unknown config key %q", lineno+1, key)
			}
			continue
		}
		// Indented content belongs to the current section.
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(trimmed[2:])
			switch section {
			case "ignore":
				cfg.IgnorePaths = append(cfg.IgnorePaths, unquote(item))
			case "patterns":
				flush()
				current = &Pattern{}
				if k, v, ok := strings.Cut(item, ":"); ok {
					if err := setPatternField(current, strings.TrimSpace(k), unquote(v)); err != nil {
						return nil, fmt.Errorf("line %d: %w", lineno+1, err)
					}
				} else if item != "" {
					return nil, fmt.Errorf("line %d: expected name: or regex: in redact pattern", lineno+1)
				}
			}
			continue
		}
		if section == "patterns" && current != nil {
			if k, v, ok := strings.Cut(trimmed, ":"); ok {
				if err := setPatternField(current, strings.TrimSpace(k), unquote(v)); err != nil {
					return nil, fmt.Errorf("line %d: %w", lineno+1, err)
				}
				continue
			}
		}
		return nil, fmt.Errorf("line %d: unexpected content %q", lineno+1, trimmed)
	}
	flush()
	return cfg, nil
}

func setPatternField(p *Pattern, key, value string) error {
	switch key {
	case "name":
		p.Name = value
	case "regex":
		p.Regex = value
	default:
		return fmt.Errorf("unknown redact pattern field %q", key)
	}
	return nil
}
