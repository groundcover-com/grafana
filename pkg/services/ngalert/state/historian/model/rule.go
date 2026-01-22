package model

import (
	"context"
	"strconv"

	"github.com/grafana/grafana/pkg/expr"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/services/ngalert/models"
)

// RuleMeta is the metadata about a rule that is needed by state history.
type RuleMeta struct {
	ID           int64
	OrgID        int64
	UID          string
	Title        string
	Group        string
	NamespaceUID string
	DashboardUID string
	PanelID      int64
	Condition    string
	Query        string
}

func NewRuleMeta(r *models.AlertRule, logger log.Logger) RuleMeta {
	dashUID, ok := r.Annotations[models.DashboardUIDAnnotation]
	var panelID int64
	if ok {
		panelAnno := r.Annotations[models.PanelIDAnnotation]
		pid, err := strconv.ParseInt(panelAnno, 10, 64)
		if err != nil {
			logger.Error("Error parsing panelUID for alert annotation", "ruleID", r.ID, "dash", dashUID, "actual", panelAnno, "error", err)
			pid = 0
			dashUID = ""
		}
		panelID = pid
	}

	// groundcover
	// Find the first data source query (not an expression) to get the actual query
	// that was run against the datasource (e.g., PromQL, ClickHouse SQL, etc.)
	var query string
	for i := range r.Data {
		if !expr.IsDataSource(r.Data[i].DatasourceUID) {
			q, err := r.Data[i].GetQuery()
			if err == nil {
				query = q
				break
			}
			// On error, continue to try the next data source query
		}
	}

	return RuleMeta{
		ID:           r.ID,
		OrgID:        r.OrgID,
		UID:          r.UID,
		Title:        r.Title,
		Group:        r.RuleGroup,
		NamespaceUID: r.NamespaceUID,
		DashboardUID: dashUID,
		PanelID:      panelID,
		Condition:    r.Condition,
		Query:        query,
	}
}

func WithRuleData(ctx context.Context, rule RuleMeta) context.Context {
	return models.WithRuleKey(ctx, models.AlertRuleKey{OrgID: rule.OrgID, UID: rule.UID})
}
