package getiplist

import "github.com/gameap/gameap/internal/domain"

type ipListResponse []string

func newIPListResponse(ips domain.IPList) ipListResponse {
	if ips == nil {
		return ipListResponse{}
	}

	return ipListResponse(ips)
}
