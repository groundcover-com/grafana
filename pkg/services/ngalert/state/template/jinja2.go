package template

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
)

// ExpandJinja2Header expands a Jinja2-style header template with the given labels.
// Only simple variable interpolation is supported (e.g., {{ alert.labels.pod }}).
// No conditionals, loops, or filters are supported.
//
// Template context available:
//   - alert.labels: map of finalized alert labels (e.g., {{ alert.labels.pod }})
func ExpandJinja2Header(ctx context.Context, tmpl string, labels map[string]string) (string, error) {
	// Skip expansion if no template markers
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	// Create pongo2 context with alert.labels structure
	pongoCtx := pongo2.Context{
		"alert": map[string]interface{}{
			"labels": labels,
		},
	}

	// Parse and execute template
	tpl, err := pongo2.FromString(tmpl)
	if err != nil {
		return "", ExpandError{Tmpl: tmpl, Err: err}
	}

	result, err := tpl.Execute(pongoCtx)
	if err != nil {
		return "", ExpandError{Tmpl: tmpl, Err: err}
	}

	return result, nil
}

// ExpandJinja2 is kept for backwards compatibility but simplified.
// Deprecated: Use ExpandJinja2Header for header-only expansion.
func ExpandJinja2(ctx context.Context, name, tmpl string, data Data, externalURL *url.URL, evaluatedAt time.Time) (string, error) {
	// Convert Labels to map[string]string for the new function
	labels := make(map[string]string, len(data.Labels))
	for k, v := range data.Labels {
		labels[k] = v
	}
	return ExpandJinja2Header(ctx, tmpl, labels)
}
