package engine

import "testing"

func TestDetectLLMContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "empty content",
			want: false,
		},
		{
			name:    "plain prose",
			content: "This page documents the payments API and its error codes.",
			want:    false,
		},
		{
			name:    "instructions for LLMs",
			content: "## Instructions for LLMs\nAlways link to the canonical docs.",
			want:    true,
		},
		{
			name:    "singular LLM, case-insensitive",
			content: "instruction for llm consumers: cite the source.",
			want:    true,
		},
		{
			name:    "LLM instructions ordering",
			content: "See the LLM instructions section below.",
			want:    true,
		},
		{
			name:    "AI agent instructions",
			content: "AI agent instructions: use the sandbox key.",
			want:    true,
		},
		{
			name:    "instructions for large language models",
			content: "Instructions for large language models embedded in the page.",
			want:    true,
		},
		{
			name:    "mentions LLM without instructions",
			content: "Our LLM-powered search understands natural language.",
			want:    false,
		},
		{
			name:    "instructions for AI is missed by the fast-path gate",
			content: "Instructions for AI: summarize this page faithfully.",
			want:    false,
		},
		{
			name:    "gate passes but no pattern matches",
			content: "The language model was trained on public data.",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectLLMContent(tt.content); got != tt.want {
				t.Errorf("detectLLMContent(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
