package base

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

// SuspensionResponse is the suspension field of a server in API responses.
type SuspensionResponse struct {
	Since  *time.Time `json:"since"`
	Reason string     `json:"reason"`
}

// NewSuspensionResponse returns nil, shown as null, for a server that is not
// suspended.
func NewSuspensionResponse(server *domain.Server) *SuspensionResponse {
	suspension := server.Suspension()
	if suspension == nil {
		return nil
	}

	return &SuspensionResponse{
		Since:  suspension.Since,
		Reason: suspension.Reason,
	}
}
