// OWASP API Security Top 10:2023 — API7:2023 Server Side Request Forgery.
// gameap-ssh is the one host library where a plugin names the target itself,
// so the address policy is the boundary that keeps a plugin from reaching the
// panel's own network or the cloud metadata service. These tests pin it, plus
// the host-key verification that stops a plugin from talking to an impostor.
package pluginssh

import (
	"context"
	"net"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// recordingDialer proves the policy decided before any socket was opened.
type recordingDialer struct {
	dialed []string
}

func (d *recordingDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.dialed = append(d.dialed, address)

	return nil, net.ErrClosed
}

func newPolicySessions(t *testing.T, cfg Config, resolver netResolver) (*Sessions, *recordingDialer) {
	t.Helper()

	dialer := &recordingDialer{}
	svc := newService(nil, nil, cfg, nil, resolver, dialer)
	sessions := svc.NewSessions(testPluginID)
	t.Cleanup(func() {
		sessions.Close()
		svc.Stop()
	})

	return sessions, dialer
}

func connectTo(t *testing.T, sessions *Sessions, host string) error {
	t.Helper()

	_, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     22,
		User:     "root",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})

	return err
}

// TestSSH_SSRF_CloudMetadataIsAlwaysBlocked: the metadata service hands out
// the credentials of the machine the panel runs on. It must stay unreachable
// whatever the operator relaxed.
func TestSSH_SSRF_CloudMetadataIsAlwaysBlocked(t *testing.T) {
	t.Parallel()
	metadataAddresses := []string{
		"169.254.169.254",
		"100.100.100.200",
		"fd00:ec2::254",
		"::ffff:169.254.169.254",
	}

	for _, blockPrivate := range []bool{true, false} {
		for _, address := range metadataAddresses {
			t.Run(address+"_block_private_"+boolName(blockPrivate), func(t *testing.T) {
				t.Parallel()
				cfg := Config{BlockPrivateIPs: blockPrivate, AllowedHosts: []string{"metadata.example.com"}}
				sessions, dialer := newPolicySessions(t, cfg, staticResolver{
					answers: map[string][]netip.Addr{"metadata.example.com": {netip.MustParseAddr(address)}},
				})

				err := connectTo(t, sessions, address)
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrDialBlocked)
				assert.Contains(t, err.Error(), "cloud_metadata")

				// Also through a name, and even one on the allow-list.
				err = connectTo(t, sessions, "metadata.example.com")
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrDialBlocked)

				assert.Empty(t, dialer.dialed, "a blocked target must never be dialed")
			})
		}
	}
}

func TestSSH_SSRF_PrivateAddressPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cfg         Config
		host        string
		wantBlocked bool
	}{
		{
			name:        "private_blocked_by_default_config",
			cfg:         Config{BlockPrivateIPs: true},
			host:        "10.0.0.5",
			wantBlocked: true,
		},
		{
			name:        "loopback_blocked",
			cfg:         Config{BlockPrivateIPs: true},
			host:        "127.0.0.1",
			wantBlocked: true,
		},
		{
			name:        "operator_may_allow_private_networks",
			cfg:         Config{BlockPrivateIPs: false},
			host:        "10.0.0.5",
			wantBlocked: false,
		},
		{
			name:        "public_address_is_allowed",
			cfg:         Config{BlockPrivateIPs: true},
			host:        "203.0.113.10",
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sessions, dialer := newPolicySessions(t, tt.cfg, staticResolver{})

			err := connectTo(t, sessions, tt.host)
			require.Error(t, err, "the recording dialer never completes a connection")

			if tt.wantBlocked {
				assert.ErrorIs(t, err, ErrDialBlocked)
				assert.Empty(t, dialer.dialed)

				return
			}

			assert.NotErrorIs(t, err, ErrDialBlocked)
			require.Len(t, dialer.dialed, 1)
			assert.Equal(t, net.JoinHostPort(tt.host, "22"), dialer.dialed[0])
		})
	}
}

// TestSSH_SSRF_AllowedHostsBypassesPrivateBlock lets an operator reach nodes on
// a private network without opening the panel to every private address.
func TestSSH_SSRF_AllowedHostsBypassesPrivateBlock(t *testing.T) {
	t.Parallel()
	cfg := Config{BlockPrivateIPs: true, AllowedHosts: []string{"NODE.internal"}}
	resolver := staticResolver{answers: map[string][]netip.Addr{
		"node.internal":  {netip.MustParseAddr("10.0.0.5")},
		"other.internal": {netip.MustParseAddr("10.0.0.6")},
	}}

	sessions, dialer := newPolicySessions(t, cfg, resolver)

	err := connectTo(t, sessions, "node.internal")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDialBlocked, "the allow-list is matched case-insensitively")
	require.Len(t, dialer.dialed, 1)
	assert.Equal(t, "10.0.0.5:22", dialer.dialed[0], "the resolved address is dialed as a literal")

	err = connectTo(t, sessions, "other.internal")
	assert.ErrorIs(t, err, ErrDialBlocked, "a host outside the list stays blocked")
}

// TestSSH_SSRF_MixedAnswerIsRejected: a name that resolves to a public and a
// private address at once is the classic way past a naive check, so any
// blocked answer refuses the whole connection.
func TestSSH_SSRF_MixedAnswerIsRejected(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{answers: map[string][]netip.Addr{
		"mixed.example.com": {
			netip.MustParseAddr("203.0.113.10"),
			netip.MustParseAddr("127.0.0.1"),
		},
	}}

	sessions, dialer := newPolicySessions(t, Config{BlockPrivateIPs: true}, resolver)

	err := connectTo(t, sessions, "mixed.example.com")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDialBlocked)
	assert.Empty(t, dialer.dialed)
}

// TestSSH_SSRF_UnresolvableHost keeps DNS failures distinguishable from policy
// refusals, so a plugin author is not sent hunting for the wrong problem.
func TestSSH_SSRF_UnresolvableHost(t *testing.T) {
	t.Parallel()
	sessions, dialer := newPolicySessions(t, Config{}, staticResolver{})

	err := connectTo(t, sessions, "nowhere.example.com")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHostNotResolved)
	assert.Empty(t, dialer.dialed)
}

// TestSSH_SSRF_ResolverErrorIsReported — API7:2023 Server Side Request Forgery.
// A lookup that errors must be as final as one that answers with nothing: the
// address policy has nothing to check, so there is no target to dial.
func TestSSH_SSRF_ResolverErrorIsReported(t *testing.T) {
	t.Parallel()
	resolver := staticResolver{err: errors.New("dns is unavailable")}
	sessions, dialer := newPolicySessions(t, Config{BlockPrivateIPs: true}, resolver)

	err := connectTo(t, sessions, "node.example.com")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHostNotResolved)
	assert.Contains(t, err.Error(), "dns is unavailable",
		"the resolver failure is kept so an operator can tell it from a policy refusal")
	assert.Empty(t, dialer.dialed, "a name that never resolved must not be dialed")
}

// TestSSH_SSRF_DefaultPortIsDialed — API7:2023 Server Side Request Forgery.
// The port a plugin left out is filled in by the panel, so omitting it cannot
// steer the connection anywhere the policy did not check.
func TestSSH_SSRF_DefaultPortIsDialed(t *testing.T) {
	t.Parallel()
	sessions, dialer := newPolicySessions(t, Config{BlockPrivateIPs: true}, staticResolver{})

	_, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     "203.0.113.10",
		User:     "root",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})

	require.Error(t, err, "the recording dialer never completes a connection")
	assert.NotErrorIs(t, err, ErrDialBlocked)
	require.Len(t, dialer.dialed, 1)
	assert.Equal(t, "203.0.113.10:22", dialer.dialed[0])
}

func TestSSH_HostKeyVerification(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	host, port := server.addr()

	otherPair, err := GenerateKeyPair(KeyTypeED25519, "")
	require.NoError(t, err)

	tests := []struct {
		name         string
		policy       func() HostKeyPolicy
		wantVerified bool
		wantErrorIs  error
		wantError    string
	}{
		{
			name:   "accept_any_trusts_first_contact",
			policy: func() HostKeyPolicy { return HostKeyPolicy{AcceptAny: true} },
		},
		{
			name: "pinned_fingerprint_matches",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{FingerprintsSHA256: []string{server.fingerprint}}
			},
			wantVerified: true,
		},
		{
			name: "pinned_fingerprint_without_prefix_matches",
			policy: func() HostKeyPolicy {
				bare := server.fingerprint[len("SHA256:"):]

				return HostKeyPolicy{FingerprintsSHA256: []string{bare}}
			},
			wantVerified: true,
		},
		{
			name: "pinned_public_key_matches",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{PublicKeys: []string{string(ssh.MarshalAuthorizedKey(server.hostKey.PublicKey()))}}
			},
			wantVerified: true,
		},
		{
			name: "wrong_fingerprint_is_rejected",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{FingerprintsSHA256: []string{otherPair.FingerprintSHA256}}
			},
			wantErrorIs: ErrHostKeyRejected,
			wantError:   "host key verification failed",
		},
		{
			name: "wrong_public_key_is_rejected",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{PublicKeys: []string{otherPair.PublicKey}}
			},
			wantErrorIs: ErrHostKeyRejected,
			wantError:   "host key verification failed",
		},
		{
			name:        "no_policy_is_refused",
			policy:      func() HostKeyPolicy { return HostKeyPolicy{} },
			wantErrorIs: ErrHostKeyPolicyRequired,
			wantError:   "set accept_any or pin a fingerprint",
		},
		{
			name: "unparsable_pinned_public_key_is_refused",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{PublicKeys: []string{"ssh-ed25519 not-a-base64-blob"}}
			},
			wantErrorIs: ErrHostKeyInvalid,
			wantError:   "invalid host public key",
		},
		{
			name: "accept_any_combined_with_pins_is_refused",
			policy: func() HostKeyPolicy {
				// The natural shape of a plugin that kept accept_any from its
				// template and later added a pin, believing pins are checked:
				// accept_any would skip them, so the combination is an error.
				return HostKeyPolicy{AcceptAny: true, FingerprintsSHA256: []string{server.fingerprint}}
			},
			wantErrorIs: ErrHostKeyPolicyConflict,
			wantError:   "accept_any cannot be combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sessions := newTestSessions(t, Config{})

			result, err := sessions.Connect(context.Background(), ConnectParams{
				Host:     host,
				Port:     port,
				User:     "gameap",
				Password: testPassword,
				HostKey:  tt.policy(),
			})

			if tt.wantErrorIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrorIs)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, result, "a refused host key must not hand out a handle")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, server.fingerprint, result.HostKeyFingerprintSHA256)
			assert.Equal(t, tt.wantVerified, result.HostKeyVerified,
				"the audit trail keys off whether the key was actually checked")
		})
	}
}

// TestSSH_AcceptAnyDisabledByTheOperator: PLUGINS_SSH_ALLOW_ACCEPT_ANY_HOST_KEY
// is the operator's lever against unverified connections; with it off,
// accept_any must be refused before anything is dialed while pinned policies
// keep working (API8:2023 Security Misconfiguration).
func TestSSH_AcceptAnyDisabledByTheOperator(t *testing.T) {
	t.Parallel()

	t.Run("accept_any_is_refused_before_dialing", func(t *testing.T) {
		t.Parallel()

		cfg := Config{BlockPrivateIPs: false, DisallowAcceptAnyHostKey: true}
		sessions, dialer := newPolicySessions(t, cfg, staticResolver{})

		err := connectTo(t, sessions, "8.8.8.8")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrHostKeyAcceptAnyDisabled)
		assert.Empty(t, dialer.dialed, "a refused policy must never open a socket")
	})

	t.Run("pinned_fingerprint_still_connects", func(t *testing.T) {
		t.Parallel()

		server := newTestSSHServer(t)
		sessions := newTestSessions(t, Config{DisallowAcceptAnyHostKey: true})
		host, port := server.addr()

		result, err := sessions.Connect(context.Background(), ConnectParams{
			Host:     host,
			Port:     port,
			User:     "gameap",
			Password: testPassword,
			HostKey:  HostKeyPolicy{FingerprintsSHA256: []string{server.fingerprint}},
		})

		require.NoError(t, err)
		assert.True(t, result.HostKeyVerified)
	})
}

// TestSSH_HostKeyRejectionNamesTheObservedKey: after a mismatch the plugin has
// to be able to tell an operator what actually answered.
func TestSSH_HostKeyRejectionNamesTheObservedKey(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	_, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     port,
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{FingerprintsSHA256: []string{"SHA256:definitelyNotTheKey"}},
	})

	require.Error(t, err)

	var rejected *HostKeyRejectedError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, server.fingerprint, rejected.FingerprintSHA256)
	assert.Equal(t, "ssh-ed25519", rejected.KeyType)
}

// TestSSH_HostKeyInvalidPinIsRefusedBeforeDialing — API8:2023 Security
// Misconfiguration. A pin the panel cannot parse is a policy it cannot
// enforce, so the connection has to stop before a socket exists rather than
// quietly degrade to trusting whatever answers.
func TestSSH_HostKeyInvalidPinIsRefusedBeforeDialing(t *testing.T) {
	t.Parallel()
	sessions, dialer := newPolicySessions(t, Config{}, staticResolver{})

	_, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     "203.0.113.10",
		Port:     22,
		User:     "root",
		Password: testPassword,
		HostKey:  HostKeyPolicy{PublicKeys: []string{"definitely not an authorized_keys line"}},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHostKeyInvalid)
	assert.Empty(t, dialer.dialed, "an unenforceable host key policy must never reach the network")
}

// TestSSH_PaddedFingerprintPinMatches — API8:2023 Security Misconfiguration.
// Operators paste fingerprints from whatever tool printed them, padding and
// stray whitespace included. A pin that silently fails to match pushes them
// back to accept-any, which is the configuration this policy exists to avoid.
func TestSSH_PaddedFingerprintPinMatches(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     port,
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{FingerprintsSHA256: []string{"  " + server.fingerprint + "=  "}},
	})

	require.NoError(t, err)
	assert.Equal(t, server.fingerprint, result.HostKeyFingerprintSHA256)
}

// TestSSH_ForeignHandleIsUnknown: handles live in the session set of one
// plugin, so a handle leaked to another plugin resolves to nothing.
func TestSSH_ForeignHandleIsUnknown(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)

	svc := newService(nil, nil, Config{}, nil, staticResolver{}, realDialer{})
	t.Cleanup(svc.Stop)

	victim := svc.NewSessions(1)
	attacker := svc.NewSessions(2)

	handle := connectToTestServer(t, victim, server)

	_, err := attacker.StartExec(context.Background(), ExecParams{Handle: handle, Command: "echo hi"})
	assert.ErrorIs(t, err, ErrConnectionNotFound)

	assert.ErrorIs(t, attacker.Disconnect(handle), ErrConnectionNotFound)
}

// TestSSH_ConnectTimeoutIsBounded: an unresponsive machine must not hold the
// guest call past its deadline, which would kill the plugin's wasm module.
func TestSSH_ConnectTimeoutIsBounded(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	// Accept and then stay silent: the TCP connect succeeds, the SSH handshake
	// never completes.
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		<-time.After(30 * time.Second)
		_ = conn.Close()
	}()

	sessions := newTestSessions(t, Config{ConnectTimeout: 300 * time.Millisecond})
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	start := time.Now()
	_, err = sessions.Connect(context.Background(), ConnectParams{
		Host:     "127.0.0.1",
		Port:     uint32(tcpAddr.Port),
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConnectTimeout)
	assert.Less(t, time.Since(start), 5*time.Second, "the handshake must honour the connect budget")
}

// frozenDeadlineConn ignores the deadline dialSSH sets, so the one already on
// the socket stands.
type frozenDeadlineConn struct {
	net.Conn
}

func (frozenDeadlineConn) SetDeadline(_ time.Time) error { return nil }

// expiredDeadlineDialer hands back a socket whose read budget has already run
// out: the handshake then fails with a net timeout while the context is still
// alive, which is the ordering a loaded machine produces.
type expiredDeadlineDialer struct{}

func (expiredDeadlineDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(-time.Second)); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return frozenDeadlineConn{Conn: conn}, nil
}

// TestSSH_ConnectTimeoutSurvivesSocketDeadline: the connect budget is enforced
// both on the socket and on the context, and either one can trip first. A
// plugin distinguishes an unreachable machine from a rejected one by the
// sentinel, so the socket deadline must not leak the raw "i/o timeout" out.
func TestSSH_ConnectTimeoutSurvivesSocketDeadline(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)

	svc := newService(nil, nil, Config{ConnectTimeout: time.Minute}, nil, staticResolver{}, expiredDeadlineDialer{})
	t.Cleanup(svc.Stop)

	sessions := svc.NewSessions(testPluginID)
	t.Cleanup(sessions.Close)

	host, port := server.addr()

	_, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     port,
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConnectTimeout)
}

func boolName(v bool) string {
	if v {
		return "on"
	}

	return "off"
}

// failingDialer answers every dial with a staged error, so the tests can pin
// how the engine classifies transport failures.
type failingDialer struct {
	err error
}

func (d *failingDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, d.err
}

// timeoutNetError mimics the OS-level "i/o timeout" the dialer produces.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "dial tcp 203.0.113.9:22: i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return false }

// TestSSH_DialFailureClassification: a dial failure reaches the plugin as a
// coarse sentinel it can act on, never as the raw OS text — echoing
// "refused" vs "no route" details would let a plugin port-scan through the
// panel's network position (API7:2023 Server Side Request Forgery).
func TestSSH_DialFailureClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialErr error
		want    error
	}{
		{
			name:    "timeout_flavored_errors_map_to_connect_timeout",
			dialErr: &net.OpError{Op: "dial", Net: "tcp", Err: timeoutNetError{}},
			want:    ErrConnectTimeout,
		},
		{
			name:    "refused_connections_map_to_connect_refused",
			dialErr: &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)},
			want:    ErrConnectRefused,
		},
		{
			name:    "other_transport_errors_map_to_dial_failed",
			dialErr: errors.New("raw-os-detail: no route to host 203.0.113.9"),
			want:    ErrDialFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := newService(nil, nil, Config{BlockPrivateIPs: false}, nil, staticResolver{}, &failingDialer{err: tt.dialErr})
			sessions := svc.NewSessions(testPluginID)
			t.Cleanup(func() {
				sessions.Close()
				svc.Stop()
			})

			err := connectTo(t, sessions, "8.8.8.8")

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
			// The sentinel text is the whole answer: no OS detail leaks out.
			assert.EqualError(t, err, tt.want.Error())
		})
	}
}

// TestSSH_SSRF_HostIsTrimmedBeforeResolutionAndAllowlist: the allow-list
// comparison trims and lowercases its side, so the same normalization must be
// applied before resolution — otherwise " node.internal " passes the list but
// resolves as a different name (API7:2023 Server Side Request Forgery).
func TestSSH_SSRF_HostIsTrimmedBeforeResolutionAndAllowlist(t *testing.T) {
	t.Parallel()

	cfg := Config{BlockPrivateIPs: true, AllowedHosts: []string{"node.internal"}}
	sessions, dialer := newPolicySessions(t, cfg, staticResolver{
		answers: map[string][]netip.Addr{"node.internal": {netip.MustParseAddr("10.10.0.5")}},
	})

	err := connectTo(t, sessions, " node.internal ")

	// The recording dialer refuses every dial; what matters is that the
	// blocked-address policy and the resolver both saw the trimmed name and
	// the dial was attempted exactly once.
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDialBlocked)
	assert.NotErrorIs(t, err, ErrHostNotResolved)
	require.Len(t, dialer.dialed, 1)
	assert.Equal(t, "10.10.0.5:22", dialer.dialed[0])
}
