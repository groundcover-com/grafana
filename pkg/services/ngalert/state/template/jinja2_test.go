package template

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExpandJinja2Header_BasicVariables(t *testing.T) {
	labels := map[string]string{
		"pod":       "web-1",
		"namespace": "prod",
	}

	result, err := ExpandJinja2Header(context.Background(), "Pod {{ alert.labels.pod }} in {{ alert.labels.namespace }}", labels)

	require.NoError(t, err)
	require.Equal(t, "Pod web-1 in prod", result)
}

func TestExpandJinja2Header_NoTemplateMarkers(t *testing.T) {
	labels := map[string]string{"pod": "web-1"}

	// Template without {{ markers should return unchanged
	result, err := ExpandJinja2Header(context.Background(), "Plain text without markers", labels)

	require.NoError(t, err)
	require.Equal(t, "Plain text without markers", result)
}

func TestExpandJinja2Header_InvalidSyntax(t *testing.T) {
	labels := map[string]string{"pod": "web-1"}

	// Invalid template syntax should return ExpandError
	_, err := ExpandJinja2Header(context.Background(), "{{ invalid syntax {% ", labels)

	require.Error(t, err)
	var expandErr ExpandError
	require.ErrorAs(t, err, &expandErr)
}

func TestExpandJinja2Header_MissingLabel(t *testing.T) {
	labels := map[string]string{"pod": "web-1"}

	// Missing label should render as empty string (pongo2 default)
	result, err := ExpandJinja2Header(context.Background(), "{{ alert.labels.nonexistent }}", labels)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

func TestExpandJinja2Header_EmptyLabels(t *testing.T) {
	labels := map[string]string{}

	// Empty labels should work without error
	result, err := ExpandJinja2Header(context.Background(), "{{ alert.labels.pod }}", labels)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

func TestExpandJinja2Header_NilLabels(t *testing.T) {
	// Nil labels should work without error
	result, err := ExpandJinja2Header(context.Background(), "{{ alert.labels.pod }}", nil)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

func TestExpandJinja2Header_SpecialCharactersInLabels(t *testing.T) {
	labels := map[string]string{
		"pod": "web-1 <test> & \"special\"",
	}

	// Pongo2 HTML-escapes output by default for safety
	result, err := ExpandJinja2Header(context.Background(), "Pod: {{ alert.labels.pod }}", labels)

	require.NoError(t, err)
	require.Equal(t, "Pod: web-1 &lt;test&gt; &amp; &quot;special&quot;", result)
}

func TestExpandJinja2Header_MultiplePlaceholders(t *testing.T) {
	labels := map[string]string{
		"pod":       "web-1",
		"namespace": "prod",
		"service":   "api",
		"cluster":   "us-east-1",
	}

	tmpl := "[{{ alert.labels.cluster }}] {{ alert.labels.namespace }}/{{ alert.labels.service }}: {{ alert.labels.pod }}"
	result, err := ExpandJinja2Header(context.Background(), tmpl, labels)

	require.NoError(t, err)
	require.Equal(t, "[us-east-1] prod/api: web-1", result)
}

func TestExpandJinja2Header_EmptyTemplate(t *testing.T) {
	labels := map[string]string{"pod": "web-1"}

	// Empty template should return empty string
	result, err := ExpandJinja2Header(context.Background(), "", labels)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

// Test the deprecated ExpandJinja2 function for backwards compatibility
func TestExpandJinja2_BackwardsCompatibility(t *testing.T) {
	data := Data{
		Labels: Labels{"pod": "web-1", "namespace": "prod"},
		Values: map[string]Value{},
		Value:  "95.5",
	}
	externalURL, _ := url.Parse("http://localhost:3000")

	// The deprecated function should work with the new alert.labels syntax
	result, err := ExpandJinja2(context.Background(), "test", "Pod {{ alert.labels.pod }} in {{ alert.labels.namespace }}", data, externalURL, time.Now())

	require.NoError(t, err)
	require.Equal(t, "Pod web-1 in prod", result)
}
