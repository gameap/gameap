package getstatus

import (
	"github.com/gameap/gameap/internal/domain"
)

type statusResponse struct {
	ProcessActive bool `json:"processActive"`
}

func newStatusResponse(s *domain.Server) statusResponse {
	return statusResponse{
		ProcessActive: s.IsOnline(),
	}
}
