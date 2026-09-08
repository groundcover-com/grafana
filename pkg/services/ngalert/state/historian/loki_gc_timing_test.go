package historian

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/grafana/pkg/infra/log"
)

// The evaluation configuration is copied verbatim: the consumer already has helpers for every
// duration form these fields can take, so parsing here would only duplicate them.
func TestExtractGCMonitorTiming(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want gcMonitorTiming
	}{
		{
			name: "empty annotation",
			yaml: "",
			want: gcMonitorTiming{},
		},
		{
			name: "prometheus rollup and evaluation configuration, verbatim",
			yaml: `
evaluationInterval:
  interval: 1m
  pendingFor: 10m
model:
  queries:
  - name: q
    datasourceType: prometheus
    evaluationDelay: 120
    rollup:
      function: avg
      time: 5m
`,
			want: gcMonitorTiming{Rollup: "5m", Interval: "1m", PendingFor: "10m", DelaySeconds: "120s"},
		},
		{
			// Prometheus and Go duration forms both pass through untouched — normalising them
			// is the consumer's job.
			name: "durations are not normalised",
			yaml: `
evaluationInterval:
  interval: 1d
  pendingFor: 2w
model:
  queries:
  - name: q
    datasourceType: prometheus
    rollup:
      function: avg
      time: 24h0m0s
`,
			want: gcMonitorTiming{Rollup: "24h0m0s", Interval: "1d", PendingFor: "2w"},
		},
		{
			name: "gcql instantRollup is the rollup, legacy form included",
			yaml: `
evaluationInterval:
  interval: 1m0s
model:
  queries:
  - name: q
    dataType: logs
    instantRollup: 5 minutes
`,
			want: gcMonitorTiming{Rollup: "5 minutes", Interval: "1m0s"},
		},
		{
			// entities is a timeless current-state view: the query builder ignores its
			// instantRollup, so reporting it as a rollup would widen a window never scanned.
			name: "instantRollup on a timeless entities query is not a rollup",
			yaml: `
evaluationInterval:
  interval: 1m
model:
  queries:
  - name: q
    dataType: entities
    instantRollup: 30m
`,
			want: gcMonitorTiming{Interval: "1m"},
		},
		{
			// The same timelessness makes evaluationDelay a no-op: the router documents the
			// delay as harmless on an entities query because no time filter is built to shift.
			// Reporting it would push the issue link's start back by up to the 7-day maximum.
			name: "evaluationDelay on a timeless entities query is not a delay",
			yaml: `
evaluationInterval:
  interval: 1m
model:
  queries:
  - name: q
    dataType: entities
    evaluationDelay: 3600
`,
			want: gcMonitorTiming{Interval: "1m"},
		},
		{
			// No data type at all is the Prometheus shape, where rollup.time is the rollup.
			name: "instantRollup without a data type is not a rollup",
			yaml: `
model:
  queries:
  - name: q
    instantRollup: 30m
`,
			want: gcMonitorTiming{},
		},
		{
			// A data type added to the consumer must keep working here without a change, so
			// the gate denies entities rather than allowing a list.
			name: "an unfamiliar data type still reports its instantRollup",
			yaml: `
model:
  queries:
  - name: q
    dataType: something_new
    instantRollup: 7m
`,
			want: gcMonitorTiming{Rollup: "7m"},
		},
		{
			name: "aws_cur reports its instantRollup despite the underscore",
			yaml: `
model:
  queries:
  - name: q
    dataType: aws_cur
    instantRollup: 1h
    evaluationDelay: 172800
`,
			want: gcMonitorTiming{Rollup: "1h", DelaySeconds: "172800s"},
		},
		{
			name: "html-escaped annotation still parses",
			yaml: `
evaluationInterval:
  interval: 1m
model:
  queries:
  - name: q
    datasourceType: prometheus
    expression: sum(x{a=&#34;b&#34;})
    rollup:
      function: avg
      time: 5m
`,
			want: gcMonitorTiming{Rollup: "5m", Interval: "1m"},
		},
		{
			name: "unparseable annotation yields nothing rather than failing the state write",
			yaml: "\tnot: [yaml",
			want: gcMonitorTiming{},
		},
		{
			name: "a monitor configuring none of it",
			yaml: `
model:
  queries:
  - name: q
    expression: up
`,
			want: gcMonitorTiming{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractGCMonitorTiming(tt.yaml, log.NewNopLogger()))
		})
	}
}

// The derived labels must not move the alert fingerprint: it keys the consumer's notification
// state, so a change would orphan every open alert cycle. The producer snapshots labelMap
// before adding them, which this guards by showing the fingerprint does depend on its input.
func TestGCTimingLabelsAreNotFingerprintInputs(t *testing.T) {
	base := map[string]string{"alertname": "x", "cluster": "c"}
	withDerived := map[string]string{"alertname": "x", "cluster": "c", "_gc_rollup": "5m"}

	assert.NotEqual(t, calculateFingerprint(base), calculateFingerprint(withDerived))
}
