package getprofile

import (
	"github.com/gameap/gameap/internal/domain"
)

type profileResponse struct {
	ID    uint     `json:"id"`
	Login string   `json:"login"`
	Email string   `json:"email"`
	Name  *string  `json:"name"`
	Roles []string `json:"roles"`
}

func newProfileResponseFromUser(u *domain.User, roles []domain.RestrictedRole) profileResponse {
	roleNames := make([]string, 0, len(roles))
	for _, r := range roles {
		roleNames = append(roleNames, r.Name)
	}

	return profileResponse{
		ID:    u.ID,
		Login: u.Login,
		Name:  u.Name,
		Email: u.Email,
		Roles: roleNames,
	}
}
