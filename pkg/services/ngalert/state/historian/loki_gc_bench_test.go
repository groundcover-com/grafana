package historian

import (
	"fmt"
	"testing"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/services/ngalert/eval"
	"github.com/grafana/grafana/pkg/services/ngalert/state"

	"github.com/grafana/grafana-plugin-sdk-go/data"
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

// One cold derivation: parse plus both projections, which is what a rule's first state pays.
func BenchmarkGCMonitorAnnotationCold(b *testing.B) {
	logger := log.NewNopLogger()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		memo := &gcMonitorAnnotationMemo{}
		_ = memo.get(benchMonitorYAML, logger)
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
			// A fresh memo per state is the pre-memo shape: one parse per series.
			memo := &gcMonitorAnnotationMemo{}
			_ = memo.get(benchMonitorYAML, logger)
		}
	}
}

// The derived-label emission runs per state, inside the loop the memo above exists to keep off
// the allocator. This measures that loop end to end for a rule with many series, which is where
// a per-state allocation actually costs something.
func BenchmarkStatesToStream_1000States_WithMonitorAnnotation(b *testing.B) {
	rule := createTestRule()
	logger := log.NewNopLogger()

	states := make([]state.StateTransition, 0, 1000)
	for i := 0; i < 1000; i++ {
		states = append(states, state.StateTransition{
			PreviousState: eval.Normal,
			State: &state.State{
				State:       eval.Alerting,
				Labels:      data.Labels{"alertname": "bench", "series": fmt.Sprintf("s%d", i)},
				Annotations: map[string]string{gcMonitorYamlAnnotation: benchMonitorYAML},
				Values:      map[string]float64{},
			},
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = StatesToStream(rule, states, nil, logger, false, nil)
	}
}
