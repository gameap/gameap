package getprofile

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/mfanudge"
)

type profileResponse struct {
	ID               uint           `json:"id"`
	Login            string         `json:"login"`
	Email            string         `json:"email"`
	Name             *string        `json:"name"`
	Roles            []string       `json:"roles"`
	TwoFactorEnabled bool           `json:"two_factor_enabled"`
	MFANudge         *mfanudge.View `json:"mfa_nudge,omitempty"`
}

func newProfileResponseFromUser(
	u *domain.User, roles []domain.RestrictedRole, nudge *mfanudge.View,
) profileResponse {
	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		roleNames = append(roleNames, r.Name)
	}

	return profileResponse{
		ID:               u.ID,
		Login:            u.Login,
		Name:             u.Name,
		Email:            u.Email,
		Roles:            roleNames,
		TwoFactorEnabled: u.TwoFactorEnabled,
		MFANudge:         nudge,
	}
}
