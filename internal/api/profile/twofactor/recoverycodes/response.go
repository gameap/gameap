package recoverycodes

type recoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

func newRecoveryCodesResponse(codes []string) recoveryCodesResponse {
	return recoveryCodesResponse{
		RecoveryCodes: codes,
	}
}
