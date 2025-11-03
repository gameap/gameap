package getuser

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type userResponse struct {
	ID        uint       `json:"id"`
	Login     string     `json:"login"`
	Email     string     `json:"email"`
	Name      *string    `json:"name"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	Roles     []string   `json:"roles"`
}

func newUserResponseFromUser(u *domain.User, roles []domain.RestrictedRole) userResponse {
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
	}

	return userResponse{
		ID:        u.ID,
		Login:     u.Login,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Roles:     roleNames,
	}
}
