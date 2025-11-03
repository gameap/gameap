package getusers

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
}

func newUsersResponseFromUsers(users []domain.User) []userResponse {
	response := make([]userResponse, 0, len(users))

	for _, u := range users {
		response = append(response, newUserResponseFromUser(&u))
	}

	return response
}

func newUserResponseFromUser(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Login:     u.Login,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
