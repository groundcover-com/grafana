package notifier

import (
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/prometheus/common/model"
)

// GetSilenceIds returns the IDs of active silences that mute the given labels (groundcover).
func (am *alertmanager) GetSilenceIds(labels data.Labels) ([]string, error) {
	labelSet := make(model.LabelSet, len(labels))
	for k, v := range labels {
		labelSet[model.LabelName(k)] = model.LabelValue(v)
	}
	return am.Base.Mutes(labelSet)
}
