package session_test

import (
	"testing"

	"agentledger/internal/session"
)

func TestParseMetrics(t *testing.T) {
	m := session.ParseMetrics([]string{
		`{"input_tokens": 1000, "output_tokens": 250}`,
		"total cost: $0.042",
		`{"input_tokens": 3000, "output_tokens": 400}`,
	})
	if m.InputTokens == nil || *m.InputTokens != 3000 {
		t.Fatalf("InputTokens = %+v, want 3000", m.InputTokens)
	}
	if m.OutputTokens == nil || *m.OutputTokens != 400 {
		t.Fatalf("OutputTokens = %+v, want 400", m.OutputTokens)
	}
	if m.CostUSD == nil || *m.CostUSD != 0.042 {
		t.Fatalf("CostUSD = %+v, want 0.042", m.CostUSD)
	}
}

func TestParseMetricsNone(t *testing.T) {
	m := session.ParseMetrics([]string{"hello world", "cost 3 tokens"})
	if m.InputTokens != nil || m.OutputTokens != nil || m.CostUSD != nil {
		t.Fatalf("no metrics expected, got %+v", m)
	}
}
