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
	"testing"
	"time"

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

func TestSSH_HostKeyVerification(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	host, port := server.addr()

	otherPair, err := GenerateKeyPair(KeyTypeED25519, "")
	require.NoError(t, err)

	tests := []struct {
		name        string
		policy      func() HostKeyPolicy
		wantErr     bool
		wantErrorIs error
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
		},
		{
			name: "pinned_fingerprint_without_prefix_matches",
			policy: func() HostKeyPolicy {
				bare := server.fingerprint[len("SHA256:"):]

				return HostKeyPolicy{FingerprintsSHA256: []string{bare}}
			},
		},
		{
			name: "pinned_public_key_matches",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{PublicKeys: []string{string(ssh.MarshalAuthorizedKey(server.hostKey.PublicKey()))}}
			},
		},
		{
			name: "wrong_fingerprint_is_rejected",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{FingerprintsSHA256: []string{otherPair.FingerprintSHA256}}
			},
			wantErr:     true,
			wantErrorIs: ErrHostKeyRejected,
		},
		{
			name: "wrong_public_key_is_rejected",
			policy: func() HostKeyPolicy {
				return HostKeyPolicy{PublicKeys: []string{otherPair.PublicKey}}
			},
			wantErr:     true,
			wantErrorIs: ErrHostKeyRejected,
		},
		{
			name:        "no_policy_is_refused",
			policy:      func() HostKeyPolicy { return HostKeyPolicy{} },
			wantErr:     true,
			wantErrorIs: ErrHostKeyPolicyRequired,
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

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrorIs)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, server.fingerprint, result.HostKeyFingerprintSHA256)
		})
	}
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

func boolName(v bool) string {
	if v {
		return "on"
	}

	return "off"
}
