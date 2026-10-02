package historian

// groundcover: state history enrichment for monitors-manager. Kept out of loki.go so upstream
// bumps only conflict on the few call sites (StatesToStream, NewRemoteLokiBackend, ngalert.go).

import (
	"encoding/json"
	"fmt"
	"html"
	"maps"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"gopkg.in/yaml.v3"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/tracing"
	"github.com/grafana/grafana/pkg/services/ngalert/client"
	"github.com/grafana/grafana/pkg/services/ngalert/eval"
	"github.com/grafana/grafana/pkg/services/ngalert/metrics"
	"github.com/grafana/grafana/pkg/services/ngalert/notifier"
	"github.com/grafana/grafana/pkg/services/ngalert/state"
	history_model "github.com/grafana/grafana/pkg/services/ngalert/state/historian/model"
	"github.com/grafana/grafana/pkg/services/ngalert/state/template"
)

const (
	MonitorNameLabel = "monitor_name"
	// Error annotation name.
	errAnnotationName       = "Error"
	gcMonitorYamlAnnotation = "_gc_monitor_yaml"

	// gcIssueHeaderAnnotation holds the Jinja2 template for the alert issue summary.
	gcIssueHeaderAnnotation = "_gc_issue_header"
	// gcSeverityLabel is the label key for alert severity.
	gcSeverityLabel = "_gc_severity"
	// gcCreatorAnnotation is the annotation key for the alert creator.
	gcCreatorAnnotation = "_gc_creator"
	// gcQueryLabel is the label key for the alert query.
	gcQueryLabel = "_gc_query"
	// gcThresholdInputQueryKey is the values map key for the threshold input query value.
	gcThresholdInputQueryKey = "threshold_input_query"
	// gcThreshold1Key is the values map key for the threshold_1 value.
	gcThreshold1Key = "threshold_1"
)

var annotationsToDelete = map[string]struct{}{
	gcMonitorYamlAnnotation: {},
}

// MuteChecker is an interface for checking if an alert is muted based on its labels
type MuteChecker interface {
	GetSilenceIds(orgID int64, labels data.Labels) ([]string, error)
}

// AttachMuteChecker wires silence lookup into every Loki backend of a configured historian.
func AttachMuteChecker(h any, moa *notifier.MultiOrgAlertmanager) {
	if moa == nil {
		return
	}
	switch b := h.(type) {
	case *RemoteLokiBackend:
		b.muteChecker = NewMultiOrgAlertmanagerMuteChecker(moa)
	case *MultipleBackend:
		AttachMuteChecker(b.primary, moa)
		for _, s := range b.secondaries {
			AttachMuteChecker(s, moa)
		}
	}
}

// gcLokiEntry builds the state history entry for one transition, including groundcover fields.
func gcLokiEntry(rule history_model.RuleMeta, state state.StateTransition, logger log.Logger, muteChecker MuteChecker) LokiEntry {
	// Add monitor name label directly to the state labels
	if state.Labels == nil {
		state.Labels = data.Labels{}
	}
	state.Labels[MonitorNameLabel] = rule.Title
	sanitizedLabels := removePrivateLabels(state.Labels)
	var errMsg string
	switch {
	case state.State.State == eval.Error:
		// state.Error is sometimes nil even in an error state; fall back to the annotation.
		if state.Error != nil {
			errMsg = state.Error.Error()
		} else {
			errMsg = state.Annotations[errAnnotationName]
		}
		state.State.Values = map[string]float64{}
	case state.Annotations[errAnnotationName] != "" &&
		state.State.LatestResult != nil &&
		state.State.LatestResult.EvaluationState == eval.Error:
		// The final state is not Error, but the current evaluation errored and was mapped to another
		// state (e.g. ExecErrState=OK -> Normal, ExecErrState=Alerting -> Alerting/Pending). The error
		// detail only lives in the annotation. Gating on the current evaluation result, rather than the
		// mapped state, records the error for the evaluation that produced it while avoiding stale
		// errors that drifted onto unrelated states (e.g. Normal or NoData).
		errMsg = state.Annotations[errAnnotationName]
		state.State.Values = map[string]float64{}
	case state.State.State == eval.NoData || state.State.StateReason == eval.NoData.String():
		state.State.Values = map[string]float64{}
	}

	thresholdInputValue := 0.0
	if value, ok := state.State.Values[rule.Condition]; ok {
		thresholdInputValue = value
	}

	// Check if the alert is muted
	var silenceIds []string
	var err error
	if muteChecker != nil {
		silenceIds, err = muteChecker.GetSilenceIds(rule.OrgID, state.Labels)
		if err != nil {
			logger.Error("Failed to check if alert is muted", "error", err, "labels", state.Labels)
		}
	}

	labelMap := make(map[string]string, len(sanitizedLabels))
	maps.Copy(labelMap, sanitizedLabels)

	gcQuery := extractGCQuery(state.Annotations[gcMonitorYamlAnnotation], logger)
	sanitizedLabels[gcQueryLabel] = gcQuery
	fingerprint := calculateFingerprint(labelMap)

	parsedSummary := expandSummaryTemplate(
		state.Annotations[gcIssueHeaderAnnotation],
		template.SummaryContext{
			MonitorName: rule.Title,
			Severity:    labelMap[gcSeverityLabel],
			Labels:      template.Labels(labelMap),
			Fingerprint: fingerprint,
			Value:       state.State.Values[gcThresholdInputQueryKey],
			Threshold:   state.State.Values[gcThreshold1Key],
			State:       state.Formatted(),
			Query:       gcQuery,
			Creator:     state.Annotations[gcCreatorAnnotation],
		},
		logger,
	)

	return LokiEntry{
		SchemaVersion:             1,
		Previous:                  state.PreviousFormatted(),
		Current:                   state.Formatted(),
		Values:                    valuesAsDataBlob(state.State),
		Condition:                 rule.Condition,
		DashboardUID:              rule.DashboardUID,
		PanelID:                   rule.PanelID,
		Fingerprint:               fingerprint,
		RuleTitle:                 rule.Title,
		RuleID:                    rule.ID,
		RuleUID:                   rule.UID,
		InstanceLabels:            sanitizedLabels,
		Annotations:               cleanAnnotations(state.Annotations, annotationsToDelete),
		Error:                     errMsg,
		EvaluationDurationSeconds: state.EvaluationDuration.Seconds(),
		ThresholdInputValue:       thresholdInputValue,
		SilenceIds:                silenceIds,
		Summary:                   parsedSummary,
	}
}

// gcMonitorQueryOutput is the normalized output format for queries.
type gcMonitorQueryOutput struct {
	DataType      string `json:"data_type"`
	Expression    string `json:"expression"`
	InstantRollup string `json:"instant_rollup"`
}

// gcMonitorQueryRaw represents the raw query from the _gc_monitor_yaml annotation.
// Supports both logs and metrics formats.
type gcMonitorQueryRaw struct {
	Expression string `yaml:"expression"`

	// gcql format fields
	DataType      string `yaml:"dataType"`
	InstantRollup string `yaml:"instantRollup"`

	// metrics format fields
	DatasourceType string `yaml:"datasourceType"`
	Rollup         struct {
		Function string `yaml:"function"`
		Time     string `yaml:"time"`
	} `yaml:"rollup"`
}

type gcMonitorYaml struct {
	Model struct {
		Queries []gcMonitorQueryRaw `yaml:"queries"`
	} `yaml:"model"`
}

// extractGCQuery parses the _gc_monitor_yaml annotation and extracts model.queries[0] as a JSON string.
// Supports two YAML formats:
//
// gcql format:
//
//	model:
//	  queries:
//	  - dataType: logs
//	    name: threshold_input_query
//	    expression: '* | stats by (cluster) count() count_all_result'
//	    instantRollup: 5 minutes
//
// metrics format:
//
//	model:
//	  queries:
//	  - name: threshold_input_query
//	    expression: avg(groundcover_node_rt_disk_space_used_percent{cluster="omerk-Cluster"}) by (cluster)
//	    datasourceType: prometheus
//	    queryType: instant
//	    rollup:
//	      function: avg
//	      time: 5m
func extractGCQuery(yamlContent string, logger log.Logger) string {
	if yamlContent == "" {
		return ""
	}

	cleanYaml := html.UnescapeString(yamlContent)

	var parsed gcMonitorYaml
	if err := yaml.Unmarshal([]byte(cleanYaml), &parsed); err != nil {
		logger.Debug("Failed to parse _gc_monitor_yaml annotation", "error", err)
		return ""
	}

	if len(parsed.Model.Queries) == 0 {
		logger.Debug("No queries found in _gc_monitor_yaml annotation")
		return ""
	}

	raw := parsed.Model.Queries[0]

	output := gcMonitorQueryOutput{
		Expression: raw.Expression,
	}

	if raw.DataType != "" {
		output.DataType = raw.DataType
		output.InstantRollup = raw.InstantRollup
	} else if raw.DatasourceType == "prometheus" {
		output.DataType = "metrics"
		if raw.Rollup.Function != "" && raw.Rollup.Time != "" {
			output.InstantRollup = fmt.Sprintf("%s(%s)", raw.Rollup.Function, raw.Rollup.Time)
		}
	}

	queryJSON, err := json.Marshal(output)
	if err != nil {
		logger.Debug("Failed to marshal query to JSON", "error", err)
		return ""
	}

	return string(queryJSON)
}

func calculateFingerprint(labels data.Labels) string {
	cpLabels := labels.Copy()
	for k, v := range cpLabels {
		// The Grafana Alertmanager skips empty and namespace UID labels.
		// To get the same alert fingerprint we need to remove these labels too.
		// https://github.com/grafana/alerting/blob/2dda1c67ec02625ac9fc8607157b3d5825d47919/notify/grafana_alertmanager.go#L722-L724
		if len(v) == 0 || k == "__alert_rule_namespace_uid__" {
			delete(cpLabels, k)
		}
	}
	return labelFingerprint(cpLabels)
}

func NewHistorianExportClient(cfg LokiConfig, req client.Requester, metrics *metrics.Historian, logger log.Logger, tracer tracing.Tracer) remoteLokiClient {
	if cfg.OtelConfig.Enabled {
		return NewOtelLokiClient(cfg.OtelConfig, metrics)
	}

	return NewLokiClient(cfg, req, metrics, logger, tracer)
}

func cleanAnnotations(annotations map[string]string, annotationsToDelete map[string]struct{}) map[string]string {
	filtered := make(map[string]string, len(annotations))
	for k, v := range annotations {
		if _, shouldDelete := annotationsToDelete[k]; !shouldDelete {
			filtered[k] = v
		}
	}
	return filtered
}

func expandSummaryTemplate(
	summaryTemplate string,
	summaryCtx template.SummaryContext,
	logger log.Logger,
) string {
	if summaryTemplate == "" {
		return ""
	}

	parsed, err := template.ExpandJinja2Summary(summaryTemplate, summaryCtx)
	if err != nil {
		logger.Warn("Failed to expand issue summary template", "error", err, "template", summaryTemplate)
		return summaryTemplate
	}

	return parsed
}

type OrgAlertmanager interface {
	AlertmanagerFor(orgID int64) (notifier.Alertmanager, error)
}

// MultiOrgAlertmanagerMuteChecker implements MuteChecker using a MultiOrgAlertmanager
type MultiOrgAlertmanagerMuteChecker struct {
	moa OrgAlertmanager
}

type muteChecker interface {
	GetSilenceIds(labels data.Labels) ([]string, error)
}

// NewMultiOrgAlertmanagerMuteChecker creates a new mute checker that uses the MultiOrgAlertmanager
func NewMultiOrgAlertmanagerMuteChecker(moa OrgAlertmanager) *MultiOrgAlertmanagerMuteChecker {
	if moa == nil {
		return nil
	}
	return &MultiOrgAlertmanagerMuteChecker{
		moa: moa,
	}
}

// IsMuted checks if an alert with the given labels is muted
func (c *MultiOrgAlertmanagerMuteChecker) GetSilenceIds(orgID int64, labels data.Labels) ([]string, error) {
	if c.moa == nil {
		return nil, nil
	}

	// Get the alertmanager for this org
	am, err := c.moa.AlertmanagerFor(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get alertmanager for org %d: %w", orgID, err)
	}

	// Check if the alertmanager has the Mutes method
	// This assumes your forked alertmanager has this method

	muteAM, ok := am.(muteChecker)
	if !ok {
		// If the alertmanager doesn't support mute checking, return false
		return nil, nil
	}

	return muteAM.GetSilenceIds(labels)
}
