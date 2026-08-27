package ssomint

type ticketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int64  `json:"expires_in"`

	// RedirectTo echoes the validated target so the caller can build the
	// login URL without having to re-derive it.
	RedirectTo string `json:"redirect_to,omitempty"`
}
