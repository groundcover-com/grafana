package state

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/services/ngalert/eval"
	ngModels "github.com/grafana/grafana/pkg/services/ngalert/models"
	"github.com/grafana/grafana/pkg/services/ngalert/state/template"
	"github.com/stretchr/testify/require"
)

func TestExpandAnnotationsAndLabels_Jinja2(t *testing.T) {
	logger := log.NewNopLogger()
	externalURL, _ := url.Parse("http://localhost:3000")

	alertRule := &ngModels.AlertRule{
		Title: "Test Alert",
		Annotations: map[string]string{
			ngModels.GCTemplateLanguageAnnotation: ngModels.TemplateLanguageJinja2,
			"summary":                             "Pod {{ labels.pod }} is down",
		},
		Labels: map[string]string{},
	}

	result := eval.Result{
		Instance: data.Labels{"pod": "web-1", "namespace": "prod"},
		Values:   map[string]eval.NumberValueCapture{},
	}

	lbs, annotations := expandAnnotationsAndLabels(context.Background(), logger, alertRule, result, data.Labels{}, externalURL)

	require.Equal(t, "Pod web-1 is down", annotations["summary"])
	require.Equal(t, "web-1", lbs["pod"])
}

func TestExpandAnnotationsAndLabels_LegacyTemplate(t *testing.T) {
	logger := log.NewNopLogger()
	externalURL, _ := url.Parse("http://localhost:3000")

	// No _gc_template_language annotation - should use legacy Go text/template
	alertRule := &ngModels.AlertRule{
		Title: "Test Alert",
		Annotations: map[string]string{
			"summary": "Pod {{ $labels.pod }} is down",
		},
		Labels: map[string]string{},
	}

	result := eval.Result{
		Instance: data.Labels{"pod": "web-1", "namespace": "prod"},
		Values:   map[string]eval.NumberValueCapture{},
	}

	lbs, annotations := expandAnnotationsAndLabels(context.Background(), logger, alertRule, result, data.Labels{}, externalURL)

	require.Equal(t, "Pod web-1 is down", annotations["summary"])
	require.Equal(t, "web-1", lbs["pod"])
}

func Test_expandJinja2(t *testing.T) {
	// Test successful expansion
	t.Run("successful expansion", func(t *testing.T) {
		logger := log.NewNopLogger()
		externalURL, _ := url.Parse("http://localhost:3000")
		data := template.Data{
			Labels: template.Labels{"pod": "web-1"},
		}
		original := map[string]string{
			"summary": "Pod {{ labels.pod }} is down",
		}

		result, err := expandJinja2(context.Background(), logger, "test", original, data, externalURL, time.Now())

		require.NoError(t, err)
		require.Equal(t, "Pod web-1 is down", result["summary"])
	})

	// Test error returns original template
	t.Run("error returns original template", func(t *testing.T) {
		logger := log.NewNopLogger()
		externalURL, _ := url.Parse("http://localhost:3000")
		data := template.Data{}
		original := map[string]string{
			"summary": "{{ invalid syntax }%}",
		}

		result, err := expandJinja2(context.Background(), logger, "test", original, data, externalURL, time.Now())

		require.Error(t, err)
		require.Equal(t, original["summary"], result["summary"]) // Original preserved on error
	})

	// Test empty map
	t.Run("empty map", func(t *testing.T) {
		logger := log.NewNopLogger()
		externalURL, _ := url.Parse("http://localhost:3000")
		data := template.Data{}

		result, err := expandJinja2(context.Background(), logger, "test", map[string]string{}, data, externalURL, time.Now())

		require.NoError(t, err)
		require.Empty(t, result)
	})
}

func TestExpandAnnotationsAndLabels_Jinja2WithValues(t *testing.T) {
	logger := log.NewNopLogger()
	externalURL, _ := url.Parse("http://localhost:3000")

	alertRule := &ngModels.AlertRule{
		Title: "Test Alert",
		Annotations: map[string]string{
			ngModels.GCTemplateLanguageAnnotation: ngModels.TemplateLanguageJinja2,
			"summary":                             "Value is {{ values.A.value }}",
		},
		Labels: map[string]string{},
	}

	val := 95.5
	result := eval.Result{
		Instance: data.Labels{"pod": "web-1"},
		Values: map[string]eval.NumberValueCapture{
			"A": {Value: &val, Labels: data.Labels{"instance": "server-1"}},
		},
	}

	lbs, annotations := expandAnnotationsAndLabels(context.Background(), logger, alertRule, result, data.Labels{}, externalURL)

	require.Contains(t, annotations["summary"], "95.5")
	require.Equal(t, "web-1", lbs["pod"])
}
