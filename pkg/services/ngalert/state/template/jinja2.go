package template

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
)

// ExpandJinja2 expands a Jinja2 template with the given data.
//
// Template context available to templates:
//   - labels: map of alert labels (e.g., {{ labels.pod }})
//   - values: map of query results, each with "labels" and "value" fields
//     (e.g., {{ values.A.value }}, {{ values.A.labels.instance }})
//   - value: the single reduced value as a string (e.g., {{ value }})
//   - externalURL: the Grafana external URL if configured
//
// Note: Unlike Go templates which output "[no value]" for missing keys,
// Jinja2/pongo2 outputs an empty string for missing keys. This is standard
// Jinja2 behavior and is expected.
func ExpandJinja2(ctx context.Context, name, tmpl string, data Data, externalURL *url.URL, evaluatedAt time.Time) (string, error) {
	// Skip expansion if no template markers
	if !strings.Contains(tmpl, "{{") && !strings.Contains(tmpl, "{%") {
		return tmpl, nil
	}

	// Create pongo2 context with template data
	pongoCtx := pongo2.Context{
		"labels": data.Labels,
		"values": convertValuesToMap(data.Values),
		"value":  data.Value,
	}

	// Add external URL if available
	if externalURL != nil {
		pongoCtx["externalURL"] = externalURL.String()
	}

	// Parse and execute template
	tpl, err := pongo2.FromString(tmpl)
	if err != nil {
		return "", ExpandError{Tmpl: tmpl, Err: fmt.Errorf("parse error: %w", err)}
	}

	result, err := tpl.Execute(pongoCtx)
	if err != nil {
		return "", ExpandError{Tmpl: tmpl, Err: fmt.Errorf("execution error: %w", err)}
	}

	return result, nil
}

// convertValuesToMap converts Values to a map structure usable in Jinja2 templates.
// Uses lowercase keys ("labels", "value") to match Jinja2 naming conventions.
func convertValuesToMap(values map[string]Value) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range values {
		result[k] = map[string]interface{}{
			"labels": v.Labels,
			"value":  v.Value,
		}
	}
	return result
}
