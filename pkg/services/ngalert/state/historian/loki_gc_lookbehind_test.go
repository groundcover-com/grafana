package historian

import (
	"testing"
	"time"

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

// The annotation is either the YAML the user submitted or a re-marshal of the model, so one
// field can arrive in any of three forms. Prometheus durations are the trap: time.ParseDuration
// rejects "1d" and "2w", and model.Duration.String() emits exactly those.
func TestParseGCDuration(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		// Prometheus form — d and w are unparseable as Go durations.
		{"1d", 24 * time.Hour, true},
		{"2w", 14 * 24 * time.Hour, true},
		{"5m", 5 * time.Minute, true},
		{"1h30m", 90 * time.Minute, true},
		// Go form, as emitted by monitors.Duration.MarshalYAML.
		{"24h0m0s", 24 * time.Hour, true},
		{"90s", 90 * time.Second, true},
		{"1m0s", time.Minute, true},
		// Legacy ClickHouse instantRollup form, full unit set, singular and plural.
		{"5 minutes", 5 * time.Minute, true},
		{"1 minute", time.Minute, true},
		{"2 hours", 2 * time.Hour, true},
		{"1 day", 24 * time.Hour, true},
		{"3 weeks", 21 * 24 * time.Hour, true},
		{"1 MONTH", 30 * 24 * time.Hour, true},
		{"1 quarter", 91 * 24 * time.Hour, true},
		{"1 year", 365 * 24 * time.Hour, true},
		// Nothing usable.
		{"", 0, false},
		{"0s", 0, false},
		{"-5m", 0, false},
		{"soon", 0, false},
		{"5 bananas", 0, false},
		{"5", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, ok := parseGCDuration(tt.value)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// instantRollup only widens the scanned window for gcQL queries, and entities is a timeless
// view with no time filter at all. A monitor of any other kind may carry a value the query
// builder ignores, and counting it would widen the link for a window never scanned.
func TestInstantRollupWidensWindow(t *testing.T) {
	for _, dataType := range []string{"logs", "traces", "events", "rum", "issues", "apm", "ingestion", "aws_cur", "logs_something"} {
		assert.True(t, instantRollupWidensWindow(dataType), dataType)
	}
	for _, dataType := range []string{"", "entities", "entities_live", "measurements", "nonsense"} {
		assert.False(t, instantRollupWidensWindow(dataType), dataType)
	}
}

// The gates above have to hold end to end, not just in isolation.
func TestExtractGCLookbehind_RobustToRealYAMLForms(t *testing.T) {
	t.Run("prometheus durations throughout", func(t *testing.T) {
		lookbehind, rollup := extractGCLookbehind(`
evaluationInterval:
  interval: 1d
  pendingFor: 1w
model:
  queries:
  - name: q
    datasourceType: prometheus
    rollup:
      function: avg
      time: 2d
`, log.NewNopLogger())
		// 2d rollup + 1w pendingFor + 1d interval
		assert.Equal(t, (10 * 24 * time.Hour).String(), lookbehind)
		assert.Equal(t, (48 * time.Hour).String(), rollup)
	})

	t.Run("legacy instantRollup on a gcql query", func(t *testing.T) {
		lookbehind, rollup := extractGCLookbehind(`
evaluationInterval:
  interval: 1m0s
model:
  queries:
  - name: q
    dataType: logs
    instantRollup: 5 minutes
`, log.NewNopLogger())
		assert.Equal(t, (6 * time.Minute).String(), lookbehind)
		assert.Equal(t, (5 * time.Minute).String(), rollup)
	})

	t.Run("instantRollup on an entities query does not widen", func(t *testing.T) {
		lookbehind, rollup := extractGCLookbehind(`
evaluationInterval:
  interval: 1m
model:
  queries:
  - name: q
    dataType: entities
    instantRollup: 30m
`, log.NewNopLogger())
		assert.Equal(t, (time.Minute).String(), lookbehind, "only the interval counts")
		assert.Equal(t, "", rollup)
	})

	t.Run("html-escaped annotation", func(t *testing.T) {
		lookbehind, _ := extractGCLookbehind(`
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
`, log.NewNopLogger())
		assert.Equal(t, (6 * time.Minute).String(), lookbehind)
	})

	t.Run("evaluationDelay is seconds and the widest query wins", func(t *testing.T) {
		lookbehind, _ := extractGCLookbehind(`
model:
  queries:
  - name: a
    datasourceType: prometheus
    evaluationDelay: 60
    rollup:
      function: avg
      time: 1m
  - name: b
    datasourceType: prometheus
    evaluationDelay: 172800
    rollup:
      function: avg
      time: 5m
`, log.NewNopLogger())
		// widest rollup 5m + widest delay 48h
		assert.Equal(t, (48*time.Hour + 5*time.Minute).String(), lookbehind)
	})
}
