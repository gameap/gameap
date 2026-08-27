package ssoexchange

import "github.com/gameap/gameap/pkg/api"

var errTicketRequired = api.NewValidationError("ticket is required")

type exchangeInput struct {
	Ticket string `json:"ticket"`
}

func (in *exchangeInput) Validate() error {
	if in.Ticket == "" {
		return errTicketRequired
	}

	return nil
}
