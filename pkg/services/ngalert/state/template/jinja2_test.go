package template

import (
	"context"
	"testing"

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

	result, err := ExpandJinja2Header(context.Background(), "Plain text without markers", labels)

	require.NoError(t, err)
	require.Equal(t, "Plain text without markers", result)
}

func TestExpandJinja2Header_InvalidSyntax(t *testing.T) {
	labels := map[string]string{"pod": "web-1"}

	_, err := ExpandJinja2Header(context.Background(), "{{ invalid syntax {% ", labels)

	require.Error(t, err)
	var expandErr ExpandError
	require.ErrorAs(t, err, &expandErr)
}

func TestExpandJinja2Header_MissingLabel(t *testing.T) {
	labels := map[string]string{"pod": "web-1"}

	result, err := ExpandJinja2Header(context.Background(), "{{ alert.labels.nonexistent }}", labels)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

func TestExpandJinja2Header_EmptyLabels(t *testing.T) {
	labels := map[string]string{}

	result, err := ExpandJinja2Header(context.Background(), "{{ alert.labels.pod }}", labels)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

func TestExpandJinja2Header_NilLabels(t *testing.T) {
	result, err := ExpandJinja2Header(context.Background(), "{{ alert.labels.pod }}", nil)

	require.NoError(t, err)
	require.Equal(t, "", result)
}

func TestExpandJinja2Header_SpecialCharactersInLabels(t *testing.T) {
	labels := map[string]string{
		"pod": "web-1 <test> & \"special\"",
	}

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

	result, err := ExpandJinja2Header(context.Background(), "", labels)

	require.NoError(t, err)
	require.Equal(t, "", result)
}
