package createarchive

type createArchiveRequest struct {
	Disk             string   `json:"disk"`
	Path             string   `json:"path"`
	Name             string   `json:"name"`
	Format           string   `json:"format"`
	Sources          []string `json:"sources"`
	CompressionLevel *int32   `json:"compression_level"`
	Overwrite        bool     `json:"overwrite"`
}
