package gateway

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestService_validateAuth(t *testing.T) {
	tests := []struct {
		name      string
		setupNode func(*testing.T, *serviceDeps)
		setupAuth func(*serviceDeps)
		req       *proto.RegisterRequest
		wantCode  codes.Code
		wantError string
	}{
		{
			name: "valid_apikey_with_active_node_returns_nil",
			setupNode: func(t *testing.T, d *serviceDeps) {
				t.Helper()
				require.NoError(t, d.nodeRepo.Save(context.Background(), &domain.Node{
					Enabled:       true,
					Name:          "node1",
					GdaemonAPIKey: "secret-key",
				}))
			},
			setupAuth: func(d *serviceDeps) {
				d.apiKeyVerifier.valid = map[string]uint64{"secret-key": 1}
			},
			req: &proto.RegisterRequest{
				NodeId: 1,
				ApiKey: "secret-key",
			},
		},
		{
			name: "node_not_found_returns_not_found",
			setupNode: func(_ *testing.T, _ *serviceDeps) {
				// no nodes saved
			},
			setupAuth: func(d *serviceDeps) {
				d.apiKeyVerifier.valid = map[string]uint64{"key": 1}
			},
			req: &proto.RegisterRequest{
				NodeId: 1,
				ApiKey: "key",
			},
			wantCode:  codes.NotFound,
			wantError: "node not found",
		},
		{
			name: "disabled_node_returns_permission_denied",
			setupNode: func(t *testing.T, d *serviceDeps) {
				t.Helper()
				require.NoError(t, d.nodeRepo.Save(context.Background(), &domain.Node{
					Enabled:       false,
					Name:          "disabled",
					GdaemonAPIKey: "key",
				}))
			},
			setupAuth: func(d *serviceDeps) {
				d.apiKeyVerifier.valid = map[string]uint64{"key": 1}
			},
			req: &proto.RegisterRequest{
				NodeId: 1,
				ApiKey: "key",
			},
			wantCode:  codes.PermissionDenied,
			wantError: "node is disabled",
		},
		{
			name: "missing_apikey_returns_invalid_argument",
			setupNode: func(t *testing.T, d *serviceDeps) {
				t.Helper()
				require.NoError(t, d.nodeRepo.Save(context.Background(), &domain.Node{
					Enabled:       true,
					Name:          "n",
					GdaemonAPIKey: "key",
				}))
			},
			setupAuth: func(_ *serviceDeps) {},
			req: &proto.RegisterRequest{
				NodeId: 1,
				ApiKey: "",
			},
			wantCode:  codes.InvalidArgument,
			wantError: "API key is required",
		},
		{
			name: "wrong_apikey_returns_unauthenticated",
			setupNode: func(t *testing.T, d *serviceDeps) {
				t.Helper()
				require.NoError(t, d.nodeRepo.Save(context.Background(), &domain.Node{
					Enabled:       true,
					Name:          "n",
					GdaemonAPIKey: "real-key",
				}))
			},
			setupAuth: func(d *serviceDeps) {
				d.apiKeyVerifier.valid = map[string]uint64{"real-key": 1}
			},
			req: &proto.RegisterRequest{
				NodeId: 1,
				ApiKey: "wrong-key",
			},
			wantCode:  codes.Unauthenticated,
			wantError: "invalid API key",
		},
		{
			name: "verifier_error_returns_internal",
			setupNode: func(t *testing.T, d *serviceDeps) {
				t.Helper()
				require.NoError(t, d.nodeRepo.Save(context.Background(), &domain.Node{
					Enabled:       true,
					Name:          "n",
					GdaemonAPIKey: "key",
				}))
			},
			setupAuth: func(d *serviceDeps) {
				d.apiKeyVerifier.err = errSentinel
			},
			req: &proto.RegisterRequest{
				NodeId: 1,
				ApiKey: "key",
			},
			wantCode:  codes.Internal,
			wantError: "failed to verify API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			svc, deps := newServiceWithDeps(t)
			tt.setupNode(t, deps)
			tt.setupAuth(deps)

			// ACT
			err := svc.validateAuth(context.Background(), tt.req)

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err, "validateAuth must succeed")

				return
			}

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "error must be a gRPC status")
			assert.Equal(t, tt.wantCode, st.Code(), "gRPC code mismatch")
			assert.Contains(t, st.Message(), tt.wantError, "error message must contain expected substring")
		})
	}
}

// TestService_validateAuth_nilVerifierFallsBackToNodeAPIKey — OWASP API
// Security Top 10:2023 API2:2023 Broken Authentication: the no-verifier
// fallback must hash the presented gdaemon API key and constant-time compare
// it against the at-rest digest (security review findings #4/#6).
func TestService_validateAuth_nilVerifierFallsBackToNodeAPIKey(t *testing.T) {
	t.Run("matching_apikey_succeeds_when_no_verifier_configured", func(t *testing.T) {
		// ARRANGE
		// Security finding #4/#6: gdaemon_api_key is persisted hashed. The
		// fallback path must hash the presented plaintext and constant-time
		// compare it against the stored digest, so a DB read never yields a
		// usable key. Store the hash, present the plaintext.
		svc, deps := newServiceWithDeps(t)
		require.NoError(t, deps.nodeRepo.Save(context.Background(), &domain.Node{
			Enabled:       true,
			GdaemonAPIKey: pkgstrings.SHA256("stored-key"),
		}))
		// drop verifier to exercise the fallback branch
		svc.apiKeyVerifier = nil

		// ACT
		err := svc.validateAuth(context.Background(), &proto.RegisterRequest{
			NodeId: 1,
			ApiKey: "stored-key",
		})

		// ASSERT
		require.NoError(t, err, "presented plaintext hashing to the stored digest must authenticate")
	})

	t.Run("plaintext_apikey_stored_at_rest_is_rejected_when_no_verifier_configured", func(t *testing.T) {
		// ARRANGE
		// Defense-in-depth: even if a legacy plaintext key somehow remained in
		// the column, the fallback hashes the presented value before comparing,
		// so presenting that same plaintext must NOT authenticate.
		svc, deps := newServiceWithDeps(t)
		require.NoError(t, deps.nodeRepo.Save(context.Background(), &domain.Node{
			Enabled:       true,
			GdaemonAPIKey: "stored-key",
		}))
		svc.apiKeyVerifier = nil

		// ACT
		err := svc.validateAuth(context.Background(), &proto.RegisterRequest{
			NodeId: 1,
			ApiKey: "stored-key",
		})

		// ASSERT
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
		assert.Contains(t, st.Message(), "invalid API key")
	})

	t.Run("mismatched_apikey_returns_unauthenticated_when_no_verifier_configured", func(t *testing.T) {
		// ARRANGE
		// Store the hash of the real key; presenting a different plaintext must
		// hash to a different digest and be rejected.
		svc, deps := newServiceWithDeps(t)
		require.NoError(t, deps.nodeRepo.Save(context.Background(), &domain.Node{
			Enabled:       true,
			GdaemonAPIKey: pkgstrings.SHA256("stored-key"),
		}))
		svc.apiKeyVerifier = nil

		// ACT
		err := svc.validateAuth(context.Background(), &proto.RegisterRequest{
			NodeId: 1,
			ApiKey: "different-key",
		})

		// ASSERT
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
		assert.Contains(t, st.Message(), "invalid API key")
	})
}
