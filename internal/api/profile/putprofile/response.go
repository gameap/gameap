package putprofile

type updateProfileResponse struct {
	Message string `json:"message"`
	// Token carries a fresh session token when the password was changed (a
	// change revokes every previously-issued token, including the caller's).
	Token string `json:"token,omitempty"`
}

func newUpdateProfileResponse() updateProfileResponse {
	return updateProfileResponse{
		Message: "Profile updated successfully",
	}
}
