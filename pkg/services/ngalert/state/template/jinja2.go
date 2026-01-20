package template

import (
	"context"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// ExpandJinja2Header expands a Jinja2-style header template with the given labels.
// Supports {{ alert.labels.X }} syntax for variable interpolation.
func ExpandJinja2Header(ctx context.Context, tmpl string, labels map[string]string) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	pongoCtx := pongo2.Context{
		"alert": map[string]interface{}{
			"labels": labels,
		},
	}

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
