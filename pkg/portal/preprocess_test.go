package portal

import (
	"strings"
	"testing"
)

func TestReplaceTwoslashButtons(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic twoslash button",
			input:    `<button class="twoslash-hover" data-state="closed">createServer</button>`,
			expected: `<span>createServer</span>`,
		},
		{
			name:     "twoslash button with other classes",
			input:    `<button class="code twoslash-hover highlight" data-state="closed">require</button>`,
			expected: `<span>require</span>`,
		},
		{
			name:     "multiple buttons in code span",
			input:    `<span>const { <button class="twoslash-hover">createServer</button> } = <button class="twoslash-hover">require</button>('node:http')</span>`,
			expected: `<span>const { <span>createServer</span> } = <span>require</span>('node:http')</span>`,
		},
		{
			name:     "no twoslash buttons",
			input:    `<button class="normal-button">Click me</button>`,
			expected: `<button class="normal-button">Click me</button>`,
		},
		{
			name:     "twoslash with single quotes",
			input:    `<button class='twoslash-hover'>varName</button>`,
			expected: `<span>varName</span>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceTwoslashButtons(tt.input)
			if result != tt.expected {
				t.Errorf("replaceTwoslashButtons() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPreprocessHTMLPreservesCodeContent(t *testing.T) {
	// Simulate nodejs.org code block pattern
	input := `<pre><code><span style="color:#569CD6"><button data-state="closed" class="twoslash-hover">const</button></span> { <span style="color:#4FC1FF"><button data-state="closed" class="twoslash-hover">createServer</button></span> } = <span style="color:#4FC1FF"><button data-state="closed" class="twoslash-hover">require</button></span>(<span style="color:#CE9178">'node:http'</span>);</code></pre>`

	result := PreprocessHTML(input)

	// Should preserve const, createServer, require
	if !strings.Contains(result, "const") {
		t.Error("Expected 'const' to be preserved")
	}
	if !strings.Contains(result, "createServer") {
		t.Error("Expected 'createServer' to be preserved")
	}
	if !strings.Contains(result, "require") {
		t.Error("Expected 'require' to be preserved")
	}
	// Should not contain button tags
	if strings.Contains(result, "<button") {
		t.Error("Expected button tags to be replaced")
	}
}
