package pluginssh

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/pkg/netutil"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

const defaultSSHPort = 22

// resolveAndCheck performs the SSRF-safe lookup for a target. Every resolved
// address is checked and the chosen one is dialed as a literal, so a name that
// answers differently on the second lookup cannot smuggle the connection to a
// blocked address.
func (s *Service) resolveAndCheck(ctx context.Context, host string) (netip.Addr, error) {
	allowBypass := s.hostAllowed(host)

	if ip, err := netip.ParseAddr(host); err == nil {
		if err := s.checkIP(ip, allowBypass); err != nil {
			return netip.Addr{}, err
		}

		return ip, nil
	}

	ips, err := s.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, errors.Wrap(ErrHostNotResolved, err.Error())
	}

	if len(ips) == 0 {
		return netip.Addr{}, errors.Wrap(ErrHostNotResolved, host)
	}

	// Reject if ANY resolved address is blocked rather than picking a "safe"
	// one: a hostname that answers with a public and a private address at once
	// is the classic way past a naive check.
	for _, ip := range ips {
		if err := s.checkIP(ip, allowBypass); err != nil {
			return netip.Addr{}, err
		}
	}

	return ips[0], nil
}

func (s *Service) checkIP(ip netip.Addr, allowBypass bool) error {
	// An IPv4-mapped IPv6 address routes to the IPv4 target but fails the
	// Is4-based checks, so canonicalise before deciding.
	ip = ip.Unmap()

	// Cloud metadata is blocked whatever the operator allowed: it hands out
	// credentials of the machine the panel runs on.
	if netutil.IsCloudMetadataIP(ip) {
		return errors.Wrapf(ErrDialBlocked, "ip=%s reason=%s", ip, netutil.BlockReasonCloudMetadata)
	}

	if !s.cfg.BlockPrivateIPs || allowBypass {
		return nil
	}

	if reason := netutil.BlockReason(ip); reason != "" {
		return errors.Wrapf(ErrDialBlocked, "ip=%s reason=%s", ip, reason)
	}

	return nil
}

func (s *Service) hostAllowed(host string) bool {
	if len(s.allowedHosts) == 0 {
		return false
	}

	_, ok := s.allowedHosts[strings.ToLower(strings.TrimSpace(host))]

	return ok
}

// dialSSH opens the TCP connection and completes the SSH handshake. The
// handshake runs on its own goroutine because ssh.ClientConfig.Timeout covers
// only the TCP dial, and an unresponsive server would otherwise hold the guest
// call past its deadline.
func (s *Service) dialSSH(
	ctx context.Context,
	params ConnectParams,
	clientConfig *ssh.ClientConfig,
) (*ssh.Client, error) {
	port := params.Port
	if port == 0 {
		port = defaultSSHPort
	}

	ip, err := s.resolveAndCheck(ctx, params.Host)
	if err != nil {
		return nil, err
	}

	address := net.JoinHostPort(ip.String(), strconv.FormatUint(uint64(port), 10))

	conn, err := s.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, errors.Wrap(ErrConnectTimeout, err.Error())
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	type handshakeResult struct {
		conn  ssh.Conn
		chans <-chan ssh.NewChannel
		reqs  <-chan *ssh.Request
		err   error
	}

	// The original host name goes into the handshake so host-key checking and
	// server-side logging see the name the plugin asked for, not the literal.
	handshakeAddr := net.JoinHostPort(params.Host, strconv.FormatUint(uint64(port), 10))

	results := make(chan handshakeResult, 1)
	go func() {
		sshConn, chans, reqs, err := ssh.NewClientConn(conn, handshakeAddr, clientConfig)
		results <- handshakeResult{conn: sshConn, chans: chans, reqs: reqs, err: err}
	}()

	select {
	case result := <-results:
		if result.err != nil {
			_ = conn.Close()

			return nil, classifyHandshakeError(ctx, result.err)
		}

		_ = conn.SetDeadline(time.Time{})

		return ssh.NewClient(result.conn, result.chans, result.reqs), nil
	case <-ctx.Done():
		// Closing the socket unblocks the handshake goroutine, which then
		// drains into the buffered channel and exits.
		_ = conn.Close()

		return nil, errors.Wrap(ErrConnectTimeout, ctx.Err().Error())
	}
}

// classifyHandshakeError keeps an exhausted connect budget behind the
// ErrConnectTimeout sentinel. The read deadline on the socket and the context
// expire at the same instant, so a stalled handshake surfaces either as a net
// timeout from the handshake goroutine or as ctx.Done(), depending on which
// one the scheduler reaches first; the plugin must see the same error either
// way.
func classifyHandshakeError(ctx context.Context, err error) error {
	var netErr net.Error

	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout())

	if timedOut {
		return errors.Wrap(ErrConnectTimeout, err.Error())
	}

	return err
}
