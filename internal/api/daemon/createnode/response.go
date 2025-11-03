package createnode

import "fmt"

func buildCreateResponse(nodeID uint, apiKey, rootCert, signedCert string) string {
	return fmt.Sprintf("Success %d %s\n%s\n\n%s", nodeID, apiKey, rootCert, signedCert)
}
