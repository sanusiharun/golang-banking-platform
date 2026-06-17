// Package template provides a notification template rendering engine.
// HTML format uses html/template (auto-escaping); all other formats use text/template.
package template

import (
	"bytes"
	"fmt"
	htemplate "html/template"
	"strings"
	ttemplate "text/template"
)

// Engine renders notification templates with dynamic variable substitution.
type Engine struct{}

// New creates a new Engine.
func New() *Engine { return &Engine{} }

// Render substitutes vars into body according to format.
// format must be "HTML" or "TEXT".
func (e *Engine) Render(format, body string, vars map[string]any) (string, error) {
	if vars == nil {
		vars = map[string]any{}
	}

	switch strings.ToUpper(format) {
	case "HTML":
		return renderHTML(body, vars)
	default:
		return renderText(body, vars)
	}
}

// RenderSubject renders a plain-text subject line (no HTML escaping).
func (e *Engine) RenderSubject(subject string, vars map[string]any) (string, error) {
	if subject == "" {
		return "", nil
	}
	if vars == nil {
		vars = map[string]any{}
	}
	return renderText(subject, vars)
}

func renderHTML(body string, vars map[string]any) (string, error) {
	t, err := htemplate.New("notification").Parse(body)
	if err != nil {
		return "", fmt.Errorf("template engine: parse html: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template engine: execute html: %w", err)
	}
	return buf.String(), nil
}

func renderText(body string, vars map[string]any) (string, error) {
	t, err := ttemplate.New("notification").Parse(body)
	if err != nil {
		return "", fmt.Errorf("template engine: parse text: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template engine: execute text: %w", err)
	}
	return buf.String(), nil
}
