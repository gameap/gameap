package postnode

import (
	"github.com/gameap/gameap/internal/domain"
)

type dedicatedServerResponse struct {
	ID      uint   `json:"id"`
	Message string `json:"message"`
	Result  uint   `json:"result"`
}

func newDedicatedServerResponse(node *domain.Node) dedicatedServerResponse {
	return dedicatedServerResponse{
		ID:      node.ID,
		Message: "success",
		Result:  node.ID,
	}
}
