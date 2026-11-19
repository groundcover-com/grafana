package api

import (
	"testing"

	"github.com/stretchr/testify/require"

	ac "github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/services/accesscontrol/actest"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/services/licensing"
	"github.com/grafana/grafana/pkg/setting"
)

type recordingACService struct {
	actest.FakeService
	roles map[string][]string
}

func (r *recordingACService) DeclareFixedRoles(regs ...ac.RoleRegistration) error {
	for _, reg := range regs {
		for _, p := range reg.Role.Permissions {
			r.roles[reg.Role.Name] = append(r.roles[reg.Role.Name], p.Action)
		}
	}
	return nil
}

// Guards the theatre org-admin restriction against upstream role renames.
func TestGCRestrictOrgAdminRoles(t *testing.T) {
	rec := &recordingACService{roles: map[string][]string{}}
	hs := &HTTPServer{Cfg: setting.NewCfg(), Features: featuremgmt.WithFeatures(), License: &licensing.OSSLicensingService{}, accesscontrolService: rec}
	require.NoError(t, hs.declareFixedRoles())

	for name, removed := range gcRemovedActions {
		actions, ok := rec.roles[name]
		require.True(t, ok, "role %s no longer declared upstream", name)
		for action := range removed {
			require.NotContains(t, actions, action, name)
		}
	}
	require.ElementsMatch(t, []string{ac.ActionOrgsRead, ac.ActionOrgsQuotasRead, ac.ActionOrgsPreferencesRead}, rec.roles["fixed:organization:writer"])
	require.ElementsMatch(t, []string{ac.ActionTeamsPermissionsRead, ac.ActionTeamsRead}, rec.roles["fixed:teams:writer"])
}
