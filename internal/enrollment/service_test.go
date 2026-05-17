package enrollment

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupService(t *testing.T) (*Service, cache.Cache) {
	t.Helper()

	cacheInstance := cache.NewInMemory()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	keyManager := NewSetupKeyManager(cacheInstance, "")

	svc := NewService(keyManager, nodesRepo, clientCertsRepo, certsSvc)

	return svc, cacheInstance
}

func TestService_Enroll_Success(t *testing.T) {
	svc, cacheInstance := setupService(t)
	ctx := context.Background()

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "test-setup-key-32-chars-long1234")
	require.NoError(t, err)

	result, err := svc.Enroll(ctx, "test-setup-key-32-chars-long1234", &EnrollInput{
		Host:    "192.168.1.100",
		Port:    31717,
		OS:      "linux",
		Version: "1.0.0",
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotZero(t, result.NodeID)
	assert.Len(t, result.APIKey, apiKeyLength)
	assert.Contains(t, result.RootCertificate, "BEGIN CERTIFICATE")
	assert.Contains(t, result.ServerCertificate, "BEGIN CERTIFICATE")
	assert.Contains(t, result.ServerPrivateKey, "BEGIN PRIVATE KEY")
}

func TestService_Enroll_invalid_setup_key(t *testing.T) {
	svc, cacheInstance := setupService(t)
	ctx := context.Background()

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "correct-key-32-chars-long1234567")
	require.NoError(t, err)

	result, err := svc.Enroll(ctx, "wrong-key", &EnrollInput{
		Host: "192.168.1.100",
		Port: 31717,
		OS:   "linux",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSetupKey)
	assert.Nil(t, result)
}

func TestService_Enroll_no_setup_key_configured(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()

	result, err := svc.Enroll(ctx, "some-key", &EnrollInput{
		Host: "192.168.1.100",
		Port: 31717,
		OS:   "linux",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
	assert.Nil(t, result)
}

func TestService_Enroll_default_port(t *testing.T) {
	svc, cacheInstance := setupService(t)
	ctx := context.Background()

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "test-setup-key-32-chars-long1234")
	require.NoError(t, err)

	result, err := svc.Enroll(ctx, "test-setup-key-32-chars-long1234", &EnrollInput{
		Host: "10.0.0.1",
		Port: 0,
		OS:   "linux",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotZero(t, result.NodeID)
}

func TestService_Enroll_creates_node_with_correct_fields(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	keyManager := NewSetupKeyManager(cacheInstance, "")

	svc := NewService(keyManager, nodesRepo, clientCertsRepo, certsSvc)
	ctx := context.Background()

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "test-setup-key-32-chars-long1234")
	require.NoError(t, err)

	_, err = svc.Enroll(ctx, "test-setup-key-32-chars-long1234", &EnrollInput{
		Host: "gameap.example.com",
		Port: 9000,
		OS:   "windows",
	})
	require.NoError(t, err)

	nodes, err := nodesRepo.FindAll(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	node := nodes[0]
	assert.True(t, node.Enabled)
	assert.Equal(t, "gameap.example.com", node.Name)
	assert.Equal(t, "gameap.example.com", node.GdaemonHost)
	assert.Equal(t, 9000, node.GdaemonPort)
	assert.Equal(t, domain.NodeOSWindows, node.OS)
	assert.Equal(t, domain.IPList{"gameap.example.com"}, node.IPs)
	assert.Equal(t, defaultWorkPath, node.WorkPath)
	require.NotNil(t, node.SteamcmdPath)
	assert.Equal(t, defaultSteamCMDPath, *node.SteamcmdPath)
	assert.Equal(t, domain.NodePreferInstallMethodAuto, node.PreferInstallMethod)
	assert.Len(t, node.GdaemonAPIKey, apiKeyLength)
	assert.NotNil(t, node.CreatedAt)
	assert.NotNil(t, node.UpdatedAt)
}

func TestService_Enroll_with_env_setup_key(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	keyManager := NewSetupKeyManager(cacheInstance, "env-key-override")

	svc := NewService(keyManager, nodesRepo, clientCertsRepo, certsSvc)
	ctx := context.Background()

	result, err := svc.Enroll(ctx, "env-key-override", &EnrollInput{
		Host: "10.0.0.1",
		Port: 31717,
		OS:   "linux",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotZero(t, result.NodeID)
}

func TestService_Enroll_key_invalidated_after_use(t *testing.T) {
	svc, cacheInstance := setupService(t)
	ctx := context.Background()

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "one-time-key")
	require.NoError(t, err)

	_, err = svc.Enroll(ctx, "one-time-key", &EnrollInput{
		Host: "node1.example.com",
		Port: 31717,
		OS:   "linux",
	})
	require.NoError(t, err)

	_, err = svc.Enroll(ctx, "one-time-key", &EnrollInput{
		Host: "node2.example.com",
		Port: 31717,
		OS:   "linux",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
}

func TestService_Enroll_env_key_invalidated_after_use(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	fileManager := files.NewInMemoryFileManager()
	certsSvc := certificates.NewService(fileManager)
	nodesRepo := inmemory.NewNodeRepository()
	clientCertsRepo := inmemory.NewClientCertificateRepository()
	keyManager := NewSetupKeyManager(cacheInstance, "env-key")

	svc := NewService(keyManager, nodesRepo, clientCertsRepo, certsSvc)
	ctx := context.Background()

	_, err := svc.Enroll(ctx, "env-key", &EnrollInput{
		Host: "node1.example.com",
		Port: 31717,
		OS:   "linux",
	})
	require.NoError(t, err)

	_, err = svc.Enroll(ctx, "env-key", &EnrollInput{
		Host: "node2.example.com",
		Port: 31717,
		OS:   "linux",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
}
