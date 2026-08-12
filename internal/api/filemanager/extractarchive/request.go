package extractarchive

type extractArchiveRequest struct {
	Disk              string `json:"disk"`
	Path              string `json:"path"`
	Destination       string `json:"destination"`
	Format            string `json:"format"`
	ConflictPolicy    string `json:"conflict_policy"`
	CreateDestination *bool  `json:"create_destination"`
}
