package session

import (
	"regexp"
	"strconv"
)

// Metrics are optional token/cost figures parsed from an agent's
// output streams. Every field is nil when the agent emitted no
// recognizable metric line, so "not reported" is never confused
// with zero.
type Metrics struct {
	InputTokens  *int64
	OutputTokens *int64
	CostUSD      *float64
}

var (
	inputTokensRe  = regexp.MustCompile(`(?i)(?:input_tokens|prompt_tokens)\D{0,5}(\d+)`)
	outputTokensRe = regexp.MustCompile(`(?i)(?:output_tokens|completion_tokens)\D{0,5}(\d+)`)
	// Cost lines look like "total cost: $0.042" or
	// "cost_usd": 0.042. A decimal point is required so a bare
	// count ("cost 3 tokens") is never mistaken for dollars.
	costRe = regexp.MustCompile(`(?i)cost\D{0,10}\$?\s*(\d+\.\d+)`)
)

// ParseMetrics scans output lines for token/cost summaries emitted
// by common agents (Claude Code, Aider, and tools that print
// input_tokens/output_tokens/cost fields). The last occurrence of
// each metric wins: summaries are normally printed at the end of a
// run, and an earlier partial figure must not override them.
func ParseMetrics(lines []string) Metrics {
	var m Metrics
	for _, line := range lines {
		if mm := inputTokensRe.FindStringSubmatch(line); mm != nil {
			if n, err := strconv.ParseInt(mm[1], 10, 64); err == nil {
				m.InputTokens = &n
			}
		}
		if mm := outputTokensRe.FindStringSubmatch(line); mm != nil {
			if n, err := strconv.ParseInt(mm[1], 10, 64); err == nil {
				m.OutputTokens = &n
			}
		}
		if mm := costRe.FindStringSubmatch(line); mm != nil {
			if f, err := strconv.ParseFloat(mm[1], 64); err == nil {
				m.CostUSD = &f
			}
		}
	}
	return m
}
