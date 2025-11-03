package getnodes

import (
	"github.com/gameap/gameap/internal/domain"
)

type nodeResponse struct {
	ID       uint     `json:"id"`
	Enabled  bool     `json:"enabled"`
	Name     string   `json:"name"`
	OS       string   `json:"os"`
	Location string   `json:"location"`
	Provider *string  `json:"provider"`
	IP       []string `json:"ip"`
}

func newNodesResponseFromNodes(nodes []domain.Node) []nodeResponse {
	response := make([]nodeResponse, 0, len(nodes))

	for _, n := range nodes {
		response = append(response, newNodeResponseFromNode(&n))
	}

	return response
}

func newNodeResponseFromNode(n *domain.Node) nodeResponse {
	return nodeResponse{
		ID:       n.ID,
		Enabled:  n.Enabled,
		Name:     n.Name,
		OS:       string(n.OS),
		Location: n.Location,
		Provider: n.Provider,
		IP:       n.IPs,
	}
}
