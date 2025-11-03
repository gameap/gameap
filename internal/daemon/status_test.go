package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// func TestStatusService_Status_Success(t *testing.T) {
//	// ARRANGE
//	nodeRepo := inmemory.NewNodeRepository()
//	certRepo := inmemory.NewClientCertificateRepository()
//	fileManager := files.NewInMemoryFileManager()
//
//	ctx := context.Background()
//
//	cert := &domain.ClientCertificate{
//		Fingerprint: "test-fingerprint",
//		Expires:     time.Now().Add(365 * 24 * time.Hour),
//		Certificate: "certificates/client.crt",
//		PrivateKey:  "certificates/client.key",
//	}
//	err := certRepo.Save(ctx, cert)
//	require.NoError(t, err)
//	require.NotZero(t, cert.ID)
//
//	// Save certificate files
//	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
//	require.NoError(t, err)
//	err = fileManager.Write(ctx, cert.PrivateKey, []byte(clientKey))
//	require.NoError(t, err)
//
//	// Create node
//	node := &domain.Node{
//		Enabled:             true,
//		Name:                "Test Node",
//		OS:                  "linux",
//		Location:            "test-location",
//		WorkPath:            "/srv/gameap",
//		GdaemonHost:         "127.0.0.1",
//		GdaemonPort:         31717,
//		GDaemonAPIKey:       "test-key",
//		GdaemonServerCert:   "certificates/server.crt",
//		ClientCertificateID: cert.ID,
//		PreferInstallMethod: "auto",
//	}
//	err = nodeRepo.Save(ctx, node)
//	require.NoError(t, err)
//	require.NotZero(t, node.ID)
//
//	// Save server certificate
//	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
//	require.NoError(t, err)
//
//	// Create status service
//	statusService := NewStatusService(certRepo, fileManager)
//
//	// ACT
//	status, err := statusService.Status(ctx, node.ID)
//
//	// ASSERT
//	require.NoError(t, err)
//	assert.NotNil(t, status)
//	assert.NotEmpty(t, status.Version)
//	assert.NotEmpty(t, status.BuildDate)
//	assert.GreaterOrEqual(t, status.Uptime, time.Duration(0))
//	assert.GreaterOrEqual(t, status.WorkingTasks, 0)
//	assert.GreaterOrEqual(t, status.WaitingTasks, 0)
//	assert.GreaterOrEqual(t, status.OnlineServers, 0)
//
//	t.Logf("Status: Version=%s, BuildDate=%s, Uptime=%s, WorkingTasks=%d, WaitingTasks=%d, OnlineServers=%d",
//		status.Version,
//		status.BuildDate,
//		status.Uptime,
//		status.WorkingTasks,
//		status.WaitingTasks,
//		status.OnlineServers,
//	)
//}

func TestStatusService_Status_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.StatusVersionResponseMessage{
			Version:   "3.9.0",
			BuildDate: "2025-10-15",
		},
		&binnapi.StatusInfoBaseResponseMessage{
			Uptime:        "2h30m15s",
			WorkingTasks:  "5",
			WaitingTasks:  "3",
			OnlineServers: "10",
		},
	}

	mockServer.Start()

	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	ctx := context.Background()

	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err = certRepo.Save(ctx, cert)
	require.NoError(t, err)

	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
	require.NoError(t, err)
	err = fileManager.Write(ctx, cert.PrivateKey, []byte(clientKey))
	require.NoError(t, err)

	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		Location:            "test-location",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
		PreferInstallMethod: "auto",
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	statusService := NewStatusService(certRepo, fileManager)

	// ACT
	status, err := statusService.Status(ctx, node)

	// ASSERT
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "3.9.0", status.Version)
	assert.Equal(t, "2025-10-15", status.BuildDate)
	assert.Equal(t, 2*time.Hour+30*time.Minute+15*time.Second, status.Uptime)
	assert.Equal(t, 5, status.WorkingTasks)
	assert.Equal(t, 3, status.WaitingTasks)
	assert.Equal(t, 10, status.OnlineServers)

	t.Logf("Status: Version=%s, BuildDate=%s, Uptime=%s, WorkingTasks=%d, WaitingTasks=%d, OnlineServers=%d",
		status.Version,
		status.BuildDate,
		status.Uptime,
		status.WorkingTasks,
		status.WaitingTasks,
		status.OnlineServers,
	)
}

func TestStatusService_Status_MissingServerCertificate(t *testing.T) {
	// Setup repositories
	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	// Create test data
	ctx := context.Background()

	// Save client certificate
	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err := certRepo.Save(ctx, cert)
	require.NoError(t, err)

	// Create node without saving server certificate
	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "127.0.0.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	// Create status service
	statusService := NewStatusService(certRepo, fileManager)

	// Execute test
	status, err := statusService.Status(ctx, node)

	// Assert results
	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "failed to read server certificate")
}

func TestStatusService_Status_MissingClientCertificate(t *testing.T) {
	// Setup repositories
	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	// Create test data
	ctx := context.Background()

	// Save client certificate but not the certificate files
	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err := certRepo.Save(ctx, cert)
	require.NoError(t, err)

	// Create node
	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "127.0.0.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	// Save only server certificate
	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	// Create status service
	statusService := NewStatusService(certRepo, fileManager)

	// Execute test
	status, err := statusService.Status(ctx, node)

	// Assert results
	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "failed to read client certificate")
}

func TestStatusService_Status_MissingPrivateKey(t *testing.T) {
	// Setup repositories
	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	// Create test data
	ctx := context.Background()

	// Save client certificate
	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err := certRepo.Save(ctx, cert)
	require.NoError(t, err)

	// Save only client certificate, not private key
	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
	require.NoError(t, err)

	// Create node
	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "127.0.0.1",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	// Save server certificate
	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	// Create status service
	statusService := NewStatusService(certRepo, fileManager)

	// Execute test
	status, err := statusService.Status(ctx, node)

	// Assert results
	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "failed to read private key")
}

func TestStatusService_Status_PoolReuse(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.StatusVersionResponseMessage{Version: "3.9.0", BuildDate: "2025-10-15"},
		&binnapi.StatusInfoBaseResponseMessage{Uptime: "1h0m0s", WorkingTasks: "1", WaitingTasks: "1", OnlineServers: "1"},
		&binnapi.StatusVersionResponseMessage{Version: "3.9.0", BuildDate: "2025-10-15"},
		&binnapi.StatusInfoBaseResponseMessage{Uptime: "1h1m0s", WorkingTasks: "2", WaitingTasks: "2", OnlineServers: "2"},
		&binnapi.StatusVersionResponseMessage{Version: "3.9.0", BuildDate: "2025-10-15"},
		&binnapi.StatusInfoBaseResponseMessage{Uptime: "1h2m0s", WorkingTasks: "3", WaitingTasks: "3", OnlineServers: "3"},
	}

	mockServer.Start()

	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	ctx := context.Background()

	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err = certRepo.Save(ctx, cert)
	require.NoError(t, err)

	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
	require.NoError(t, err)
	err = fileManager.Write(ctx, cert.PrivateKey, []byte(clientKey))
	require.NoError(t, err)

	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonLogin:        lo.ToPtr("gameap"),
		GdaemonPassword:     lo.ToPtr("gameap123"),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	statusService := NewStatusService(certRepo, fileManager)

	// ACT - Execute multiple status requests
	status1, err := statusService.Status(ctx, node)
	require.NoError(t, err)
	assert.Equal(t, 1, status1.WorkingTasks)

	status2, err := statusService.Status(ctx, node)
	require.NoError(t, err)
	assert.Equal(t, 2, status2.WorkingTasks)

	status3, err := statusService.Status(ctx, node)
	require.NoError(t, err)
	assert.Equal(t, 3, status3.WorkingTasks)

	// ASSERT
	statusService.mu.RLock()
	poolCount := len(statusService.pools)
	statusService.mu.RUnlock()

	assert.Equal(t, 1, poolCount, "Expected only one pool to be created for the same node")
	mockServer.AssertMinRequestCount(6)

	t.Logf("Successfully executed %d status requests using the same pool", 3)
}

func TestStatusService_Status_MultipleNodes(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.StatusVersionResponseMessage{Version: "3.9.0", BuildDate: "2025-10-15"},
		&binnapi.StatusInfoBaseResponseMessage{Uptime: "1h0m0s", WorkingTasks: "1", WaitingTasks: "1", OnlineServers: "5"},
		&binnapi.StatusVersionResponseMessage{Version: "3.9.0", BuildDate: "2025-10-15"},
		&binnapi.StatusInfoBaseResponseMessage{Uptime: "2h0m0s", WorkingTasks: "1", WaitingTasks: "1", OnlineServers: "5"},
	}

	mockServer.Start()

	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	ctx := context.Background()

	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err = certRepo.Save(ctx, cert)
	require.NoError(t, err)

	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
	require.NoError(t, err)
	err = fileManager.Write(ctx, cert.PrivateKey, []byte(clientKey))
	require.NoError(t, err)

	node1 := &domain.Node{
		Enabled:             true,
		Name:                "Test Node 1",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonLogin:        lo.ToPtr("gameap"),
		GdaemonPassword:     lo.ToPtr("gameap123"),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node1)
	require.NoError(t, err)

	node2 := &domain.Node{
		Enabled:             true,
		Name:                "Test Node 2",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonLogin:        lo.ToPtr("gameap"),
		GdaemonPassword:     lo.ToPtr("gameap123"),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node2)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node1.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	statusService := NewStatusService(certRepo, fileManager)

	// ACT
	status1, err := statusService.Status(ctx, node1)
	require.NoError(t, err)
	assert.NotNil(t, status1)
	assert.Equal(t, "3.9.0", status1.Version)
	assert.Equal(t, 1, status1.WorkingTasks)

	status2, err := statusService.Status(ctx, node2)
	require.NoError(t, err)
	assert.NotNil(t, status2)
	assert.Equal(t, "3.9.0", status2.Version)
	assert.Equal(t, 1, status2.WorkingTasks)

	// ASSERT
	statusService.mu.RLock()
	poolCount := len(statusService.pools)
	statusService.mu.RUnlock()

	assert.Equal(t, 2, poolCount, "Expected separate pools for each node")
	mockServer.AssertMinRequestCount(4)
}

func TestStatusService_Status_InvalidNodeID(t *testing.T) {
	// Setup repositories
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	// Create status service
	statusService := NewStatusService(certRepo, fileManager)

	// Execute test with invalid node ID (0)
	ctx := context.Background()
	status, err := statusService.Status(ctx, nil)

	// Assert results
	assert.Error(t, err)
	assert.Nil(t, status)
}

func TestStatusService_Status_WithMockDaemon(t *testing.T) {
	// Create and start mock daemon server
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	// Configure custom responses for this test
	mockServer.Responses = []any{
		&binnapi.StatusVersionResponseMessage{
			Version:   "3.5.0-mock",
			BuildDate: "2025-10-15",
		},
		&binnapi.StatusInfoBaseResponseMessage{
			Uptime:        "5h20m30s",
			WorkingTasks:  "8",
			WaitingTasks:  "12",
			OnlineServers: "6",
		},
	}

	mockServer.Start()

	// Setup repositories
	nodeRepo := inmemory.NewNodeRepository()
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	// Create test data
	ctx := context.Background()

	// Save client certificate
	cert := &domain.ClientCertificate{
		Fingerprint: "test-fingerprint-mock",
		Expires:     time.Now().Add(365 * 24 * time.Hour),
		Certificate: "certificates/client.crt",
		PrivateKey:  "certificates/client.key",
	}
	err = certRepo.Save(ctx, cert)
	require.NoError(t, err)
	require.NotZero(t, cert.ID)

	// Save certificate files
	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
	require.NoError(t, err)
	err = fileManager.Write(ctx, cert.PrivateKey, []byte(clientKey))
	require.NoError(t, err)

	// Create node pointing to mock server
	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node with Mock Daemon",
		OS:                  "linux",
		Location:            "test-location",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonAPIKey:       "test-key",
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
		PreferInstallMethod: "auto",
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)
	require.NotZero(t, node.ID)

	// Save server certificate
	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	// Create status service
	statusService := NewStatusService(certRepo, fileManager)

	// Execute test
	status, err := statusService.Status(ctx, node)

	// Assert results
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "3.5.0-mock", status.Version)
	assert.Equal(t, "2025-10-15", status.BuildDate)
	assert.Equal(t, 5*time.Hour+20*time.Minute+30*time.Second, status.Uptime)
	assert.Equal(t, 8, status.WorkingTasks)
	assert.Equal(t, 12, status.WaitingTasks)
	assert.Equal(t, 6, status.OnlineServers)
	mockServer.AssertRequestCount(2)
	mockServer.AssertAnyRequestEquals(binnapi.StatusRequestVersion)
	mockServer.AssertAnyRequestEquals(binnapi.StatusRequestStatusBase)
}
