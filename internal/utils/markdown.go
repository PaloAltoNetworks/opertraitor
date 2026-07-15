package utils

import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// safePolicy is a sanitizer for attacker-controlled markdown (operator descriptions, LLM output).
func safePolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Force rel="nofollow noreferrer" on every rendered link so untrusted
	// content cannot influence referrer headers or SEO from our dashboard.
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	return p
}

// RenderSafeHTML takes raw markdown, converts it to HTML, sanitizes it,
// and returns it as a template.HTML type so Go templates render it properly.
func RenderSafeHTML(markdown string) template.HTML {
	if markdown == "" {
		return template.HTML("")
	}

	// Convert Markdown to HTML
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM), // GitHub Flavored Markdown (tables, etc)
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		// On conversion failure, sanitize the raw input as a safety net.
		return template.HTML(string(safePolicy().SanitizeBytes([]byte(markdown))))
	}

	return template.HTML(string(safePolicy().SanitizeBytes(buf.Bytes())))
}
