package postsuspend

type suspendResponse struct {
	DaemonTaskID *uint `json:"gdaemonTaskId"`
}

func newSuspendResponse(stopTaskID uint) *suspendResponse {
	if stopTaskID == 0 {
		return &suspendResponse{}
	}

	return &suspendResponse{DaemonTaskID: new(stopTaskID)}
}
