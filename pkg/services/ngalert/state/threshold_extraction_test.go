package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/expr"
	ngmodels "github.com/grafana/grafana/pkg/services/ngalert/models"
)

func makeThresholdModel(t *testing.T, evalType string, params []float64) json.RawMessage {
	t.Helper()
	m := map[string]any{
		"type":       "threshold",
		"expression": "A",
		"conditions": []map[string]any{
			{
				"evaluator": map[string]any{
					"type":   evalType,
					"params": params,
				},
			},
		},
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func TestExtractConfiguredThreshold(t *testing.T) {
	tests := []struct {
		name      string
		rule      *ngmodels.AlertRule
		wantVal   float64
		wantFound bool
	}{
		{
			name: "gt threshold",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: makeThresholdModel(t, "gt", []float64{90})},
				},
			},
			wantVal:   90,
			wantFound: true,
		},
		{
			name: "lt threshold",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: makeThresholdModel(t, "lt", []float64{50})},
				},
			},
			wantVal:   50,
			wantFound: true,
		},
		{
			name: "within_range uses first param",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: makeThresholdModel(t, "within_range", []float64{10, 100})},
				},
			},
			wantVal:   10,
			wantFound: true,
		},
		{
			name: "outside_range uses first param",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: makeThresholdModel(t, "outside_range", []float64{10, 100})},
				},
			},
			wantVal:   10,
			wantFound: true,
		},
		{
			name: "old datasource UID",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.OldDatasourceUID, Model: makeThresholdModel(t, "gt", []float64{42})},
				},
			},
			wantVal:   42,
			wantFound: true,
		},
		{
			name: "condition RefID not found",
			rule: &ngmodels.AlertRule{
				Condition: "C",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: makeThresholdModel(t, "gt", []float64{90})},
				},
			},
			wantVal:   0,
			wantFound: false,
		},
		{
			name: "condition is not expression datasource",
			rule: &ngmodels.AlertRule{
				Condition: "A",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus", Model: makeThresholdModel(t, "gt", []float64{90})},
				},
			},
			wantVal:   0,
			wantFound: false,
		},
		{
			name: "condition is math type not threshold",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: func() json.RawMessage {
						m := map[string]any{
							"type":       "math",
							"expression": "$A > 90",
						}
						b, _ := json.Marshal(m)
						return b
					}()},
				},
			},
			wantVal:   0,
			wantFound: false,
		},
		{
			name: "empty conditions array",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: func() json.RawMessage {
						m := map[string]any{
							"type":       "threshold",
							"expression": "A",
							"conditions": []map[string]any{},
						}
						b, _ := json.Marshal(m)
						return b
					}()},
				},
			},
			wantVal:   0,
			wantFound: false,
		},
		{
			name: "empty params",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "A", DatasourceUID: "prometheus"},
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: makeThresholdModel(t, "gt", []float64{})},
				},
			},
			wantVal:   0,
			wantFound: false,
		},
		{
			name: "malformed model JSON",
			rule: &ngmodels.AlertRule{
				Condition: "B",
				Data: []ngmodels.AlertQuery{
					{RefID: "B", DatasourceUID: expr.DatasourceUID, Model: json.RawMessage(`{invalid json}`)},
				},
			},
			wantVal:   0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := extractConfiguredThreshold(tt.rule)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantVal, val)
		})
	}
}
