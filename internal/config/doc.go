// Package config loads an optional project configuration file for
// agentledger from the repository root.
//
// Supported files, in precedence order:
//
//  1. .agentledger.yaml
//  2. .agentledger.json
//
// If neither exists, Load returns the default configuration. The JSON
// form uses standard encoding/json. The YAML form is a small subset
// sufficient for this file's fields: top-level keys, string lists,
// booleans, and a list of name/regex maps:
//
//	ignore_paths:
//	  - "*.log"
//	  - "vendor/"
//	redact_patterns:
//	  - name: INTERNAL_TOKEN
//	    regex: "tok-[A-Za-z0-9]{16,}"
//	redact_env_files: true
//
// Values may be single- or double-quoted. Unknown top-level keys
// are rejected: a typo in a key must fail loudly rather than
// silently disabling an ignore or redaction rule.
package config
