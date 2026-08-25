package pluginssh

import "time"

// Defaults chosen so a plugin bootstrapping a machine works out of the box
// while a misbehaving one cannot exhaust the panel.
const (
	defaultMaxConnections     = 8
	defaultMaxOperations      = 16
	defaultConnectTimeout     = 30 * time.Second
	defaultMaxExecTimeout     = 30 * time.Minute
	defaultIdleTimeout        = 10 * time.Minute
	defaultMaxOutputBytes     = 1 << 20
	defaultMaxStdinBytes      = 1 << 20
	defaultOperationRetention = 10 * time.Minute
	defaultRetainedOperations = 64
	defaultKeepaliveInterval  = 30 * time.Second

	defaultCompletionCallTimeout = 30 * time.Second
	defaultBusyRetryDelay        = 2 * time.Second
	defaultBusyRetries           = 5
)

// Config is the operator-facing configuration of the SSH engine. It mirrors
// internal/config Plugin.SSH one-for-one; the indirection lets tests build a
// service without depending on the whole config package.
type Config struct {
	// BlockPrivateIPs refuses to dial a target whose post-DNS address is
	// loopback / RFC1918 / link-local / CGNAT. Cloud-metadata addresses are
	// blocked regardless.
	BlockPrivateIPs bool
	// AllowedHosts bypasses the private-IP block for the listed hostnames.
	AllowedHosts []string

	// DisallowAcceptAnyHostKey refuses connections whose host key policy is
	// accept_any, forcing plugins to pin. The zero value permits accept_any:
	// first contact with a machine that was just created has nothing to pin
	// yet, so trust-on-first-use stays the default.
	DisallowAcceptAnyHostKey bool

	MaxConnections int
	MaxOperations  int
	ConnectTimeout time.Duration
	MaxExecTimeout time.Duration
	IdleTimeout    time.Duration
	MaxOutputBytes int
	MaxStdinBytes  int

	// OperationRetention keeps a finished operation readable so a plugin that
	// polls late still sees the outcome.
	OperationRetention    time.Duration
	MaxRetainedOperations int
	KeepaliveInterval     time.Duration

	CompletionCallTimeout time.Duration
	BusyRetryDelay        time.Duration
	BusyRetries           int
}

func (c Config) withDefaults() Config {
	if c.MaxConnections <= 0 {
		c.MaxConnections = defaultMaxConnections
	}
	if c.MaxOperations <= 0 {
		c.MaxOperations = defaultMaxOperations
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = defaultConnectTimeout
	}
	if c.MaxExecTimeout <= 0 {
		c.MaxExecTimeout = defaultMaxExecTimeout
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.MaxOutputBytes <= 0 {
		c.MaxOutputBytes = defaultMaxOutputBytes
	}
	if c.MaxStdinBytes <= 0 {
		c.MaxStdinBytes = defaultMaxStdinBytes
	}
	if c.OperationRetention <= 0 {
		c.OperationRetention = defaultOperationRetention
	}
	if c.MaxRetainedOperations <= 0 {
		c.MaxRetainedOperations = defaultRetainedOperations
	}
	if c.KeepaliveInterval <= 0 {
		c.KeepaliveInterval = defaultKeepaliveInterval
	}
	if c.CompletionCallTimeout <= 0 {
		c.CompletionCallTimeout = defaultCompletionCallTimeout
	}
	if c.BusyRetryDelay <= 0 {
		c.BusyRetryDelay = defaultBusyRetryDelay
	}
	if c.BusyRetries <= 0 {
		c.BusyRetries = defaultBusyRetries
	}

	return c
}
