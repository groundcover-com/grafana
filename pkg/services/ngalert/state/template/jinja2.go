package template

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
)

func init() {
	// Register custom filters for label manipulation
	pongo2.RegisterFilter("filterLabels", filterLabelsFilter)
	pongo2.RegisterFilter("removeLabels", removeLabelsFilter)
	pongo2.RegisterFilter("filterLabelsRe", filterLabelsReFilter)
	pongo2.RegisterFilter("removeLabelsRe", removeLabelsReFilter)
}

// filterLabelsFilter keeps only labels matching the given string.
func filterLabelsFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	labels, ok := in.Interface().(Labels)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	match := param.String()
	filtered := filterLabelsFunc(labels, match)
	return pongo2.AsValue(filtered.String()), nil
}

// removeLabelsFilter removes labels matching the given string.
func removeLabelsFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	labels, ok := in.Interface().(Labels)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	match := param.String()
	filtered := removeLabelsFunc(labels, match)
	return pongo2.AsValue(filtered.String()), nil
}

// filterLabelsReFilter keeps only labels matching the given regex pattern.
func filterLabelsReFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	labels, ok := in.Interface().(Labels)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	pattern := param.String()
	// Validate regex before calling function that uses MustCompile
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, &pongo2.Error{
			Sender:    "filter:filterLabelsRe",
			OrigError: fmt.Errorf("invalid regex pattern: %w", err),
		}
	}
	filtered := filterLabelsReFunc(labels, pattern)
	return pongo2.AsValue(filtered.String()), nil
}

// removeLabelsReFilter removes labels matching the given regex pattern.
func removeLabelsReFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	labels, ok := in.Interface().(Labels)
	if !ok {
		return pongo2.AsValue(""), nil
	}
	pattern := param.String()
	// Validate regex before calling function that uses MustCompile
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, &pongo2.Error{
			Sender:    "filter:removeLabelsRe",
			OrigError: fmt.Errorf("invalid regex pattern: %w", err),
		}
	}
	filtered := removeLabelsReFunc(labels, pattern)
	return pongo2.AsValue(filtered.String()), nil
}

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
