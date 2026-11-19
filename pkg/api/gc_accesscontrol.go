package api

import ac "github.com/grafana/grafana/pkg/services/accesscontrol"

// gcRemovedActions are the write actions groundcover (theatre) withholds from org admins.
var gcRemovedActions = map[string]map[string]bool{
	"fixed:organization:writer": {ac.ActionOrgsWrite: true, ac.ActionOrgsPreferencesWrite: true},
	"fixed:teams:writer": {
		ac.ActionTeamsCreate: true, ac.ActionTeamsDelete: true,
		ac.ActionTeamsPermissionsWrite: true, ac.ActionTeamsWrite: true,
	},
}

// gcRestrictOrgAdminRoles drops gcRemovedActions from the fixed roles before they are declared.
func gcRestrictOrgAdminRoles(roles []ac.RoleRegistration) []ac.RoleRegistration {
	for i := range roles {
		removed := gcRemovedActions[roles[i].Role.Name]
		if removed == nil {
			continue
		}
		kept := make([]ac.Permission, 0, len(roles[i].Role.Permissions))
		for _, p := range roles[i].Role.Permissions {
			if !removed[p.Action] {
				kept = append(kept, p)
			}
		}
		roles[i].Role.Permissions = kept
	}
	return roles
}
