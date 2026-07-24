package hostlibrary

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	netsdk "github.com/gameap/gameap/pkg/plugin/sdk/net"
	"github.com/tetratelabs/wazero"
)

const (
	defaultNetMaxReadBytes = 64 * 1024
	defaultNetMaxTimeout   = 10 * time.Second
)

// NetConfig bounds plugin socket I/O performed through the gameap-net library.
type NetConfig struct {
	// MaxReadBytes caps a single Recv; a larger max_bytes request is clamped.
	MaxReadBytes int
	// MaxTimeout caps any single read/write regardless of the requested timeout
	// and of the connection's overall deadline.
	MaxTimeout time.Duration
}

func (c NetConfig) withDefaults() NetConfig {
	if c.MaxReadBytes <= 0 {
		c.MaxReadBytes = defaultNetMaxReadBytes
	}

	if c.MaxTimeout <= 0 {
		c.MaxTimeout = defaultNetMaxTimeout
	}

	return c
}

// NetServiceImpl implements netsdk.NetService for a single plugin. It performs
// read/write on host-opened connections resolved through the shared registry,
// verifying that the calling plugin owns the handle. It never dials — a plugin
// can only ever touch connections the host opened for it.
type NetServiceImpl struct {
	pluginID string
	registry *pkgplugin.ConnRegistry
	cfg      NetConfig
}

func (s *NetServiceImpl) Send(_ context.Context, req *netsdk.NetSendRequest) (*netsdk.NetSendResponse, error) {
	conn, deadline, err := s.registry.Conn(req.Handle, s.pluginID)
	if err != nil {
		return &netsdk.NetSendResponse{Error: netErrString(err)}, nil
	}

	_ = conn.SetWriteDeadline(s.effectiveDeadline(deadline, 0))

	n, err := conn.Write(req.Data)
	resp := &netsdk.NetSendResponse{Written: int32(n)} //nolint:gosec
	if err != nil {
		resp.Error = netErrString(err)
	}

	return resp, nil
}

func (s *NetServiceImpl) Recv(_ context.Context, req *netsdk.NetRecvRequest) (*netsdk.NetRecvResponse, error) {
	conn, deadline, err := s.registry.Conn(req.Handle, s.pluginID)
	if err != nil {
		return &netsdk.NetRecvResponse{Error: netErrString(err)}, nil
	}

	maxBytes := int(req.MaxBytes)
	if maxBytes <= 0 || maxBytes > s.cfg.MaxReadBytes {
		maxBytes = s.cfg.MaxReadBytes
	}

	_ = conn.SetReadDeadline(s.effectiveDeadline(deadline, time.Duration(req.TimeoutMs)*time.Millisecond))

	buf := make([]byte, maxBytes)
	n, err := conn.Read(buf)
	resp := &netsdk.NetRecvResponse{Data: buf[:n]}

	if err != nil {
		if isTimeoutErr(err) {
			resp.Timeout = true
		} else {
			resp.Error = netErrString(err)
		}
	}

	return resp, nil
}

func (s *NetServiceImpl) Close(_ context.Context, req *netsdk.NetCloseRequest) (*netsdk.NetCloseResponse, error) {
	if err := s.registry.CloseOwned(req.Handle, s.pluginID); err != nil {
		return &netsdk.NetCloseResponse{Error: netErrString(err)}, nil
	}

	return &netsdk.NetCloseResponse{}, nil
}

// effectiveDeadline is the earliest of: now+MaxTimeout, the connection's overall
// deadline, and (when non-zero) now+callTimeout.
func (s *NetServiceImpl) effectiveDeadline(connDeadline time.Time, callTimeout time.Duration) time.Time {
	deadline := time.Now().Add(s.cfg.MaxTimeout)

	if !connDeadline.IsZero() && connDeadline.Before(deadline) {
		deadline = connDeadline
	}

	if callTimeout > 0 {
		if callDeadline := time.Now().Add(callTimeout); callDeadline.Before(deadline) {
			deadline = callDeadline
		}
	}

	return deadline
}

// NetHostLibrary wires a per-plugin gameap-net module into a wazero runtime.
type NetHostLibrary struct {
	impl *NetServiceImpl
}

func (l *NetHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return netsdk.Instantiate(ctx, r, l.impl)
}

// NetHostLibraryFactory builds a gameap-net library bound to each plugin's ID,
// all sharing one connection registry.
type NetHostLibraryFactory struct {
	registry *pkgplugin.ConnRegistry
	cfg      NetConfig
}

// NewNetHostLibraryFactory constructs the factory. The registry must be the same
// instance handed to the protocol runner so handles opened by the host resolve
// for the plugin performing I/O.
func NewNetHostLibraryFactory(registry *pkgplugin.ConnRegistry, cfg NetConfig) *NetHostLibraryFactory {
	return &NetHostLibraryFactory{registry: registry, cfg: cfg.withDefaults()}
}

func (f *NetHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &NetHostLibrary{impl: &NetServiceImpl{
		pluginID: pkgplugin.CompactPluginID(domain.Uint64ID(pluginID)),
		registry: f.registry,
		cfg:      f.cfg,
	}}
}

func isTimeoutErr(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}

func netErrString(err error) *string {
	s := err.Error()

	return &s
}
