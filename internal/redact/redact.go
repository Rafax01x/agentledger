package redact

import "regexp"

// A redaction rule: a pattern for a secret, and what replaces it.
// Order matters: Anthropic sk-ant- must run before OpenAI sk-, and
// the multiline private-key block must run before any pattern that
// could match inside it.
type rule struct {
	pattern     *regexp.Regexp
	replacement string
}

var rules = []rule{
	{
		// Private key blocks, including the BEGIN/END lines.
		pattern:     regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
		replacement: "<REDACTED:PRIVATE_KEY>",
	},
	{
		// JWTs: base64url header.payload.signature, header starting eyJ.
		pattern:     regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
		replacement: "<REDACTED:JWT>",
	},
	{
		// Bearer headers: the scheme is not secret, the token is.
		pattern:     regexp.MustCompile(`Bearer [A-Za-z0-9._~+/-]+={0,2}`),
		replacement: "Bearer <REDACTED:BEARER>",
	},
	{
		// Anthropic keys, before OpenAI (both start with sk-).
		pattern:     regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`),
		replacement: "<REDACTED:ANTHROPIC_KEY>",
	},
	{
		// GitHub tokens: ghp_, gho_, ghs_, ghu_. Minimum 10
		// characters after the prefix: real tokens are 36+,
		// but a shorter test/example token must still be
		// redacted rather than leaked.
		pattern:     regexp.MustCompile(`\bgh[pous]_[A-Za-z0-9]{10,}`),
		replacement: "<REDACTED:GITHUB_TOKEN>",
	},
	{
		// Slack tokens: xoxb-, xoxp-.
		pattern:     regexp.MustCompile(`\bxox[bp]-[A-Za-z0-9-]{10,}`),
		replacement: "<REDACTED:SLACK_TOKEN>",
	},
	{
		// AWS access key IDs: AKIA... and ASIA... (20 characters).
		pattern:     regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		replacement: "<REDACTED:AWS_KEY>",
	},
	{
		// OpenAI keys: sk- (and sk-proj-), after Anthropic.
		pattern:     regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_-]{20,}`),
		replacement: "<REDACTED:OPENAI_KEY>",
	},
}

// Redact replaces every recognized secret in s with its
// <REDACTED:TYPE> placeholder. Text that matches no pattern is
// returned unchanged.
func Redact(s string) string {
	for _, r := range rules {
		s = r.pattern.ReplaceAllString(s, r.replacement)
	}
	return s
}
