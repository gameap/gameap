package pluginssh

import (
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// buildAuthMethods turns the credentials a plugin supplied into SSH auth
// methods. A key is offered before a password when both are present.
func buildAuthMethods(params ConnectParams) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0, 2)

	if params.PrivateKeyPEM != "" {
		signer, err := parsePrivateKey(params.PrivateKeyPEM, params.Passphrase)
		if err != nil {
			return nil, err
		}

		methods = append(methods, ssh.PublicKeys(signer))
	}

	if params.Password != "" {
		password := params.Password
		methods = append(methods,
			ssh.Password(password),
			// Many sshd setups answer password auth with a keyboard-interactive
			// challenge; answering every prompt with the same password keeps
			// those servers reachable.
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = password
				}

				return answers, nil
			}),
		)
	}

	if len(methods) == 0 {
		return nil, ErrAuthRequired
	}

	return methods, nil
}

func parsePrivateKey(pemData, passphrase string) (ssh.Signer, error) {
	if passphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase([]byte(pemData), []byte(passphrase))
		if err != nil {
			return nil, errors.Wrap(ErrInvalidPrivateKey, err.Error())
		}

		return signer, nil
	}

	signer, err := ssh.ParsePrivateKey([]byte(pemData))
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, ErrPassphraseRequired
		}

		return nil, errors.Wrap(ErrInvalidPrivateKey, err.Error())
	}

	return signer, nil
}
