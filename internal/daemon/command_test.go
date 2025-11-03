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

// func TestCommandService_ExecuteCommand_Success(t *testing.T) {
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
//		GdaemonHost:         "127.0.0.0",
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
//	service := NewCommandService(nodeRepo, certRepo, fileManager)
//
//	// ACT
//	result, err := service.ExecuteCommand(
//		ctx,
//		node.ID,
//		"ls -al ./",
//		CommandServiceOptionWithWorkDir("/srv/gameap"),
//	)
//
//	// ASSERT
//	require.NoError(t, err)
//	assert.NotNil(t, result)
//}

func TestCommandService_ExecuteCommand_WithMockDaemon(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.CommandExecResponseMessage{
			Output:   "total 48\ndrwxr-xr-x 5 root root 4096 Oct 15 10:30 .\ndrwxr-xr-x 3 root root 4096 Oct 15 10:30 ..",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
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
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result, err := service.ExecuteCommand(
		ctx,
		node,
		"ls -al",
		CommandServiceOptionWithWorkDir("/root"),
	)

	// ASSERT
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "total 48\ndrwxr-xr-x 5 root root 4096 Oct 15 10:30 .\ndrwxr-xr-x 3 root root 4096 Oct 15 10:30 ..", result.Output)
	assert.Equal(t, 0, result.ExitCode)

	mockServer.AssertRequestCount(1)

	var cmdReq binnapi.CommandExecRequestMessage
	mockServer.UnmarshalRequest(0, &cmdReq)
	assert.Equal(t, "ls -al", cmdReq.Command)
	assert.Equal(t, "/root", cmdReq.WorkDir)
}

func TestCommandService_ExecuteCommand_CommandFailure(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.CommandExecResponseMessage{
			Output:   "ls: cannot access '/nonexistent': No such file or directory",
			ExitCode: 2,
			Code:     binnapi.StatusCodeOK,
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
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result, err := service.ExecuteCommand(
		ctx,
		node,
		"ls /nonexistent",
		CommandServiceOptionWithWorkDir("/root"),
	)

	// ASSERT
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.ExitCode)
	assert.Contains(t, result.Output, "No such file or directory")
}

func TestCommandService_ExecuteCommand_NodeNotFound(t *testing.T) {
	// ARRANGE
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	service := NewCommandService(certRepo, fileManager)

	// ACT
	ctx := context.Background()
	result, err := service.ExecuteCommand(ctx, nil, "ls -al")

	// ASSERT
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestCommandService_ExecuteCommand_MissingServerCertificate(t *testing.T) {
	// ARRANGE
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
	err := certRepo.Save(ctx, cert)
	require.NoError(t, err)

	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "127.0.0.1",
		GdaemonPort:         31717,
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result, err := service.ExecuteCommand(ctx, node, "ls -al")

	// ASSERT
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to read server certificate")
}

func TestCommandService_ExecuteCommand_MissingClientCertificate(t *testing.T) {
	// ARRANGE
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
	err := certRepo.Save(ctx, cert)
	require.NoError(t, err)

	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "127.0.0.1",
		GdaemonPort:         31717,
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result, err := service.ExecuteCommand(ctx, node, "ls -al")

	// ASSERT
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to read client certificate")
}

func TestCommandService_ExecuteCommand_MissingPrivateKey(t *testing.T) {
	// ARRANGE
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
	err := certRepo.Save(ctx, cert)
	require.NoError(t, err)

	err = fileManager.Write(ctx, cert.Certificate, []byte(clientCert))
	require.NoError(t, err)

	node := &domain.Node{
		Enabled:             true,
		Name:                "Test Node",
		OS:                  "linux",
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "127.0.0.1",
		GdaemonPort:         31717,
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result, err := service.ExecuteCommand(ctx, node, "ls -al")

	// ASSERT
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to read private key")
}

func TestCommandService_ExecuteCommand_PoolReuse(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.CommandExecResponseMessage{
			Output:   "command 1 output",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
		},
		&binnapi.CommandExecResponseMessage{
			Output:   "command 2 output",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
		},
		&binnapi.CommandExecResponseMessage{
			Output:   "command 3 output",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
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

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result1, err := service.ExecuteCommand(ctx, node, "echo test1", CommandServiceOptionWithWorkDir("/root"))
	require.NoError(t, err)
	assert.Equal(t, "command 1 output", result1.Output)

	result2, err := service.ExecuteCommand(ctx, node, "echo test2", CommandServiceOptionWithWorkDir("/root"))
	require.NoError(t, err)
	assert.Equal(t, "command 2 output", result2.Output)

	result3, err := service.ExecuteCommand(ctx, node, "echo test3", CommandServiceOptionWithWorkDir("/root"))
	require.NoError(t, err)
	assert.Equal(t, "command 3 output", result3.Output)

	// ASSERT
	service.mu.RLock()
	poolCount := len(service.pools)
	service.mu.RUnlock()

	assert.Equal(t, 1, poolCount, "Expected only one pool to be created for the same node")
	mockServer.AssertMinRequestCount(3)

	t.Logf("Successfully executed %d commands using the same pool", 3)
}

func TestCommandService_ExecuteCommand_MultipleNodes(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.CommandExecResponseMessage{
			Output:   "node 1 output",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
		},
		&binnapi.CommandExecResponseMessage{
			Output:   "node 2 output",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
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

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result1, err := service.ExecuteCommand(ctx, node1, "echo node1")
	require.NoError(t, err)
	assert.NotNil(t, result1)

	result2, err := service.ExecuteCommand(ctx, node2, "echo node2")
	require.NoError(t, err)
	assert.NotNil(t, result2)

	// ASSERT
	service.mu.RLock()
	poolCount := len(service.pools)
	service.mu.RUnlock()

	assert.Equal(t, 2, poolCount, "Expected separate pools for each node")
}

func TestCommandService_ExecuteCommand_DifferentWorkDirectories(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.CommandExecResponseMessage{
			Output:   "/srv/gameap",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
		},
		&binnapi.CommandExecResponseMessage{
			Output:   "/root",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
		},
		&binnapi.CommandExecResponseMessage{
			Output:   "/tmp",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
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
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result1, err := service.ExecuteCommand(
		ctx,
		node,
		"pwd",
		CommandServiceOptionWithWorkDir("/srv/gameap"),
	)
	require.NoError(t, err)
	assert.Equal(t, "/srv/gameap", result1.Output)

	result2, err := service.ExecuteCommand(
		ctx,
		node,
		"pwd",
		CommandServiceOptionWithWorkDir("/root"),
	)
	require.NoError(t, err)
	assert.Equal(t, "/root", result2.Output)

	result3, err := service.ExecuteCommand(
		ctx,
		node,
		"pwd",
		CommandServiceOptionWithWorkDir("/tmp"),
	)
	require.NoError(t, err)
	assert.Equal(t, "/tmp", result3.Output)

	// ASSERT
	mockServer.AssertRequestCount(3)

	var cmdReq1 binnapi.CommandExecRequestMessage
	mockServer.UnmarshalRequest(0, &cmdReq1)
	assert.Equal(t, "/srv/gameap", cmdReq1.WorkDir)

	var cmdReq2 binnapi.CommandExecRequestMessage
	mockServer.UnmarshalRequest(1, &cmdReq2)
	assert.Equal(t, "/root", cmdReq2.WorkDir)

	var cmdReq3 binnapi.CommandExecRequestMessage
	mockServer.UnmarshalRequest(2, &cmdReq3)
	assert.Equal(t, "/tmp", cmdReq3.WorkDir)

	t.Logf("Successfully executed commands in different work directories")
}

func TestCommandService_ExecuteCommand_DefaultWorkDirectory(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.CommandExecResponseMessage{
			Output:   "/",
			ExitCode: 0,
			Code:     binnapi.StatusCodeOK,
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
		WorkPath:            "/srv/gameap",
		GdaemonHost:         mockServer.Host(),
		GdaemonPort:         mockServer.Port(),
		GdaemonServerCert:   "certificates/server.crt",
		ClientCertificateID: cert.ID,
	}
	err = nodeRepo.Save(ctx, node)
	require.NoError(t, err)

	err = fileManager.Write(ctx, node.GdaemonServerCert, []byte(daemonServerCert))
	require.NoError(t, err)

	service := NewCommandService(certRepo, fileManager)

	// ACT
	result, err := service.ExecuteCommand(ctx, node, "pwd")

	// ASSERT
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "/", result.Output)

	mockServer.AssertRequestCount(1)

	var cmdReq binnapi.CommandExecRequestMessage
	mockServer.UnmarshalRequest(0, &cmdReq)
	assert.Equal(t, "/", cmdReq.WorkDir, "Expected default work directory to be /")
}
