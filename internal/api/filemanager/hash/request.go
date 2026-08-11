package hash

type hashRequest struct {
	Disk      string   `json:"disk"`
	Paths     []string `json:"paths"`
	Algorithm string   `json:"algorithm"`
}
