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
