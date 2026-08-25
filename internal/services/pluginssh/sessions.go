package pluginssh

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"strings"
	"sync"
	"time"

	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

const (
	maxCommandBytes = 64 << 10
	maxEnvVars      = 128
	handleAttempts  = 8

	// minIdleTimeout floors a plugin-requested idle timeout: the watchdog tick
	// derives from it, and a millisecond request would turn the sweep into a
	// probe flood at the remote host. A ceiling configured below the floor
	// (tests, a deliberate operator choice) wins.
	minIdleTimeout = 10 * time.Second
)

// Sessions holds everything one gameap-ssh module instance owns: its open
// connections and the operations running on them. A module instance belongs to
// exactly one loaded plugin, so cleanup on unload is simply closing this set,
// and a handle or operation id from another instance is unknown here by
// construction.
type Sessions struct {
	svc      *Service
	pluginID uint64

	mu       sync.Mutex
	conns    map[uint64]*connection
	ops      map[string]*operation
	finished []string
	timers   map[string]*time.Timer
	running  int
	closed   bool

	closedCh chan struct{}
}

func newSessions(svc *Service, pluginID uint64) *Sessions {
	return &Sessions{
		svc:      svc,
		pluginID: pluginID,
		conns:    make(map[uint64]*connection),
		ops:      make(map[string]*operation),
		timers:   make(map[string]*time.Timer),
		closedCh: make(chan struct{}),
	}
}

// Connect validates the request, dials under the panel's address policy and
// registers the resulting client under an unpredictable handle.
func (p *Sessions) Connect(ctx context.Context, params ConnectParams) (*ConnectResult, error) {
	// hostAllowed trims its side of the comparison; resolution and the stored
	// conn.host must see the same spelling.
	params.Host = strings.TrimSpace(params.Host)

	if err := validateConnect(params); err != nil {
		return nil, err
	}

	if err := p.checkConnectionSlot(); err != nil {
		return nil, err
	}

	client, address, observed, err := p.dial(ctx, params)
	if err != nil {
		return nil, err
	}

	fingerprint, keyType := observed.get()

	handle, err := p.registerConnection(client, params, fingerprint)
	if err != nil {
		_ = client.Close()

		return nil, err
	}

	p.svc.logger.Info("plugin ssh connection opened",
		slog.Uint64("plugin_id", p.pluginID),
		slog.String("host", params.Host),
		slog.Uint64("port", uint64(params.Port)),
		slog.String("user", params.User),
		slog.String("host_key_type", keyType),
		slog.String("host_key_fingerprint", fingerprint),
		slog.Uint64("handle", handle))

	return &ConnectResult{
		Handle:                   handle,
		Address:                  address,
		HostKeyFingerprintSHA256: fingerprint,
		HostKeyType:              keyType,
		ServerVersion:            string(client.ServerVersion()),
		HostKeyVerified:          !params.HostKey.AcceptAny,
	}, nil
}

// dial builds the client config and performs the handshake. The observed host
// key is returned even on failure so the caller can report what answered; the
// address is the numeric ip:port actually dialed, for the audit trail.
func (p *Sessions) dial(ctx context.Context, params ConnectParams) (*ssh.Client, string, *observedHostKey, error) {
	observed := &observedHostKey{}

	if params.HostKey.AcceptAny && p.svc.cfg.DisallowAcceptAnyHostKey {
		return nil, "", observed, ErrHostKeyAcceptAnyDisabled
	}

	hostKeyCallback, err := buildHostKeyCallback(params.HostKey, observed)
	if err != nil {
		return nil, "", observed, err
	}

	authMethods, err := buildAuthMethods(params)
	if err != nil {
		return nil, "", observed, err
	}

	timeout := clampDuration(params.ConnectTimeout, p.svc.cfg.ConnectTimeout)

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, address, err := p.svc.dialSSH(dialCtx, p.pluginID, params, &ssh.ClientConfig{
		User:            params.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	})
	if err != nil {
		return nil, "", observed, err
	}

	return client, address, observed, nil
}

// checkConnectionSlot fails fast before an expensive dial; registerConnection
// re-checks under the lock, since concurrent connects may race here.
func (p *Sessions) checkConnectionSlot() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrSessionsClosed
	}

	if len(p.conns) >= p.svc.cfg.MaxConnections {
		return ErrTooManyConnections
	}

	return nil
}

func (p *Sessions) registerConnection(client *ssh.Client, params ConnectParams, fingerprint string) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, ErrSessionsClosed
	}

	if len(p.conns) >= p.svc.cfg.MaxConnections {
		return 0, ErrTooManyConnections
	}

	handle, err := p.newHandleLocked()
	if err != nil {
		return 0, err
	}

	idleTimeout := clampDuration(params.IdleTimeout, p.svc.cfg.IdleTimeout)
	if idleTimeout > 0 && idleTimeout < minIdleTimeout && p.svc.cfg.IdleTimeout >= minIdleTimeout {
		idleTimeout = minIdleTimeout
	}

	conn := &connection{
		handle:      handle,
		client:      client,
		host:        params.Host,
		fingerprint: fingerprint,
		idleTimeout: idleTimeout,
		lastUsed:    time.Now(),
		closed:      make(chan struct{}),
	}
	p.conns[handle] = conn

	go conn.run(p)

	return handle, nil
}

// newHandleLocked returns an unpredictable non-zero handle, so a plugin cannot
// guess one belonging to a different connection.
func (p *Sessions) newHandleLocked() (uint64, error) {
	var buf [8]byte

	for range handleAttempts {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, errors.Wrap(err, "failed to generate connection handle")
		}

		handle := binary.BigEndian.Uint64(buf[:])
		if handle == 0 {
			continue
		}

		if _, exists := p.conns[handle]; !exists {
			return handle, nil
		}
	}

	return 0, errors.New("failed to generate a unique connection handle")
}

// Disconnect closes a connection and cancels whatever was running on it.
func (p *Sessions) Disconnect(handle uint64) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()

		return ErrSessionsClosed
	}

	conn, ok := p.conns[handle]
	if !ok {
		p.mu.Unlock()

		return ErrConnectionNotFound
	}
	delete(p.conns, handle)
	p.mu.Unlock()

	p.cancelConnectionOperations(handle, "connection closed by plugin", nil)
	conn.close()

	return nil
}

func (p *Sessions) connection(handle uint64) (*connection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrSessionsClosed
	}

	conn, ok := p.conns[handle]
	if !ok {
		return nil, ErrConnectionNotFound
	}

	// The handout itself is use: prepareSession can spend longer than the idle
	// budget on a slow network, and the sweep must not close the connection
	// under a command that is still being started.
	conn.touch()

	return conn, nil
}

// ConnectionHost names the host a handle is connected to, so the audit records
// of commands and file transfers can be tied to the machine they ran on.
func (p *Sessions) ConnectionHost(handle uint64) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return "", false
	}

	conn, ok := p.conns[handle]
	if !ok {
		return "", false
	}

	return conn.host, true
}

func (p *Sessions) cancelConnectionOperations(handle uint64, reason string, cause error) {
	p.mu.Lock()
	targets := make([]*operation, 0, len(p.ops))
	for _, op := range p.ops {
		if op.handle == handle {
			targets = append(targets, op)
		}
	}
	p.mu.Unlock()

	for _, op := range targets {
		op.cancel(&cancelCauseError{reason: reason, cause: cause})
	}
}

// Close releases every connection and operation of this module instance. It
// runs while the plugin manager lock is held during unload, so it must not
// block on guest work: pending callbacks are dropped, not awaited.
func (p *Sessions) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()

		return
	}
	p.closed = true
	close(p.closedCh)

	conns := make([]*connection, 0, len(p.conns))
	for _, conn := range p.conns {
		conns = append(conns, conn)
	}
	p.conns = make(map[uint64]*connection)

	ops := make([]*operation, 0, len(p.ops))
	for _, op := range p.ops {
		ops = append(ops, op)
	}
	p.ops = make(map[string]*operation)
	p.finished = nil
	p.running = 0

	for _, timer := range p.timers {
		timer.Stop()
	}
	p.timers = make(map[string]*time.Timer)
	p.mu.Unlock()

	for _, op := range ops {
		// A reloaded instance must not receive completions of the previous
		// one, so interest is dropped before the cancellation lands.
		op.dropNotification()
		op.cancel(&cancelCauseError{reason: "plugin unloaded"})
	}

	for _, conn := range conns {
		conn.close()
	}

	p.svc.forget(p)
}

func validateConnect(params ConnectParams) error {
	if strings.TrimSpace(params.Host) == "" {
		return ErrHostRequired
	}

	if params.Port > 65535 {
		return ErrInvalidPort
	}

	if strings.TrimSpace(params.User) == "" {
		return ErrUserRequired
	}

	return nil
}

// clampDuration applies the panel ceiling: 0 means "panel default", anything
// larger than the ceiling is cut down to it.
func clampDuration(requested, ceiling time.Duration) time.Duration {
	if requested <= 0 || requested > ceiling {
		return ceiling
	}

	return requested
}

func clampBytes(requested, ceiling int) int {
	if requested <= 0 || requested > ceiling {
		return ceiling
	}

	return requested
}

// cancelCauseError carries the reason a plugin (or the panel) stopped an
// operation. cause distinguishes the engine's own interruptions (a lost
// connection) from a deliberate cancellation; classifyExec unwraps to it.
type cancelCauseError struct {
	reason string
	cause  error
}

func (c *cancelCauseError) Error() string {
	return "canceled: " + c.reason
}

func (c *cancelCauseError) Unwrap() error {
	return c.cause
}

var (
	errExecTimeout    = errors.New("timed out")
	errConnectionLost = errors.New("connection closed")
)

// completionRequest renders the operation for the guest callback.
func (o *operation) completionRequest() *sshsdk.HandleExecCompletedRequest {
	snapshot := o.snapshot(0, 0)

	request := &sshsdk.HandleExecCompletedRequest{
		OperationId:      snapshot.OperationID,
		Handle:           snapshot.Handle,
		Status:           statusToProto(snapshot.Status),
		Success:          snapshot.Succeeded(),
		ExitCode:         snapshot.ExitCode,
		ExitSignal:       snapshot.ExitSignal,
		StdoutTruncated:  snapshot.StdoutTruncated,
		StderrTruncated:  snapshot.StderrTruncated,
		StdoutTotalBytes: snapshot.StdoutTotal,
		StderrTotalBytes: snapshot.StderrTotal,
		StartedAt:        snapshot.StartedAt.UnixMilli(),
	}

	if !snapshot.FinishedAt.IsZero() {
		request.FinishedAt = snapshot.FinishedAt.UnixMilli()
	}

	if snapshot.Error != "" {
		request.Error = &snapshot.Error
	}

	return request
}

// statusToProto maps the engine status onto the wire enum.
func statusToProto(status Status) sshsdk.ExecStatus {
	switch status {
	case StatusRunning:
		return sshsdk.ExecStatus_EXEC_STATUS_RUNNING
	case StatusCompleted:
		return sshsdk.ExecStatus_EXEC_STATUS_COMPLETED
	case StatusFailed:
		return sshsdk.ExecStatus_EXEC_STATUS_FAILED
	case StatusTimedOut:
		return sshsdk.ExecStatus_EXEC_STATUS_TIMED_OUT
	case StatusCanceled:
		return sshsdk.ExecStatus_EXEC_STATUS_CANCELED
	default:
		return sshsdk.ExecStatus_EXEC_STATUS_UNSPECIFIED
	}
}
