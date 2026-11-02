package redact_test

import (
	"strings"
	"testing"

	"agentledger/internal/redact"
)

func TestRedactEachType(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"aws",
			"key AKIAIOSFODNN7EXAMPLE here",
			"key <REDACTED:AWS_KEY> here",
		},
		{
			"aws temp",
			"key ASIAIOSFODNN7EXAMPLE here",
			"key <REDACTED:AWS_KEY> here",
		},
		{
			"github personal",
			"token ghp_abcdefghijklmnopqrstuvwxyz0123456789 end",
			"token <REDACTED:GITHUB_TOKEN> end",
		},
		{
			"github oauth",
			"token gho_abcdefghijklmnopqrstuvwxyz0123456789 end",
			"token <REDACTED:GITHUB_TOKEN> end",
		},
		{
			"github short example token",
			"token ghp_abcdef1234567890 end",
			"token <REDACTED:GITHUB_TOKEN> end",
		},
		{
			"openai",
			"key sk-abcdefghijklmnopqrstuvwxyz0123456789abcdef end",
			"key <REDACTED:OPENAI_KEY> end",
		},
		{
			"openai proj",
			"key sk-proj-abcdefghijklmnopqrstuvwxyz0123456789abcdef end",
			"key <REDACTED:OPENAI_KEY> end",
		},
		{
			"anthropic not openai",
			"key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789 end",
			"key <REDACTED:ANTHROPIC_KEY> end",
		},
		{
			"slack bot",
			"token xoxb-abcdefghij-klmnopqrstuv end",
			"token <REDACTED:SLACK_TOKEN> end",
		},
		{
			"slack user",
			"token xoxp-abcdefghij-klmnopqrstuv end",
			"token <REDACTED:SLACK_TOKEN> end",
		},
		{
			"bearer keeps scheme",
			"Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789._- end",
			"Authorization: Bearer <REDACTED:BEARER> end",
		},
		{
			"jwt",
			"token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3AAA end",
			"token <REDACTED:JWT> end",
		},
		{
			"private key block",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA7\n-----END RSA PRIVATE KEY-----",
			"<REDACTED:PRIVATE_KEY>",
		},
		{
			"multiple in one line",
			"aws AKIAIOSFODNN7EXAMPLE and ghp_abcdefghijklmnopqrstuvwxyz0123456789",
			"aws <REDACTED:AWS_KEY> and <REDACTED:GITHUB_TOKEN>",
		},
		{
			"plain text untouched",
			"hello world, no secrets here, skill-level sk- is too short",
			"hello world, no secrets here, skill-level sk- is too short",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redact.Redact(tc.input); got != tc.want {
				t.Fatalf("Redact(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRedactDoesNotLeakOriginal(t *testing.T) {
	secret := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	out := redact.Redact("leaked " + secret)
	if strings.Contains(out, secret) {
		t.Fatalf("original secret still present: %q", out)
	}
}
