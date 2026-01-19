package template

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExpandJinja2_BasicVariables(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1", "namespace": "prod"},
		Values: map[string]Value{},
		Value:  "95.5",
	}
	externalURL, _ := url.Parse("http://localhost:3000")

	result, err := ExpandJinja2(context.Background(), "test", "Pod {{ labels.pod }} in {{ labels.namespace }}", data, externalURL, time.Now())

	require.NoError(t, err)
	require.Equal(t, "Pod web-1 in prod", result)
}

func TestExpandJinja2_NoTemplateMarkers(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1"},
		Values: map[string]Value{},
		Value:  "95.5",
	}

	// Template without {{ or {% markers should return unchanged
	result, err := ExpandJinja2(context.Background(), "test", "Plain text without markers", data, nil, time.Now())

	require.NoError(t, err)
	require.Equal(t, "Plain text without markers", result)
}

func TestExpandJinja2_InvalidSyntax(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1"},
		Values: map[string]Value{},
		Value:  "95.5",
	}

	// Invalid template syntax should return ExpandError
	_, err := ExpandJinja2(context.Background(), "test", "{{ invalid syntax {% ", data, nil, time.Now())

	require.Error(t, err)
	var expandErr ExpandError
	require.ErrorAs(t, err, &expandErr)
}

func TestExpandJinja2_ValuesMapAccess(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1"},
		Values: map[string]Value{
			"A": {
				Labels: Labels{"instance": "localhost:9090", "job": "prometheus"},
				Value:  123.45,
			},
			"B": {
				Labels: Labels{"instance": "localhost:9091"},
				Value:  67.89,
			},
		},
		Value: "95.5",
	}

	// Test accessing values map with lowercase keys
	result, err := ExpandJinja2(context.Background(), "test", "Query A value: {{ values.A.value }}, instance: {{ values.A.labels.instance }}", data, nil, time.Now())

	require.NoError(t, err)
	// Float values are rendered with full precision by pongo2
	require.Equal(t, "Query A value: 123.450000, instance: localhost:9090", result)
}

func TestExpandJinja2_Conditionals(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		data     Data
		expected string
	}{
		{
			name:     "if equals",
			tmpl:     "{% if labels.severity == 'critical' %}CRITICAL{% else %}normal{% endif %}",
			data:     Data{Labels: Labels{"severity": "critical"}},
			expected: "CRITICAL",
		},
		{
			name:     "if not equals",
			tmpl:     "{% if labels.severity != 'critical' %}not critical{% endif %}",
			data:     Data{Labels: Labels{"severity": "warning"}},
			expected: "not critical",
		},
		{
			name:     "if greater than",
			tmpl:     "{% if values.A.value > 90 %}HIGH{% else %}LOW{% endif %}",
			data:     Data{Values: map[string]Value{"A": {Value: 95.5}}},
			expected: "HIGH",
		},
		{
			name:     "if less than",
			tmpl:     "{% if values.A.value < 50 %}LOW{% else %}HIGH{% endif %}",
			data:     Data{Values: map[string]Value{"A": {Value: 30.0}}},
			expected: "LOW",
		},
	}

	externalURL, _ := url.Parse("http://localhost:3000")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandJinja2(context.Background(), "test", tt.tmpl, tt.data, externalURL, time.Now())
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandJinja2_Loops(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1", "namespace": "prod", "service": "api"},
	}
	tmpl := "{% for key, val in labels %}{{ key }}={{ val }}{% if not forloop.Last %}, {% endif %}{% endfor %}"
	externalURL, _ := url.Parse("http://localhost:3000")

	result, err := ExpandJinja2(context.Background(), "test", tmpl, data, externalURL, time.Now())

	require.NoError(t, err)
	// Labels iteration order may vary, just check it contains expected parts
	require.Contains(t, result, "pod=web-1")
	require.Contains(t, result, "namespace=prod")
	require.Contains(t, result, "service=api")
}

func TestExpandJinja2_SetVariable(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1"},
		Values: map[string]Value{"A": {Value: 95.5}},
	}
	// Pongo2 uses {% if %}{% set %}{% endif %} pattern instead of inline conditionals
	tmpl := "{% if values.A.value > 90 %}{% set status = 'critical' %}{% else %}{% set status = 'normal' %}{% endif %}Status: {{ status }}"
	externalURL, _ := url.Parse("http://localhost:3000")

	result, err := ExpandJinja2(context.Background(), "test", tmpl, data, externalURL, time.Now())

	require.NoError(t, err)
	require.Equal(t, "Status: critical", result)
}

func TestExpandJinja2_MathOperations(t *testing.T) {
	data := Data{
		Values: map[string]Value{"A": {Value: 100.0}},
	}
	tmpl := "Value: {{ values.A.value / 2 }}"
	externalURL, _ := url.Parse("http://localhost:3000")

	result, err := ExpandJinja2(context.Background(), "test", tmpl, data, externalURL, time.Now())

	require.NoError(t, err)
	// Pongo2 renders floats with full precision
	require.Equal(t, "Value: 50.000000", result)
}

func TestExpandJinja2_StringFunctions(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-server-1"},
	}
	tmpl := "{{ labels.pod|upper }}"
	externalURL, _ := url.Parse("http://localhost:3000")

	result, err := ExpandJinja2(context.Background(), "test", tmpl, data, externalURL, time.Now())

	require.NoError(t, err)
	require.Equal(t, "WEB-SERVER-1", result)
}

func TestExpandJinja2_CustomFilters(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		data     Data
		expected string
	}{
		{
			name:     "filterLabels",
			tmpl:     "{{ labels|filterLabels:\"pod\" }}",
			data:     Data{Labels: Labels{"pod": "web-1", "namespace": "prod"}},
			expected: "pod=web-1",
		},
		{
			name:     "removeLabels",
			tmpl:     "{{ labels|removeLabels:\"namespace\" }}",
			data:     Data{Labels: Labels{"pod": "web-1", "namespace": "prod"}},
			expected: "pod=web-1",
		},
		{
			name:     "filterLabelsRe",
			tmpl:     "{{ labels|filterLabelsRe:\"pod.*\" }}",
			data:     Data{Labels: Labels{"pod": "web-1", "pod_name": "api", "namespace": "prod"}},
			expected: "pod=web-1, pod_name=api",
		},
		{
			name:     "removeLabelsRe",
			tmpl:     "{{ labels|removeLabelsRe:\"pod.*\" }}",
			data:     Data{Labels: Labels{"pod": "web-1", "pod_name": "api", "namespace": "prod"}},
			expected: "namespace=prod",
		},
	}

	externalURL, _ := url.Parse("http://localhost:3000")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandJinja2(context.Background(), "test", tt.tmpl, tt.data, externalURL, time.Now())
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandJinja2_InvalidRegexPatterns(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
	}{
		{
			name: "filterLabelsRe with invalid regex",
			tmpl: "{{ labels|filterLabelsRe:\"[invalid\" }}",
		},
		{
			name: "removeLabelsRe with invalid regex",
			tmpl: "{{ labels|removeLabelsRe:\"[invalid\" }}",
		},
	}

	data := Data{Labels: Labels{"pod": "web-1", "namespace": "prod"}}
	externalURL, _ := url.Parse("http://localhost:3000")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should return error, not panic
			_, err := ExpandJinja2(context.Background(), "test", tt.tmpl, data, externalURL, time.Now())
			require.Error(t, err)
			var expandErr ExpandError
			require.ErrorAs(t, err, &expandErr)
			require.Contains(t, expandErr.Error(), "invalid regex pattern")
		})
	}
}

func TestExpandJinja2_EmptyLabels(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		expected string
	}{
		{
			name:     "filterLabels with empty labels",
			tmpl:     "{{ labels|filterLabels:\"pod\" }}",
			expected: "",
		},
		{
			name:     "removeLabels with empty labels",
			tmpl:     "{{ labels|removeLabels:\"pod\" }}",
			expected: "",
		},
		{
			name:     "filterLabelsRe with empty labels",
			tmpl:     "{{ labels|filterLabelsRe:\"pod.*\" }}",
			expected: "",
		},
		{
			name:     "removeLabelsRe with empty labels",
			tmpl:     "{{ labels|removeLabelsRe:\"pod.*\" }}",
			expected: "",
		},
	}

	data := Data{Labels: Labels{}}
	externalURL, _ := url.Parse("http://localhost:3000")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandJinja2(context.Background(), "test", tt.tmpl, data, externalURL, time.Now())
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}
