package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"agentledger/internal/config"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaultWhenNoFile(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RedactEnvFiles {
		t.Fatal("default RedactEnvFiles should be true")
	}
	if len(cfg.IgnorePaths) != 0 || len(cfg.RedactPatterns) != 0 {
		t.Fatalf("default should be empty, got %+v", cfg)
	}
}

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".agentledger.yaml", `ignore_paths:
  - "*.log"
  - "vendor/"
redact_patterns:
  - name: INTERNAL_TOKEN
    regex: "tok-[A-Za-z0-9]{16,}"
redact_env_files: false
`)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.IgnorePaths) != 2 || cfg.IgnorePaths[0] != "*.log" || cfg.IgnorePaths[1] != "vendor/" {
		t.Fatalf("ignore_paths = %v", cfg.IgnorePaths)
	}
	if len(cfg.RedactPatterns) != 1 || cfg.RedactPatterns[0].Name != "INTERNAL_TOKEN" {
		t.Fatalf("redact_patterns = %+v", cfg.RedactPatterns)
	}
	if cfg.RedactEnvFiles {
		t.Fatal("redact_env_files should be false")
	}
}

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".agentledger.json", `{"ignore_paths":["tmp/"],"redact_patterns":[{"name":"X","regex":"x+"}]}`)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.IgnorePaths) != 1 || cfg.IgnorePaths[0] != "tmp/" {
		t.Fatalf("ignore_paths = %v", cfg.IgnorePaths)
	}
	if !cfg.RedactEnvFiles {
		t.Fatal("redact_env_files should default to true when absent in JSON")
	}
}

func TestLoadMalformedFails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".agentledger.yaml", "not a mapping at all\n  - broken: [\n")
	if _, err := config.Load(dir); err == nil {
		t.Fatal("malformed config should fail loudly")
	}
}

func TestMatchIgnore(t *testing.T) {
	cfg := &config.Config{IgnorePaths: []string{"*.log", "vendor/", "build/output.txt", "docs/**/*.md"}}
	cases := map[string]bool{
		"a.log":                 true,
		"deep/dir/b.log":        true,
		"vendor/pkg/x.go":       true,
		"build/output.txt":      true,
		"docs/guide/chapter.md": true,
		"src/main.go":           false,
		"src/readme.md":         false,
		"vendor.go":             false,
	}
	for p, want := range cases {
		if got := cfg.MatchIgnore(p); got != want {
			t.Errorf("MatchIgnore(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestParseEnvValues(t *testing.T) {
	got := config.ParseEnvValues("# comment\nSHORT=abc\nSECRET=supersecretvalue123\nQUOTED=\"quotedsecret456\"\nexport EXPORTED=exportedsecret789\nEMPTY=\n")
	want := []string{"supersecretvalue123", "quotedsecret456", "exportedsecret789"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLoadEnvSecretsMissingFile(t *testing.T) {
	secrets, err := config.LoadEnvSecrets(t.TempDir())
	if err != nil || len(secrets) != 0 {
		t.Fatalf("missing .env: secrets=%v err=%v", secrets, err)
	}
}

func TestLoadRejectsUnknownKeysAndFlowStyle(t *testing.T) {
	for _, content := range []string{
		"ignor_paths:\n  - \"*.log\"\n",
		"ignore_paths: [\"*.log\"]\n",
		"redact_env_files: true\nsurprise_key: 1\n",
	} {
		dir := t.TempDir()
		writeFile(t, dir, ".agentledger.yaml", content)
		if _, err := config.Load(dir); err == nil {
			t.Fatalf("config %q should be rejected", content)
		}
	}
	dir := t.TempDir()
	writeFile(t, dir, ".agentledger.json", `{"bogus": true}`)
	if _, err := config.Load(dir); err == nil {
		t.Fatal("JSON with unknown key should be rejected")
	}
}
