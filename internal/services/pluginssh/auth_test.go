package pluginssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const authTestPassphrase = "unlock-me"

// authTestBrokenPEM looks like a key file to anything that only checks the
// envelope, which is what a plugin pasting a truncated key produces.
const authTestBrokenPEM = "-----BEGIN OPENSSH PRIVATE KEY-----\nbm90IGEga2V5\n-----END OPENSSH PRIVATE KEY-----\n"

// authTestEncryptedKey builds a passphrase-protected key the way ssh-keygen
// does, plus the fingerprint of the identity hiding inside it.
func authTestEncryptedKey(t *testing.T, passphrase string) (pemData, fingerprint string) {
	t.Helper()

	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte(passphrase))
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(private)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(block)), ssh.FingerprintSHA256(signer.PublicKey())
}

// authTestMethodKind names an auth method by the constructor that produced it.
// x/crypto keeps the concrete types unexported, so identity is decided against
// a reference value from the same constructor rather than a hardcoded name.
func authTestMethodKind(method ssh.AuthMethod) string {
	switch fmt.Sprintf("%T", method) {
	case fmt.Sprintf("%T", ssh.PublicKeys()):
		return "publickey"
	case fmt.Sprintf("%T", ssh.Password("")):
		return "password"
	case fmt.Sprintf("%T", ssh.KeyboardInteractive(nil)):
		return "keyboard-interactive"
	default:
		return "unknown"
	}
}

func authTestConnectionCount(sessions *Sessions) int {
	sessions.mu.Lock()
	defer sessions.mu.Unlock()

	return len(sessions.conns)
}

// TestBuildAuthMethods pins what the panel offers a server and in which order.
// The order is what the server sees: a key that is present must be tried before
// the password, and a key that cannot be read has to surface as an error rather
// than quietly downgrading the connection to password auth.
func TestBuildAuthMethods(t *testing.T) {
	t.Parallel()

	pair, err := GenerateKeyPair(KeyTypeED25519, "")
	require.NoError(t, err)

	encryptedPEM, _ := authTestEncryptedKey(t, authTestPassphrase)

	tests := []struct {
		name      string
		params    ConnectParams
		wantKinds []string
		wantError error
	}{
		{
			name:      "no_credentials_at_all_are_refused",
			params:    ConnectParams{Host: "example.com", User: "gameap"},
			wantError: ErrAuthRequired,
		},
		{
			name:      "a_password_also_offers_the_keyboard_interactive_fallback",
			params:    ConnectParams{Password: testPassword},
			wantKinds: []string{"password", "keyboard-interactive"},
		},
		{
			name:      "a_key_alone_is_the_only_method_offered",
			params:    ConnectParams{PrivateKeyPEM: pair.PrivateKeyPEM},
			wantKinds: []string{"publickey"},
		},
		{
			name:      "an_encrypted_key_is_unlocked_with_the_passphrase",
			params:    ConnectParams{PrivateKeyPEM: encryptedPEM, Passphrase: authTestPassphrase},
			wantKinds: []string{"publickey"},
		},
		{
			name: "the_key_is_offered_before_the_password",
			params: ConnectParams{
				PrivateKeyPEM: pair.PrivateKeyPEM,
				Password:      testPassword,
			},
			wantKinds: []string{"publickey", "password", "keyboard-interactive"},
		},
		{
			name: "a_broken_key_is_reported_instead_of_falling_back_to_the_password",
			params: ConnectParams{
				PrivateKeyPEM: authTestBrokenPEM,
				Password:      testPassword,
			},
			wantError: ErrInvalidPrivateKey,
		},
		{
			name: "a_locked_key_is_reported_instead_of_falling_back_to_the_password",
			params: ConnectParams{
				PrivateKeyPEM: encryptedPEM,
				Password:      testPassword,
			},
			wantError: ErrPassphraseRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE, ACT
			methods, err := buildAuthMethods(tt.params)

			// ASSERT
			if tt.wantError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantError)
				assert.Nil(t, methods, "credentials that could not be built must not be offered in part")

				return
			}

			require.NoError(t, err)
			require.Len(t, methods, len(tt.wantKinds))

			kinds := make([]string, 0, len(methods))
			for _, method := range methods {
				kinds = append(kinds, authTestMethodKind(method))
			}

			assert.Equal(t, tt.wantKinds, kinds, "the offer order is what the server walks through")
		})
	}
}

// TestParsePrivateKey separates the two failures a plugin has to tell apart: a
// key the panel cannot read at all, and a key that is fine but locked. Only the
// second one is worth retrying with a passphrase.
func TestParsePrivateKey(t *testing.T) {
	t.Parallel()

	pair, err := GenerateKeyPair(KeyTypeED25519, "panel@gameap")
	require.NoError(t, err)

	encryptedPEM, encryptedFingerprint := authTestEncryptedKey(t, authTestPassphrase)

	tests := []struct {
		name            string
		pemData         string
		passphrase      string
		wantFingerprint string
		wantError       error
	}{
		{
			name:            "a_plain_key_is_parsed",
			pemData:         pair.PrivateKeyPEM,
			wantFingerprint: pair.FingerprintSHA256,
		},
		{
			name:      "text_that_is_not_a_key_is_rejected",
			pemData:   "definitely not a pem block",
			wantError: ErrInvalidPrivateKey,
		},
		{
			name:      "an_empty_key_is_rejected",
			pemData:   "",
			wantError: ErrInvalidPrivateKey,
		},
		{
			name:      "a_pem_envelope_around_garbage_is_rejected",
			pemData:   authTestBrokenPEM,
			wantError: ErrInvalidPrivateKey,
		},
		{
			name:      "an_encrypted_key_without_a_passphrase_asks_for_one",
			pemData:   encryptedPEM,
			wantError: ErrPassphraseRequired,
		},
		{
			name:            "an_encrypted_key_with_the_right_passphrase_keeps_its_identity",
			pemData:         encryptedPEM,
			passphrase:      authTestPassphrase,
			wantFingerprint: encryptedFingerprint,
		},
		{
			name:       "an_encrypted_key_with_a_wrong_passphrase_is_rejected",
			pemData:    encryptedPEM,
			passphrase: "not-it",
			wantError:  ErrInvalidPrivateKey,
		},
		{
			name:       "text_that_is_not_a_key_is_rejected_with_a_passphrase_too",
			pemData:    "definitely not a pem block",
			passphrase: authTestPassphrase,
			wantError:  ErrInvalidPrivateKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE, ACT
			signer, err := parsePrivateKey(tt.pemData, tt.passphrase)

			// ASSERT
			if tt.wantError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantError)
				assert.Nil(t, signer)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, signer)
			assert.Equal(t, tt.wantFingerprint, ssh.FingerprintSHA256(signer.PublicKey()),
				"the signer must present the identity that was handed in, whatever the key was wrapped in")
		})
	}
}

// TestSessions_KeyboardInteractiveAuthentication: many sshd setups answer a
// password attempt with a PAM challenge instead of accepting the password
// directly. The panel has to walk that challenge, otherwise a plugin holding
// perfectly valid credentials cannot reach the machine at all.
func TestSessions_KeyboardInteractiveAuthentication(t *testing.T) {
	t.Parallel()
	// ARRANGE
	server := newTestSSHServer(t, withKeyboardInteractive())
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	// ACT
	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:     host,
		Port:     port,
		User:     "gameap",
		Password: testPassword,
		HostKey:  HostKeyPolicy{AcceptAny: true},
	})

	// ASSERT
	require.NoError(t, err, "the server refuses plain password auth, only the challenge gets in")
	require.NotZero(t, result.Handle)

	snapshot := runToCompletion(t, sessions, ExecParams{Handle: result.Handle, Command: "echo ok"})
	assert.Equal(t, "ok", string(snapshot.Stdout), "the connection must be usable, not merely established")
}

// TestSessions_AuthenticationRejected covers OWASP API Security Top 10:2023 —
// API2:2023 Broken Authentication. Credentials the server turned down must end
// the attempt: no handle is registered, so nothing can be run over a connection
// that was never authenticated, and the slot stays free for a valid login.
func TestSessions_AuthenticationRejected(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	host, port := server.addr()

	unauthorized, err := GenerateKeyPair(KeyTypeED25519, "not-in-authorized-keys")
	require.NoError(t, err)

	tests := []struct {
		name   string
		params ConnectParams
	}{
		{
			name: "a_wrong_password_is_refused",
			params: ConnectParams{
				Host:     host,
				Port:     port,
				User:     "gameap",
				Password: "not-" + testPassword,
				HostKey:  HostKeyPolicy{AcceptAny: true},
			},
		},
		{
			name: "a_key_that_is_not_authorized_is_refused",
			params: ConnectParams{
				Host:          host,
				Port:          port,
				User:          "gameap",
				PrivateKeyPEM: unauthorized.PrivateKeyPEM,
				HostKey:       HostKeyPolicy{AcceptAny: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			sessions := newTestSessions(t, Config{})

			// ACT
			result, err := sessions.Connect(context.Background(), tt.params)

			// ASSERT
			require.Error(t, err)
			assert.Nil(t, result, "a refused login must not hand out a handle")
			assert.Contains(t, err.Error(), "unable to authenticate",
				"the plugin has to see that the credentials were refused, not that the host was unreachable")
			assert.NotErrorIs(t, err, ErrHostKeyRejected, "the host key was accepted, the credentials were not")
			assert.Zero(t, authTestConnectionCount(sessions), "nothing may be left behind to run commands over")

			handle := connectToTestServer(t, sessions, server)
			assert.NotZero(t, handle, "a refused attempt must not consume the connection slot")
		})
	}
}
