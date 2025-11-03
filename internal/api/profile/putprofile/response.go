package putprofile

type updateProfileResponse struct {
	Message string `json:"message"`
}

func newUpdateProfileResponse() updateProfileResponse {
	return updateProfileResponse{
		Message: "Profile updated successfully",
	}
}
