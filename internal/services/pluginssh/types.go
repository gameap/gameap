package pluginssh

import "time"

// Status is the lifecycle of one remote command.
type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusTimedOut  Status = "timed_out"
	StatusCanceled  Status = "canceled"
)

// Finished reports whether the operation reached a terminal state.
func (s Status) Finished() bool {
	return s != StatusRunning && s != ""
}

type KeyType string

const (
	KeyTypeED25519   KeyType = "ed25519"
	KeyTypeRSA4096   KeyType = "rsa4096"
	KeyTypeECDSAP256 KeyType = "ecdsa_p256"
)

// KeyPair is a freshly generated SSH key. The private half is returned to the
// caller once and never stored by the panel.
type KeyPair struct {
	PrivateKeyPEM     string
	PublicKey         string
	FingerprintSHA256 string
	KeyType           string
}

// HostKeyPolicy decides which server keys are acceptable.
type HostKeyPolicy struct {
	AcceptAny          bool
	FingerprintsSHA256 []string
	PublicKeys         []string
}

// ConnectParams describes one outbound SSH connection.
type ConnectParams struct {
	Host           string
	Port           uint32
	User           string
	Password       string
	PrivateKeyPEM  string
	Passphrase     string
	HostKey        HostKeyPolicy
	ConnectTimeout time.Duration
	IdleTimeout    time.Duration
}

// ConnectResult carries the handle plus what the host presented, so a plugin
// using AcceptAny can pin the key for later connections. HostKeyVerified says
// whether the offered key was actually checked against pins — an accept_any
// connection reports false, and the audit trail keeps the difference.
type ConnectResult struct {
	Handle uint64
	// Address is the numeric ip:port the panel actually dialed — with an
	// allowlisted name this can differ from what the plugin asked for.
	Address                  string
	HostKeyFingerprintSHA256 string
	HostKeyType              string
	ServerVersion            string
	HostKeyVerified          bool
}

// ExecParams describes one remote command.
type ExecParams struct {
	Handle         uint64
	Command        string
	Stdin          []byte
	Env            map[string]string
	Timeout        time.Duration
	MaxOutputBytes int
	// NotifyCompletion asks for the completion callback to be delivered into
	// the plugin once the command finishes.
	NotifyCompletion bool
}

// ExecSnapshot is a point-in-time view of an operation, including the slice of
// captured output the caller asked for.
type ExecSnapshot struct {
	OperationID string
	Handle      uint64
	Status      Status
	ExitCode    int32
	ExitSignal  string
	Error       string

	Stdout           []byte
	Stderr           []byte
	StdoutNextOffset uint64
	StderrNextOffset uint64
	StdoutTotal      uint64
	StderrTotal      uint64
	StdoutTruncated  bool
	StderrTruncated  bool

	StartedAt  time.Time
	FinishedAt time.Time
}

// Succeeded is the "did what I asked" answer: the command ran and exited 0.
func (s ExecSnapshot) Succeeded() bool {
	return s.Status == StatusCompleted && s.ExitCode == 0
}
