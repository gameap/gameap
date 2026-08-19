package pluginssh

import (
	"net"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

const fingerprintPrefix = "SHA256:"

// observedHostKey records what the server actually presented, so the caller
// can report it whether the connection succeeded or the key was rejected.
type observedHostKey struct {
	mu          sync.Mutex
	fingerprint string
	keyType     string
}

func (o *observedHostKey) set(key ssh.PublicKey) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.fingerprint = ssh.FingerprintSHA256(key)
	o.keyType = key.Type()
}

func (o *observedHostKey) get() (fingerprint, keyType string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.fingerprint, o.keyType
}

// buildHostKeyCallback turns the requested policy into a verifier. The policy
// is validated up front so a plugin that forgot to state one is refused before
// anything is dialed, rather than silently trusting whatever answers.
func buildHostKeyCallback(policy HostKeyPolicy, observed *observedHostKey) (ssh.HostKeyCallback, error) {
	if !policy.AcceptAny && len(policy.FingerprintsSHA256) == 0 && len(policy.PublicKeys) == 0 {
		return nil, ErrHostKeyPolicyRequired
	}

	fingerprints := make(map[string]struct{}, len(policy.FingerprintsSHA256))
	for _, fingerprint := range policy.FingerprintsSHA256 {
		fingerprints[normalizeFingerprint(fingerprint)] = struct{}{}
	}

	pinned := make([]ssh.PublicKey, 0, len(policy.PublicKeys))
	for _, raw := range policy.PublicKeys {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
		if err != nil {
			return nil, ErrHostKeyInvalid
		}
		pinned = append(pinned, key)
	}

	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		observed.set(key)

		if policy.AcceptAny {
			return nil
		}

		if _, ok := fingerprints[normalizeFingerprint(ssh.FingerprintSHA256(key))]; ok {
			return nil
		}

		for _, candidate := range pinned {
			if candidate.Type() == key.Type() && string(candidate.Marshal()) == string(key.Marshal()) {
				return nil
			}
		}

		return &HostKeyRejectedError{
			KeyType:           key.Type(),
			FingerprintSHA256: ssh.FingerprintSHA256(key),
		}
	}, nil
}

// normalizeFingerprint accepts what ssh-keygen prints as well as the bare
// base64 digest, with or without padding.
func normalizeFingerprint(fingerprint string) string {
	value := strings.TrimSpace(fingerprint)
	value = strings.TrimPrefix(value, fingerprintPrefix)
	value = strings.TrimRight(value, "=")

	return fingerprintPrefix + value
}
