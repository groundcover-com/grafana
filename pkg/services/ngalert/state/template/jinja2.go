package template

import (
	"strings"

	"github.com/flosch/pongo2/v6"
)

// SummaryContext holds all the context fields available for summary template expansion.
type SummaryContext struct {
	MonitorName string
	Severity    string
	Labels      map[string]string
	Value       float64
	Threshold   float64
	State       string
	Query       string
	Creator     string
}

// ExpandJinja2Summary expands a Jinja2-style summary template with the given context.
// Supports variable interpolation using {{ variable }} syntax.
// Available variables: monitor_name, severity, labels, value, threshold, state, query, creator
func ExpandJinja2Summary(tmpl string, ctx SummaryContext) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	pongoCtx := pongo2.Context{
		"monitor_name": ctx.MonitorName,
		"severity":     ctx.Severity,
		"labels":       ctx.Labels,
		"value":        ctx.Value,
		"threshold":    ctx.Threshold,
		"state":        ctx.State,
		"query":        ctx.Query,
		"creator":      ctx.Creator,
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
