package sender

import (
	"fmt"
	"strings"

	alertingModels "github.com/grafana/alerting/models"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/common/model"
)

// PrepareAlerts adds a _gc_fingerprint annotation to each alert and removes
// empty and namespace UID labels so the webhook fingerprint stays consistent
// with the Grafana Alertmanager.
func PrepareAlerts(alerts []models.PostableAlert) {
	for i := range alerts {
		a := &alerts[i]
		if a.Annotations == nil {
			a.Annotations = make(map[string]string)
		}
		a.Annotations["_gc_fingerprint"] = ComputeGCFingerprint(a.Labels)
		for k, v := range a.Labels {
			if len(v) == 0 || k == alertingModels.NamespaceUIDLabel {
				delete(a.Labels, k)
			}
		}
	}
}

// ComputeGCFingerprint computes a fingerprint that matches the state history
// fingerprint for the same alert. It replicates the historian logic: adds
// monitor_name from alertname, strips private labels (__*) and empty values,
// then computes a Prometheus-style FNV-64a signature.
func ComputeGCFingerprint(labels map[string]string) string {
	fp := make(map[string]string, len(labels))
	for k, v := range labels {
		if len(v) == 0 || strings.HasPrefix(k, "__") || strings.HasSuffix(k, "__") {
			continue
		}
		fp[k] = v
	}
	if alertname, ok := labels["alertname"]; ok {
		fp["monitor_name"] = alertname
	}
	return fmt.Sprintf("%016x", model.LabelsToSignature(fp))
}
