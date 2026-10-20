package api

import (
	"strings"
	"testing"
)

// TestDocumentTitle pins the OKF-aware title extraction rules:
// frontmatter title > first meaningful body line > first-line fallback.
func TestDocumentTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "okf frontmatter title",
			content: `---
container_tag: default
timestamp: "2026-10-15T13:22:07+02:00"
title: Memory 475ae19c-9070-a9e0-e237-b7879ac5e14d
type: memory
---
[role: user]
[Xairo] ja behebe alle fehler
[user:end]`,
			want: "Memory 475ae19c-9070-a9e0-e237-b7879ac5e14d",
		},
		{
			name: "frontmatter with single-quoted title",
			content: `---
type: memory
title: 'Single quoted title'
---
body`,
			want: "Single quoted title",
		},
		{
			name: "frontmatter without title key falls through to body",
			content: `---
container_tag: default
type: memory
---
first meaningful body line`,
			want: "first meaningful body line",
		},
		{
			name:    "plain text without frontmatter keeps first line",
			content: "Alice prefers Node.js for quick MCP bridges.",
			want:    "Alice prefers Node.js for quick MCP bridges.",
		},
		{
			name:    "leading blank lines then markdown heading",
			content: "\n\n# Heading\nbody text",
			want:    "Heading",
		},
		{
			name:    "markdown heading without frontmatter",
			content: "# My Title\nrest of the doc",
			want:    "My Title",
		},
		{
			name:    "empty content falls back without panic",
			content: "",
			want:    "(untitled)",
		},
		{
			name:    "only blank lines falls back",
			content: "\n\n   \n",
			want:    "(untitled)",
		},
		{
			name:    "long first body line is truncated at 80 runes",
			content: "---\ntype: memory\n---\n" + strings.Repeat("a", 90),
			want:    strings.Repeat("a", 80) + "…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := documentTitle(tt.content)
			if got != tt.want {
				t.Errorf("documentTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDocumentTitleWindowsNewlines ensures CRLF bodies parse the same as LF.
func TestDocumentTitleWindowsNewlines(t *testing.T) {
	content := "---\r\ntitle: CRLF Title\r\ntype: memory\r\n---\r\nbody"
	if got, want := documentTitle(content), "CRLF Title"; got != want {
		t.Errorf("documentTitle() = %q, want %q", got, want)
	}
}
