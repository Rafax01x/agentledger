package redact_test

import (
	"os"
	"strings"
	"testing"

	"agentledger/internal/config"
	"agentledger/internal/redact"
)

// configLoad is a thin wrapper so the config package stays an
// implementation detail of this test file's helper.
func configLoad(dir string) (*config.Config, error) {
	return config.Load(dir)
}

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

func TestRedactorCustomPatternAndEnvSecrets(t *testing.T) {
	r, err := redact.NewRedactor(
		[]redact.CustomPattern{{Name: "INTERNAL_TOKEN", Regex: `tok-[A-Za-z0-9]{16,}`}},
		[]string{"supersecretvalue123", "tiny"},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := "token tok-abcdefghijklmnop and secret supersecretvalue123 and tiny word"
	got := r.Redact(input)
	if !strings.Contains(got, "<REDACTED:INTERNAL_TOKEN>") {
		t.Fatalf("custom pattern not applied: %q", got)
	}
	if strings.Contains(got, "supersecretvalue123") {
		t.Fatalf("env secret leaked: %q", got)
	}
	if !strings.Contains(got, "<REDACTED:ENV_SECRET>") {
		t.Fatalf("env secret placeholder missing: %q", got)
	}
	if !strings.Contains(got, "tiny") {
		t.Fatalf("short values must not be masked: %q", got)
	}
}

func TestRedactorInvalidRegexFails(t *testing.T) {
	if _, err := redact.NewRedactor([]redact.CustomPattern{{Name: "BAD", Regex: "["}}, nil); err == nil {
		t.Fatal("invalid custom regex should fail")
	}
}

func TestNewFromConfigReadsEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("API_KEY=envsecretvalue999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/.agentledger.yaml", []byte("redact_patterns:\n  - name: MINE\n    regex: \"mine-[0-9]+\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := configLoad(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := redact.NewFromConfig(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Redact("leak envsecretvalue999 and mine-12345 here")
	if strings.Contains(got, "envsecretvalue999") || strings.Contains(got, "mine-12345") {
		t.Fatalf("config redaction leaked: %q", got)
	}
}
