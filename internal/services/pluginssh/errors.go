package pluginssh

import "github.com/pkg/errors"

// Sentinels for everything a plugin can provoke. They are wrapped with
// context at the call site and surfaced through the response "error" field —
// a host function must never return a Go error, that traps the guest call.
var (
	ErrSessionsClosed     = errors.New("plugin ssh: sessions are closed")
	ErrConnectionNotFound = errors.New("plugin ssh: connection handle not found")
	ErrOperationNotFound  = errors.New("plugin ssh: operation not found")
	ErrOperationFinished  = errors.New("plugin ssh: operation already finished")
	ErrTooManyConnections = errors.New("plugin ssh: too many open connections for plugin")
	ErrTooManyOperations  = errors.New("plugin ssh: too many running operations for plugin")

	ErrHostKeyPolicyRequired = errors.New(
		"plugin ssh: host key policy required: set accept_any or pin a fingerprint",
	)
	ErrHostKeyRejected = errors.New("plugin ssh: host key verification failed")
	ErrHostKeyInvalid  = errors.New("plugin ssh: invalid host public key")

	ErrAuthRequired       = errors.New("plugin ssh: authentication required: password or private_key_pem")
	ErrInvalidPrivateKey  = errors.New("plugin ssh: invalid private key")
	ErrPassphraseRequired = errors.New("plugin ssh: private key is encrypted, passphrase required")

	ErrDialBlocked     = errors.New("plugin ssh: target address is blocked")
	ErrHostNotResolved = errors.New("plugin ssh: failed to resolve host")
	ErrConnectTimeout  = errors.New("plugin ssh: connect timed out")

	ErrHostRequired    = errors.New("plugin ssh: host is required")
	ErrUserRequired    = errors.New("plugin ssh: user is required")
	ErrCommandRequired = errors.New("plugin ssh: command is required")
	ErrPathRequired    = errors.New("plugin ssh: path is required")
	ErrCommandTooLong  = errors.New("plugin ssh: command is too long")
	ErrStdinTooLarge   = errors.New("plugin ssh: stdin is too large")
	ErrTooManyEnvVars  = errors.New("plugin ssh: too many environment variables")
	ErrInvalidEnvName  = errors.New("plugin ssh: invalid environment variable name")
	ErrEnvRejected     = errors.New("plugin ssh: environment variable rejected by the server (sshd AcceptEnv)")
	ErrStartFailed     = errors.New("plugin ssh: failed to start command")
)

// HostKeyRejectedError names the key that was actually offered, so a plugin
// can report or pin it instead of guessing why the connection failed.
type HostKeyRejectedError struct {
	KeyType           string
	FingerprintSHA256 string
}

func (e *HostKeyRejectedError) Error() string {
	return ErrHostKeyRejected.Error() + ": " + e.KeyType + " " + e.FingerprintSHA256 + " is not accepted"
}

func (e *HostKeyRejectedError) Unwrap() error {
	return ErrHostKeyRejected
}
