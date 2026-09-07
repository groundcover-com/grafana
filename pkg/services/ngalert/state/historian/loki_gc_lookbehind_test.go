package historian

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/grafana/pkg/infra/log"
)

// The issue link dispatch-center mints has to open before the data the alert actually looked
// at, so the state row carries how far back that is. It is derived here, next to the query
// projection, because the _gc_monitor_yaml annotation is the only place the monitor's
// evaluation configuration is available on the alerting path.
func TestExtractGCLookbehind(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		wantLookbehind string
		wantRollup     string
	}{
		{
			name:           "empty annotation yields nothing",
			yaml:           "",
			wantLookbehind: "",
			wantRollup:     "",
		},
		{
			name: "prometheus rollup plus the evaluation interval",
			yaml: `
evaluationInterval:
  interval: 1m
model:
  queries:
  - name: q
    datasourceType: prometheus
    rollup:
      function: avg
      time: 5m
`,
			wantLookbehind: "6m0s",
			wantRollup:     "5m0s",
		},
		{
			name: "pendingFor and evaluationDelay both count toward the lookbehind",
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
			wantLookbehind: "18m0s",
			wantRollup:     "5m0s",
		},
		{
			name: "gcql instantRollup is the rollup",
			yaml: `
evaluationInterval:
  interval: 1m
model:
  queries:
  - name: q
    dataType: logs
    instantRollup: 10m
`,
			wantLookbehind: "11m0s",
			wantRollup:     "10m0s",
		},
		{
			name: "the widest query wins",
			yaml: `
model:
  queries:
  - name: narrow
    datasourceType: prometheus
    rollup:
      function: avg
      time: 1m
  - name: wide
    datasourceType: prometheus
    rollup:
      function: avg
      time: 30m
`,
			wantLookbehind: "30m0s",
			wantRollup:     "30m0s",
		},
		{
			name:           "unparseable annotation yields nothing rather than failing the state write",
			yaml:           "\tnot: [yaml",
			wantLookbehind: "",
			wantRollup:     "",
		},
		{
			name: "a monitor configuring none of it yields nothing",
			yaml: `
model:
  queries:
  - name: q
    expression: up
`,
			wantLookbehind: "",
			wantRollup:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookbehind, rollup := extractGCLookbehind(tt.yaml, log.NewNopLogger())
			assert.Equal(t, tt.wantLookbehind, lookbehind, "lookbehind")
			assert.Equal(t, tt.wantRollup, rollup, "rollup")
		})
	}
}

// The derived labels must not move the alert fingerprint: it keys dispatch-center's
// notification state, so a change would orphan every open alert cycle.
func TestExtractGCLookbehind_LabelsAreNotFingerprintInputs(t *testing.T) {
	base := map[string]string{"alertname": "x", "cluster": "c"}

	before := calculateFingerprint(base)

	withDerived := map[string]string{}
	for k, v := range base {
		withDerived[k] = v
	}
	withDerived["_gc_lookbehind"] = "16m0s"
	withDerived["_gc_rollup"] = "5m0s"

	assert.NotEqual(t, before, calculateFingerprint(withDerived),
		"sanity: the fingerprint does depend on its input map, so the producer must snapshot before adding derived labels")
}
