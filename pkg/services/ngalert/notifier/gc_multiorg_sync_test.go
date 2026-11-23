package notifier

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/services/ngalert/models"
)

type blockingApplyAlertmanager struct {
	Alertmanager
	entered chan struct{}
	release chan struct{}
}

func (b *blockingApplyAlertmanager) ApplyConfig(ctx context.Context, cfg *models.AlertConfiguration) error {
	b.entered <- struct{}{}
	<-b.release
	return b.Alertmanager.ApplyConfig(ctx, cfg)
}

// setupBlockedSync returns a manager whose second sync blocks while applying org 1's configuration, until release is closed.
func setupBlockedSync(t *testing.T) (mam *MultiOrgAlertmanager, release chan struct{}, synced chan struct{}) {
	mam = setupMam(t, nil)
	entered := make(chan struct{}, 3)
	release = make(chan struct{})
	factory := mam.factory
	mam.factory = func(ctx context.Context, orgID int64) (Alertmanager, error) {
		am, err := factory(ctx, orgID)
		if err != nil {
			return nil, err
		}
		return &blockingApplyAlertmanager{Alertmanager: am, entered: entered, release: release}, nil
	}

	// The first sync saves default configurations, so the second one goes through ApplyConfig.
	require.NoError(t, mam.LoadAndSyncAlertmanagersForOrgs(context.Background()))

	synced = make(chan struct{})
	go func() {
		defer close(synced)
		assert.NoError(t, mam.LoadAndSyncAlertmanagersForOrgs(context.Background()))
	}()
	<-entered
	return mam, release, synced
}

func TestGCSyncAlertmanagersForOrgs_AlertmanagerForDuringSync(t *testing.T) {
	mam, release, synced := setupBlockedSync(t)

	found := make(chan error, 1)
	go func() {
		_, err := mam.AlertmanagerFor(1)
		found <- err
	}()
	var err error
	select {
	case err = <-found:
	case <-time.After(time.Second):
		t.Error("AlertmanagerFor blocked while the sync was applying a configuration")
	}

	close(release)
	<-synced
	require.NoError(t, err)
}

func TestGCSyncAlertmanagersForOrgs_KeepsConfigSavedDuringSync(t *testing.T) {
	ctx := context.Background()
	mam, release, synced := setupBlockedSync(t)
	am, err := mam.alertmanagerForOrg(2)
	require.NoError(t, err)

	cfg, err := Load([]byte(strings.ReplaceAll(defaultConfig, "grafana-default-email", "saved-during-sync")))
	require.NoError(t, err)
	saveErr := am.SaveAndApplyConfig(ctx, cfg)

	close(release)
	<-synced
	require.NoError(t, saveErr)

	receivers, err := am.GetReceivers(ctx)
	require.NoError(t, err)
	require.Len(t, receivers, 1)
	require.Equal(t, "saved-during-sync", receivers[0].Name)
}
