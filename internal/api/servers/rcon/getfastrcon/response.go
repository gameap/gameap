package getfastrcon

import "github.com/gameap/gameap/internal/domain"

type fastRconItem struct {
	Info    string                     `json:"info"`
	Command string                     `json:"command"`
	I18n    domain.GameModFastRconI18n `json:"i18n,omitempty"`
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
			I18n:    item.I18n,
		})
	}

	return response
}
