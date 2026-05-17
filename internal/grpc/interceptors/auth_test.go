// OWASP API Top 10:2023 — API2:2023 Broken Authentication.
// These tests verify that the gRPC auth interceptor correctly enforces
// mTLS, validates API keys, and rejects requests with missing/invalid
// credentials before the handler is invoked.
package interceptors

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func ctxWithMD(pairs ...string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(pairs...))
}

func makeSelfSignedCert(t *testing.T, cn string) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return cert
}

func ctxWithMTLSCert(cert *x509.Certificate) context.Context {
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	p := &peer.Peer{
		Addr:     &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)},
		AuthInfo: credentials.TLSInfo{State: state},
	}

	return peer.NewContext(context.Background(), p)
}

func ctxWithBareTLSPeer() context.Context {
	p := &peer.Peer{
		Addr:     &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)},
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{}},
	}

	return peer.NewContext(context.Background(), p)
}

func ctxWithNonTLSPeer() context.Context {
	p := &peer.Peer{Addr: &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}, AuthInfo: nil}

	return peer.NewContext(context.Background(), p)
}

type fakeNodeRepo struct {
	nodes              []domain.Node
	err                error
	calls              int
	lastFindNodeFilter *filters.FindNode
}

func (f *fakeNodeRepo) Find(
	_ context.Context,
	find *filters.FindNode,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Node, error) {
	f.calls++
	if find != nil {
		filterCopy := *find
		filterCopy.IDs = append([]uint(nil), find.IDs...)
		f.lastFindNodeFilter = &filterCopy
	}

	return f.nodes, f.err
}

func (f *fakeNodeRepo) FindAll(
	_ context.Context,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) Save(_ context.Context, _ *domain.Node) error {
	return nil
}

func (f *fakeNodeRepo) UpdateGDaemonAPIToken(_ context.Context, _ uint, _ string, _ time.Time) error {
	return nil
}

func (f *fakeNodeRepo) Delete(_ context.Context, _ uint) error {
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// OWASP API Top 10:2023 — API2:2023 Broken Authentication.
func TestAuthInterceptor_UnaryServerInterceptor(t *testing.T) {
	cert := makeSelfSignedCert(t, "test-client")

	okHandler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/SomeMethod"}
	enrollInfo := &grpc.UnaryServerInfo{FullMethod: enrollFullMethod}

	tests := []struct {
		name          string
		setupRepo     func() *fakeNodeRepo
		ctx           context.Context
		info          *grpc.UnaryServerInfo
		requireMTLS   bool
		wantCode      codes.Code
		wantResp      any
		wantError     string
		wantRepoCalls int
		wantFindIDs   []uint
	}{
		{
			name:          "enroll_method_bypasses_auth",
			setupRepo:     func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:           context.Background(),
			info:          enrollInfo,
			requireMTLS:   true,
			wantCode:      codes.OK,
			wantResp:      "ok",
			wantRepoCalls: 0,
		},
		{
			name:          "no_metadata_no_mtls_required_passes",
			setupRepo:     func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:           context.Background(),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.OK,
			wantResp:      "ok",
			wantRepoCalls: 0,
		},
		{
			name:        "mtls_required_no_peer_returns_unauthenticated",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         context.Background(),
			info:        info,
			requireMTLS: true,
			wantCode:    codes.Unauthenticated,
			wantError:   "no peer information",
		},
		{
			name:        "mtls_required_no_authinfo_returns_unauthenticated",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         ctxWithNonTLSPeer(),
			info:        info,
			requireMTLS: true,
			wantCode:    codes.Unauthenticated,
			wantError:   "no TLS info",
		},
		{
			name:        "mtls_required_no_certs_returns_unauthenticated",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         ctxWithBareTLSPeer(),
			info:        info,
			requireMTLS: true,
			wantCode:    codes.Unauthenticated,
			wantError:   "no client certificate",
		},
		{
			name:          "mtls_required_with_cert_passes",
			setupRepo:     func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:           ctxWithMTLSCert(cert),
			info:          info,
			requireMTLS:   true,
			wantCode:      codes.OK,
			wantResp:      "ok",
			wantRepoCalls: 0,
		},
		{
			name:        "apikey_metadata_without_node_id_returns_invalid_argument",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         ctxWithMD("x-api-key", "abc"),
			info:        info,
			requireMTLS: false,
			wantCode:    codes.InvalidArgument,
			wantError:   "node ID required with API key",
		},
		{
			name:        "apikey_with_non_numeric_node_id_returns_invalid_argument",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         ctxWithMD("x-api-key", "abc", "x-node-id", "abc"),
			info:        info,
			requireMTLS: false,
			wantCode:    codes.InvalidArgument,
			wantError:   "invalid node ID format",
		},
		{
			name:        "apikey_with_empty_node_id_returns_invalid_argument",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         ctxWithMD("x-api-key", "abc", "x-node-id", ""),
			info:        info,
			requireMTLS: false,
			wantCode:    codes.InvalidArgument,
			wantError:   "invalid node ID format",
		},
		{
			name:        "apikey_with_overflow_node_id_returns_invalid_argument",
			setupRepo:   func() *fakeNodeRepo { return &fakeNodeRepo{} },
			ctx:         ctxWithMD("x-api-key", "abc", "x-node-id", "18446744073709551616"),
			info:        info,
			requireMTLS: false,
			wantCode:    codes.InvalidArgument,
			wantError:   "invalid node ID format",
		},
		{
			name:          "apikey_node_not_found_returns_not_found",
			setupRepo:     func() *fakeNodeRepo { return &fakeNodeRepo{nodes: nil} },
			ctx:           ctxWithMD("x-api-key", "abc", "x-node-id", "1"),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.NotFound,
			wantError:     "node not found",
			wantRepoCalls: 1,
			wantFindIDs:   []uint{1},
		},
		{
			name: "apikey_node_disabled_returns_permission_denied",
			setupRepo: func() *fakeNodeRepo {
				return &fakeNodeRepo{
					nodes: []domain.Node{{ID: 1, Enabled: false, GdaemonAPIKey: "abc"}},
				}
			},
			ctx:           ctxWithMD("x-api-key", "abc", "x-node-id", "1"),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.PermissionDenied,
			wantError:     "node is disabled",
			wantRepoCalls: 1,
			wantFindIDs:   []uint{1},
		},
		{
			// Store the hash of the real key; a different presented plaintext
			// hashes to a different digest and must be rejected.
			name: "apikey_mismatch_returns_unauthenticated",
			setupRepo: func() *fakeNodeRepo {
				return &fakeNodeRepo{
					nodes: []domain.Node{
						{ID: 1, Enabled: true, GdaemonAPIKey: pkgstrings.SHA256("correct")},
					},
				}
			},
			ctx:           ctxWithMD("x-api-key", "abc", "x-node-id", "1"),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.Unauthenticated,
			wantError:     "invalid API key",
			wantRepoCalls: 1,
			wantFindIDs:   []uint{1},
		},
		{
			// Hashed-at-rest happy path: presenting the plaintext whose SHA-256
			// equals the stored digest authenticates and reaches the handler.
			name: "apikey_matches_stored_hash_passes",
			setupRepo: func() *fakeNodeRepo {
				return &fakeNodeRepo{
					nodes: []domain.Node{
						{ID: 1, Enabled: true, GdaemonAPIKey: pkgstrings.SHA256("plain-key")},
					},
				}
			},
			ctx:           ctxWithMD("x-api-key", "plain-key", "x-node-id", "1"),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.OK,
			wantResp:      "ok",
			wantRepoCalls: 1,
			wantFindIDs:   []uint{1},
		},
		{
			// Defense-in-depth: a legacy plaintext value left in the column is
			// not a usable credential — the presented value is hashed before
			// comparison, so presenting that same plaintext is rejected.
			name: "legacy_plaintext_key_at_rest_is_rejected",
			setupRepo: func() *fakeNodeRepo {
				return &fakeNodeRepo{
					nodes: []domain.Node{{ID: 1, Enabled: true, GdaemonAPIKey: "plain-key"}},
				}
			},
			ctx:           ctxWithMD("x-api-key", "plain-key", "x-node-id", "1"),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.Unauthenticated,
			wantError:     "invalid API key",
			wantRepoCalls: 1,
			wantFindIDs:   []uint{1},
		},
		{
			name: "apikey_repo_error_returns_internal",
			setupRepo: func() *fakeNodeRepo {
				return &fakeNodeRepo{err: errors.New("db down")}
			},
			ctx:           ctxWithMD("x-api-key", "abc", "x-node-id", "1"),
			info:          info,
			requireMTLS:   false,
			wantCode:      codes.Internal,
			wantError:     "failed to verify node",
			wantRepoCalls: 1,
			wantFindIDs:   []uint{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := tt.setupRepo()
			interceptor := NewAuthInterceptor(repo, tt.requireMTLS, discardLogger())

			// ACT
			resp, err := interceptor.UnaryServerInterceptor()(tt.ctx, "req", tt.info, okHandler)

			// ASSERT
			assert.Equal(t, tt.wantCode, status.Code(err), "expected gRPC code mismatch")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, resp, "no response on error")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantResp, resp, "handler response mismatch")
			}
			assert.Equal(t, tt.wantRepoCalls, repo.calls, "repo Find call count mismatch")
			if tt.wantRepoCalls == 0 {
				assert.Nil(t, repo.lastFindNodeFilter, "repo filter must stay nil when Find is not called")
			} else {
				require.NotNil(t, repo.lastFindNodeFilter)
				if tt.wantFindIDs != nil {
					assert.Equal(t, tt.wantFindIDs, repo.lastFindNodeFilter.IDs)
				}
			}
		})
	}
}

// TestAuthInterceptor_UnaryServerInterceptor_SetsNodeIDInContext —
// OWASP API Top 10:2023 API2:2023 Broken Authentication. The interceptor
// hashes the presented x-api-key and constant-time compares it against the
// at-rest digest (security review findings #4/#6); the fixture therefore
// stores SHA-256(plaintext) while the metadata carries the plaintext.
func TestAuthInterceptor_UnaryServerInterceptor_SetsNodeIDInContext(t *testing.T) {
	// ARRANGE
	repo := &fakeNodeRepo{
		nodes: []domain.Node{{ID: 7, Enabled: true, GdaemonAPIKey: pkgstrings.SHA256("secret")}},
	}
	interceptor := NewAuthInterceptor(repo, false, discardLogger())

	var seenNodeID uint64
	var seenOK bool
	handler := func(ctx context.Context, _ any) (any, error) {
		seenNodeID, seenOK = GetNodeIDFromContext(ctx)

		return "ok", nil
	}

	ctx := ctxWithMD("x-api-key", "secret", "x-node-id", "7")
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/SomeMethod"}

	// ACT
	resp, err := interceptor.UnaryServerInterceptor()(ctx, "req", info, handler)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, seenOK, "node id should be present in context")
	assert.Equal(t, uint64(7), seenNodeID, "node id should be extracted from x-node-id metadata")
	assert.Equal(t, 1, repo.calls)
}

type stubStream struct {
	grpc.ServerStream

	ctx context.Context
}

func (s *stubStream) Context() context.Context { return s.ctx }

// OWASP API Top 10:2023 — API2:2023 Broken Authentication.
func TestAuthInterceptor_StreamServerInterceptor(t *testing.T) {
	cert := makeSelfSignedCert(t, "test-client")

	info := &grpc.StreamServerInfo{FullMethod: "/gameap.DaemonGateway/SomeStream"}

	tests := []struct {
		name        string
		ctx         context.Context
		requireMTLS bool
		wantCode    codes.Code
		wantError   string
		wantHandled bool
	}{
		{
			name:        "stream_no_mtls_required_passes",
			ctx:         context.Background(),
			requireMTLS: false,
			wantCode:    codes.OK,
			wantHandled: true,
		},
		{
			name:        "stream_mtls_required_no_peer_returns_unauthenticated",
			ctx:         context.Background(),
			requireMTLS: true,
			wantCode:    codes.Unauthenticated,
			wantError:   "no peer information",
			wantHandled: false,
		},
		{
			name:        "stream_mtls_required_with_cert_passes",
			ctx:         ctxWithMTLSCert(cert),
			requireMTLS: true,
			wantCode:    codes.OK,
			wantHandled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			interceptor := NewAuthInterceptor(&fakeNodeRepo{}, tt.requireMTLS, discardLogger())
			ss := &stubStream{ctx: tt.ctx}
			handled := false
			streamHandler := func(_ any, _ grpc.ServerStream) error {
				handled = true

				return nil
			}

			// ACT
			err := interceptor.StreamServerInterceptor()(nil, ss, info, streamHandler)

			// ASSERT
			assert.Equal(t, tt.wantCode, status.Code(err), "expected gRPC code mismatch")
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantHandled, handled, "stream handler invocation mismatch")
		})
	}
}

func TestGetNodeIDFromContext(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		wantID    uint64
		wantFound bool
	}{
		{
			name:      "returns_value_when_set",
			ctx:       context.WithValue(context.Background(), NodeIDKey, uint64(42)),
			wantID:    42,
			wantFound: true,
		},
		{
			name:      "returns_false_when_missing",
			ctx:       context.Background(),
			wantID:    0,
			wantFound: false,
		},
		{
			name:      "returns_false_for_wrong_type",
			ctx:       context.WithValue(context.Background(), NodeIDKey, "string"),
			wantID:    0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			gotID, gotOK := GetNodeIDFromContext(tt.ctx)

			// ASSERT
			assert.Equal(t, tt.wantID, gotID, "node id mismatch")
			assert.Equal(t, tt.wantFound, gotOK, "found flag mismatch")
		})
	}
}

func TestGetNodeFromContext(t *testing.T) {
	node := &domain.Node{ID: 99}

	tests := []struct {
		name      string
		ctx       context.Context
		wantNode  *domain.Node
		wantFound bool
	}{
		{
			name:      "returns_value_when_set",
			ctx:       context.WithValue(context.Background(), NodeKey, node),
			wantNode:  node,
			wantFound: true,
		},
		{
			name:      "returns_false_when_missing",
			ctx:       context.Background(),
			wantNode:  nil,
			wantFound: false,
		},
		{
			name:      "returns_false_for_wrong_type",
			ctx:       context.WithValue(context.Background(), NodeKey, "string"),
			wantNode:  nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			gotNode, gotOK := GetNodeFromContext(tt.ctx)

			// ASSERT
			assert.Equal(t, tt.wantNode, gotNode, "node mismatch")
			assert.Equal(t, tt.wantFound, gotOK, "found flag mismatch")
		})
	}
}

func TestSecureCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "equal_strings_return_true", a: "abcdef", b: "abcdef", want: true},
		{name: "different_strings_return_false", a: "abcdef", b: "abcxyz", want: false},
		{name: "different_lengths_return_false", a: "abc", b: "abcdef", want: false},
		{name: "empty_strings_return_true", a: "", b: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got := secureCompare(tt.a, tt.b)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}
