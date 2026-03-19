package state

import (
	"encoding/json"

	"github.com/grafana/grafana/pkg/expr"
	"github.com/grafana/grafana/pkg/services/ngalert/models"
)

const gcConfiguredThresholdKey = "_gc_configured_threshold"

// extractConfiguredThreshold parses the alert rule's condition query to
// extract the configured threshold value. Returns (value, true) if found.
func extractConfiguredThreshold(alertRule *models.AlertRule) (float64, bool) {
	// Find the condition query by matching alertRule.Condition RefID
	var conditionQuery *models.AlertQuery
	for i := range alertRule.Data {
		if alertRule.Data[i].RefID == alertRule.Condition {
			conditionQuery = &alertRule.Data[i]
			break
		}
	}
	if conditionQuery == nil {
		return 0, false
	}

	if !expr.IsDataSource(conditionQuery.DatasourceUID) {
		return 0, false
	}

	// Single unmarshal: embed type check and threshold config together.
	var config struct {
		Type string `json:"type"`
		expr.ThresholdCommandConfig
	}
	if err := json.Unmarshal(conditionQuery.Model, &config); err != nil {
		return 0, false
	}
	if config.Type != string(expr.QueryTypeThreshold) {
		return 0, false
	}
	if len(config.Conditions) == 0 || len(config.Conditions[0].Evaluator.Params) == 0 {
		return 0, false
	}

	return config.Conditions[0].Evaluator.Params[0], true
}
