package pluginssh

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

const rsaKeyBits = 4096

// GenerateKeyPair creates a key a plugin can hand to a cloud provider and
// later use with Connect. RSA-4096 takes a noticeable moment (seconds on a
// slow machine) but still fits inside a guest call deadline; ed25519 is the
// default because it is instant and universally accepted.
func GenerateKeyPair(keyType KeyType, comment string) (*KeyPair, error) {
	privateKey, err := generatePrivateKey(keyType)
	if err != nil {
		return nil, err
	}

	block, err := ssh.MarshalPrivateKey(privateKey, comment)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal private key")
	}

	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to derive public key")
	}

	publicKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	if comment != "" {
		publicKey += " " + comment
	}

	return &KeyPair{
		PrivateKeyPEM:     string(pem.EncodeToMemory(block)),
		PublicKey:         publicKey,
		FingerprintSHA256: ssh.FingerprintSHA256(signer.PublicKey()),
		KeyType:           signer.PublicKey().Type(),
	}, nil
}

func generatePrivateKey(keyType KeyType) (any, error) {
	switch keyType {
	case KeyTypeRSA4096:
		key, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate rsa key")
		}

		return key, nil
	case KeyTypeECDSAP256:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate ecdsa key")
		}

		return key, nil
	case KeyTypeED25519:
		fallthrough
	default:
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate ed25519 key")
		}

		return key, nil
	}
}
