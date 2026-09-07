package historian

import (
	"testing"

	"github.com/grafana/grafana/pkg/infra/log"
)

// A realistic monitor: the shape UpdateAnnotations stores in _gc_monitor_yaml.
const benchMonitorYAML = `
title: Workload High API Error Rate Monitor
uuid: f3b3b3f0-5b7e-11eb-9e6f-42010a800002
severity: S2
measurementType: state
display:
  header: Workload High API Error Rate
  resourceHeaderLabels:
    - workload
  contextHeaderLabels:
    - cluster
    - namespace
  description: This Monitor fires when the workload's APIs are failing to handle a significant proportion of requests.
executionErrorState: OK
noDataState: NoData
labels:
  cluster: "{{ $values.api_error_rate_threshold.Labels.clusterId }}"
  namespace: "{{ $values.api_error_rate_threshold.Labels.namespace }}"
  workload: "{{ $values.api_error_rate_threshold.Labels.workload_name }}"
evaluationInterval:
  interval: 1m
  pendingFor: 10m
model:
  queries:
    - name: api_error_rate_query
      expression: |
        clamp((sum by (clusterId, namespace, workload_name, env) (increase(groundcover_workload_total_counter{role="server", status_code="error"}[1m])) / sum by (clusterId, namespace, workload_name, env) (increase(groundcover_workload_total_counter{role="server"}[1m]))) * 100, 0, 100)
      datasourceType: prometheus
      queryType: instant
      rollup:
        function: avg
        time: 5m
  reducers:
    - name: api_error_rate_mean
      inputName: api_error_rate_query
      type: mean
  thresholds:
    - name: api_error_rate_threshold
      inputName: api_error_rate_mean
      operator: gt
      values:
        - 5
`

func BenchmarkExtractGCMonitorTiming(b *testing.B) {
	logger := log.NewNopLogger()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractGCMonitorTiming(benchMonitorYAML, logger)
	}
}

func BenchmarkExtractGCQuery(b *testing.B) {
	logger := log.NewNopLogger()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractGCQuery(benchMonitorYAML, logger)
	}
}

// A rule's annotation is identical for every state in a batch, so the batch should cost one
// parse rather than one per series.
func BenchmarkGCMonitorAnnotationMemo_1000States(b *testing.B) {
	logger := log.NewNopLogger()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var memo gcMonitorAnnotationMemo
		for state := 0; state < 1000; state++ {
			_ = memo.get(benchMonitorYAML, logger)
		}
	}
}

// What the same batch costs parsing per state, as it did before.
func BenchmarkGCMonitorAnnotationUnmemoized_1000States(b *testing.B) {
	logger := log.NewNopLogger()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for state := 0; state < 1000; state++ {
			_ = extractGCQuery(benchMonitorYAML, logger)
			_ = extractGCMonitorTiming(benchMonitorYAML, logger)
		}
	}
}
