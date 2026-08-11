package hash

type hashResponse struct {
	Algorithm string     `json:"algorithm"`
	Items     []hashItem `json:"items"`
}

type hashItem struct {
	Path  string `json:"path"`
	Hash  string `json:"hash,omitempty"`
	Size  uint64 `json:"size,omitempty"`
	Error string `json:"error,omitempty"`
}
