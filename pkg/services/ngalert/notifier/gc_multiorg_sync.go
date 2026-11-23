package notifier

import (
	"context"
	"errors"
	"maps"

	"github.com/grafana/grafana/pkg/services/ngalert/store"
)

// gcSyncAlertmanagersForOrgs is SyncAlertmanagersForOrgs with configs applied outside alertmanagersMtx: with
// many orgs the loop takes minutes, and every alert delivery waits on that lock. Each Alertmanager serializes
// its own config changes, and each org's config is read right before applying it, so a save made during the
// loop is not reverted.
func (moa *MultiOrgAlertmanager) gcSyncAlertmanagersForOrgs(ctx context.Context, orgIDs []int64) {
	orgsFound := make(map[int64]struct{}, len(orgIDs))

	moa.alertmanagersMtx.RLock()
	existing := maps.Clone(moa.alertmanagers)
	moa.alertmanagersMtx.RUnlock()

	created := map[int64]Alertmanager{}
	for _, orgID := range orgIDs {
		if _, isDisabledOrg := moa.settings.UnifiedAlerting.DisabledOrgs[orgID]; isDisabledOrg {
			moa.logger.Debug("Skipping syncing Alertmanager for disabled org", "org", orgID)
			continue
		}
		orgsFound[orgID] = struct{}{}

		alertmanager, found := existing[orgID]
		if !found {
			am, err := moa.factory(ctx, orgID)
			if err != nil {
				moa.logger.Error("Unable to create Alertmanager for org", "org", orgID, "error", err)
				continue
			}
			created[orgID] = am
			alertmanager = am
		}

		dbConfig, err := moa.configStore.GetLatestAlertmanagerConfiguration(ctx, orgID)
		if errors.Is(err, store.ErrNoAlertmanagerConfiguration) {
			if found {
				moa.logger.Warn("Alertmanager exists for org but the configuration is gone. Applying the default configuration", "org", orgID)
			}
			if err := alertmanager.SaveAndApplyDefaultConfig(ctx); err != nil {
				moa.logger.Error("Failed to apply the default Alertmanager configuration", "org", orgID)
			}
			continue
		}
		if err != nil {
			moa.logger.Error("Failed to load Alertmanager configuration for org", "org", orgID, "error", err)
			continue
		}

		if err := alertmanager.ApplyConfig(ctx, dbConfig); err != nil {
			moa.logger.Error("Failed to apply Alertmanager config for org", "org", orgID, "id", dbConfig.ID, "error", err)
		}
	}

	moa.alertmanagersMtx.Lock()
	maps.Copy(moa.alertmanagers, created)
	amsToStop := map[int64]Alertmanager{}
	for orgID, am := range moa.alertmanagers {
		if _, exists := orgsFound[orgID]; !exists {
			amsToStop[orgID] = am
			delete(moa.alertmanagers, orgID)
			moa.metrics.RemoveOrgRegistry(orgID)
		}
	}
	moa.metrics.ActiveConfigurations.Set(float64(len(moa.alertmanagers)))
	moa.alertmanagersMtx.Unlock()

	for orgID, am := range amsToStop {
		moa.logger.Info("Stopping Alertmanager", "org", orgID)
		am.StopAndWait()
		moa.logger.Info("Stopped Alertmanager", "org", orgID)
	}

	moa.cleanupOrphanLocalOrgState(ctx, orgsFound)
}
