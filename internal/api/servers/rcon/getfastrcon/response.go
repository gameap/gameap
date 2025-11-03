package getfastrcon

import "github.com/gameap/gameap/internal/domain"

type fastRconItem struct {
	Info    string `json:"info"`
	Command string `json:"command"`
}

type fastRconResponse []fastRconItem

func newFastRconResponse(fastRcon domain.GameModFastRconList) fastRconResponse {
	if fastRcon == nil {
		return fastRconResponse{}
	}

	response := make(fastRconResponse, 0, len(fastRcon))
	for _, item := range fastRcon {
		response = append(response, fastRconItem{
			Info:    item.Info,
			Command: item.Command,
		})
	}

	return response
}
