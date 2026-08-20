package gateway

import (
	"context"
	"net"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func newServiceWithEnrollment(t *testing.T) (*Service, cache.Cache, *inmemory.NodeRepository) {
	t.Helper()

	svc, deps := newServiceWithDeps(t)

	cacheInstance := cache.NewInMemory()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	keyManager := enrollment.NewSetupKeyManager(cacheInstance, "")

	svc.enrollmentSvc = enrollment.NewService(keyManager, deps.nodeRepo, clientCertsRepo, certsSvc)

	return svc, cacheInstance, deps.nodeRepo
}

func TestService_Enroll(t *testing.T) {
	t.Parallel()
	t.Run("returns_unavailable_when_enrollment_disabled", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, _ := newServiceWithDeps(t)
		// enrollmentSvc is nil by default

		// ACT
		resp, err := svc.Enroll(context.Background(), &proto.EnrollRequest{SetupKey: "k"})

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
		assert.Contains(t, st.Message(), "enrollment is not enabled")
	})

	t.Run("invalid_setup_key_returns_unauthenticated", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, cacheInstance, _ := newServiceWithEnrollment(t)
		const correctKey = "correct-key-32-chars-long1234567"
		require.NoError(t, cacheInstance.Set(context.Background(), enrollment.SetupKeyCacheKey, correctKey))

		// ACT
		resp, err := svc.Enroll(context.Background(), &proto.EnrollRequest{
			SetupKey: "wrong-key",
			Host:     "1.2.3.4",
			Port:     31717,
			Os:       "linux",
		})

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
	})

	t.Run("missing_setup_key_returns_unauthenticated", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, _, _ := newServiceWithEnrollment(t)

		// ACT
		resp, err := svc.Enroll(context.Background(), &proto.EnrollRequest{
			SetupKey: "any-key",
			Host:     "1.2.3.4",
			Port:     31717,
			Os:       "linux",
		})

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, resp)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, st.Code())
	})

	t.Run("happy_path_returns_node_credentials_and_certs", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, cacheInstance, nodeRepo := newServiceWithEnrollment(t)
		const setupKey = "test-setup-key-32-chars-long1234"
		require.NoError(t, cacheInstance.Set(context.Background(), enrollment.SetupKeyCacheKey, setupKey))

		// ACT
		resp, err := svc.Enroll(context.Background(), &proto.EnrollRequest{
			SetupKey:     setupKey,
			Host:         "10.0.0.1",
			Port:         31717,
			Os:           "linux",
			Version:      "1.2.3",
			Capabilities: []string{"http_proxy"},
		})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.NotZero(t, resp.NodeId, "enrolled node id must be assigned")
		assert.NotEmpty(t, resp.ApiKey, "API key must be returned to the daemon")
		assert.Contains(t, resp.RootCertificate, "BEGIN CERTIFICATE")
		assert.Contains(t, resp.ServerCertificate, "BEGIN CERTIFICATE")
		assert.Contains(t, resp.ServerPrivateKey, "BEGIN PRIVATE KEY")

		// node must exist in the repo
		node, findErr := nodeRepo.Find(context.Background(), nil, nil, nil)
		require.NoError(t, findErr)
		require.Len(t, node, 1)
		assert.Equal(t, "10.0.0.1", node[0].Name)
	})

	t.Run("uses_peer_address_when_host_omitted", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		svc, cacheInstance, nodeRepo := newServiceWithEnrollment(t)
		const setupKey = "test-setup-key-32-chars-long1234"
		require.NoError(t, cacheInstance.Set(context.Background(), enrollment.SetupKeyCacheKey, setupKey))

		ctx := peer.NewContext(context.Background(), &peer.Peer{
			Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.5"), Port: 41234},
		})

		// ACT
		resp, err := svc.Enroll(ctx, &proto.EnrollRequest{
			SetupKey: setupKey,
			Port:     31717,
			Os:       "linux",
		})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)

		nodes, findErr := nodeRepo.Find(context.Background(), nil, nil, nil)
		require.NoError(t, findErr)
		require.Len(t, nodes, 1)
		assert.Contains(t, nodes[0].Name, "203.0.113.5",
			"node name must derive from peer address when Host is empty")
	})
}
