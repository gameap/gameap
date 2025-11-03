package daemon

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFileServiceTest(t *testing.T, mockServer *MockDaemonServer) (*FileService, *domain.Node) {
	t.Helper()

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

	fileService := NewFileService(certRepo, fileManager)

	return fileService, node
}

// func TestFileService(t *testing.T) {
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
//	fileService := NewFileService(nodeRepo, certRepo, fileManager)
//
//	// ACTs & ASSERTs
//
//	//f, err := fileService.Download(
//	//	ctx,
//	//	node.ID,
//	//	"/root/file.txt",
//	//)
//	//require.NoError(t, err)
//	//require.NotNil(t, f)
//	//
//	//err = fileService.Upload(
//	//	ctx,
//	//	node.ID,
//	//	"/srv/gameap/uploaded.txt",
//	//	[]byte("Test file content"),
//	//	0644,
//	//)
//	//require.NoError(t, err)
//
//	list, err := fileService.ReadDir(
//		ctx,
//		node.ID,
//		"/srv/gameap",
//	)
//	require.NoError(t, err)
//	require.NotEmpty(t, list)
//}

func TestFileService_MkDir_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Directory created",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.MkDir(ctx, node, "/srv/gameap/newdir")

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var mkdirRequest binnapi.MkDirRequestMessage
	mockServer.UnmarshalRequest(0, &mkdirRequest)
	assert.Equal(t, "/srv/gameap/newdir", mkdirRequest.Directory)
}

func TestFileService_Remove_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "File removed",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Remove(ctx, node, "/srv/gameap/file.txt", false)

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var removeRequest binnapi.RemoveRequestMessage
	mockServer.UnmarshalRequest(0, &removeRequest)
	assert.Equal(t, "/srv/gameap/file.txt", removeRequest.Path)
	assert.False(t, removeRequest.Recursive)
}

func TestFileService_Remove_Recursive(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Directory removed",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Remove(ctx, node, "/srv/gameap/olddir", true)

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var removeRequest binnapi.RemoveRequestMessage
	mockServer.UnmarshalRequest(0, &removeRequest)
	assert.Equal(t, "/srv/gameap/olddir", removeRequest.Path)
	assert.True(t, removeRequest.Recursive)
}

func TestFileService_Move_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "File moved",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Move(ctx, node, "/srv/gameap/file.txt", "/srv/gameap/newfile.txt")

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var moveRequest binnapi.MoveRequestMessage
	mockServer.UnmarshalRequest(0, &moveRequest)
	assert.Equal(t, "/srv/gameap/file.txt", moveRequest.Source)
	assert.Equal(t, "/srv/gameap/newfile.txt", moveRequest.Destination)
	assert.False(t, moveRequest.Copy)
}

func TestFileService_Copy(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "File copied",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Copy(ctx, node, "/srv/gameap/file.txt", "/srv/gameap/copy.txt")

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var moveRequest binnapi.MoveRequestMessage
	mockServer.UnmarshalRequest(0, &moveRequest)
	assert.Equal(t, "/srv/gameap/file.txt", moveRequest.Source)
	assert.Equal(t, "/srv/gameap/copy.txt", moveRequest.Destination)
	assert.True(t, moveRequest.Copy)
}

func TestFileService_Chmod_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Permissions changed",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Chmod(ctx, node, "/srv/gameap/file.txt", 0755)

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var chmodRequest binnapi.ChmodMessage
	mockServer.UnmarshalRequest(0, &chmodRequest)
	assert.Equal(t, "/srv/gameap/file.txt", chmodRequest.Path)
	assert.Equal(t, uint32(0755), chmodRequest.Perm)
}

func TestFileService_GetFileInfo_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "File info retrieved",
			Data: []any{
				"file.txt",         // name
				uint64(1024),       // size
				uint8(2),           // type (file)
				uint64(1634567890), // mod time
				uint64(1634567890), // access time
				uint64(1634567890), // create time
				uint32(0644),       // perm
				"text/plain",       // mime
			},
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	fileDetails, err := fileService.GetFileInfo(ctx, node, "/srv/gameap/file.txt")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, fileDetails)
	assert.Equal(t, "file.txt", fileDetails.Name)
	assert.Equal(t, uint64(1024), fileDetails.Size)
	assert.Equal(t, FileTypeFile, fileDetails.Type)
	assert.Equal(t, uint32(0644), fileDetails.Perm)
	assert.Equal(t, "text/plain", fileDetails.Mime)

	mockServer.AssertRequestCount(1)

	var fileInfoRequest binnapi.FileInfoRequestMessage
	mockServer.UnmarshalRequest(0, &fileInfoRequest)
	assert.Equal(t, "/srv/gameap/file.txt", fileInfoRequest.Path)
}

func TestFileService_ReadDir_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Directory read",
			Data: []any{
				[]any{"file1.txt", uint64(100), uint64(1634567890), uint8(2), uint32(0644)},
				[]any{"file2.txt", uint64(200), uint64(1634567890), uint8(2), uint32(0644)},
				[]any{"subdir", uint64(0), uint64(1634567890), uint8(1), uint32(0755)},
			},
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	fileList, err := fileService.ReadDir(ctx, node, "/srv/gameap")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, fileList, 3)

	assert.Equal(t, "file1.txt", fileList[0].Name)
	assert.Equal(t, uint64(100), fileList[0].Size)

	assert.Equal(t, "file2.txt", fileList[1].Name)
	assert.Equal(t, uint64(200), fileList[1].Size)

	assert.Equal(t, "subdir", fileList[2].Name)
	assert.Equal(t, FileTypeDir, fileList[2].Type) // directory

	mockServer.AssertRequestCount(1)

	var readDirRequest binnapi.ReadDirRequestMessage
	mockServer.UnmarshalRequest(0, &readDirRequest)
	assert.Equal(t, "/srv/gameap", readDirRequest.Directory)
	assert.False(t, readDirRequest.DetailsMode)
}

func TestFileService_NodeNotFound(t *testing.T) {
	// ARRANGE
	certRepo := inmemory.NewClientCertificateRepository()
	fileManager := files.NewInMemoryFileManager()

	fileService := NewFileService(certRepo, fileManager)

	// ACT
	ctx := context.Background()
	err := fileService.MkDir(ctx, nil, "/srv/gameap/newdir")

	// ASSERT
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")
}

func TestFileService_PoolReuse(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{Code: binnapi.StatusCodeOK, Info: "OK"},
		&binnapi.BaseResponseMessage{Code: binnapi.StatusCodeOK, Info: "OK"},
		&binnapi.BaseResponseMessage{Code: binnapi.StatusCodeOK, Info: "OK"},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT - Execute multiple operations
	ctx := context.Background()
	err = fileService.MkDir(ctx, node, "/srv/gameap/dir1")
	require.NoError(t, err)

	err = fileService.MkDir(ctx, node, "/srv/gameap/dir2")
	require.NoError(t, err)

	err = fileService.MkDir(ctx, node, "/srv/gameap/dir3")
	require.NoError(t, err)

	// ASSERT
	fileService.mu.RLock()
	poolCount := len(fileService.pools)
	fileService.mu.RUnlock()

	assert.Equal(t, 1, poolCount, "Expected only one pool to be created for the same node")
	mockServer.AssertMinRequestCount(3)
}

func TestFileService_Upload_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
		},
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Upload complete",
		},
	}

	testContent := []byte("Test file content")

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Upload(
		ctx,
		node,
		"/srv/gameap/file.txt",
		testContent,
		0644,
	)

	// ASSERT
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond) // TODO: Remove sleep after fixing mock server
	mockServer.AssertReceivedFileEquals(testContent)
}

func TestFileService_UploadStream_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
		},
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Upload complete",
		},
	}

	mockServer.Start()
	defer mockServer.Stop()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	testContent := []byte("Test file content for streaming")
	reader := bytes.NewReader(testContent)
	err = fileService.UploadStream(
		ctx,
		node,
		"/srv/gameap/stream-file.txt",
		reader,
		uint64(len(testContent)),
		0644,
	)

	// ASSERT
	require.NoError(t, err)
	mockServer.AssertRequestCount(1)

	var uploadRequest binnapi.UploadRequestMessage
	mockServer.UnmarshalRequest(0, &uploadRequest)
	assert.Equal(t, "/srv/gameap/stream-file.txt", uploadRequest.FilePath)
	assert.Equal(t, uint64(len(testContent)), uploadRequest.FileSize)
	assert.Equal(t, os.FileMode(0644), uploadRequest.Perms)
	assert.True(t, uploadRequest.MakeDirs)
	time.Sleep(10 * time.Millisecond)
	mockServer.AssertReceivedFileEquals(testContent)
}

func TestFileService_Download_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	testFileContent := fileRaw("Test file content")

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: len(testFileContent),
		},
		testFileContent,
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	ctx := context.Background()
	file, err := fileService.Download(
		ctx,
		node,
		"/srv/gameap/file.txt",
	)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Equal(t, []byte(testFileContent), file)
}

// Test error handling.
func TestFileService_MkDir_Failure(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeError,
			Info: "Failed to create directory",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.MkDir(ctx, node, "/srv/gameap/existingdir")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir failed")
	mockServer.AssertRequestCount(1)
}

func TestFileService_Remove_Failure(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeError,
			Info: "Failed to remove file",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Remove(ctx, node, "/srv/gameap/nonexistent.txt", false)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remove failed")
	mockServer.AssertRequestCount(1)
}

func TestFileService_Chmod_Failure(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeError,
			Info: "Failed to change permissions",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	err = fileService.Chmod(ctx, node, "/srv/gameap/protected.txt", 0777)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chmod failed")
	mockServer.AssertRequestCount(1)
}

func TestFileService_ReadDir_SingleFile(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeOK,
			Info: "Directory read",
			Data: []any{
				[]any{"file1.txt", uint64(100), uint64(1634567890), uint8(2), uint32(0644)},
			},
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	fileList, err := fileService.ReadDir(ctx, node, "/srv/gameap")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, fileList, 1)
	assert.Equal(t, "file1.txt", fileList[0].Name)

	mockServer.AssertRequestCount(1)

	var readDirRequest binnapi.ReadDirRequestMessage
	mockServer.UnmarshalRequest(0, &readDirRequest)
	assert.Equal(t, "/srv/gameap", readDirRequest.Directory)
	assert.False(t, readDirRequest.DetailsMode)
}

func TestFileService_DownloadStream_Success(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	testFileContent := fileRaw("Test file content for streaming")

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: len(testFileContent),
		},
		testFileContent,
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/file.txt")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer reader.Close()

	// Read the entire stream
	content, err := io.ReadAll(reader)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, []byte(testFileContent), content)

	mockServer.AssertRequestCount(1)

	var downloadRequest binnapi.DownloadRequestMessage
	mockServer.UnmarshalRequest(0, &downloadRequest)
	assert.Equal(t, "/srv/gameap/file.txt", downloadRequest.FilePath)
}

func TestFileService_DownloadStream_LargeFile(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	// Create a large file content (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	testFileContent := fileRaw(largeContent)

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: len(testFileContent),
		},
		testFileContent,
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/large-file.bin")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer reader.Close()

	// Read in chunks to simulate streaming
	buffer := make([]byte, 4096)
	totalRead := 0
	for {
		n, readErr := reader.Read(buffer)
		totalRead += n
		if readErr == io.EOF {
			break
		}
		require.NoError(t, readErr)
	}

	// ASSERT
	assert.Equal(t, len(testFileContent), totalRead)

	mockServer.AssertRequestCount(1)
}

func TestFileService_DownloadStream_EmptyFile(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: 0, // Empty file
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/empty.txt")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer reader.Close()

	// Read the stream
	content, err := io.ReadAll(reader)

	// ASSERT
	require.NoError(t, err)
	assert.Empty(t, content)

	mockServer.AssertRequestCount(1)
}

func TestFileService_DownloadStream_StatusCodeError(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeError,
			Info: "File not found",
		},
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeError,
			Info: "File not found",
		},
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/nonexistent.txt")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, reader)
	assert.Contains(t, err.Error(), "download failed")
	assert.Contains(t, err.Error(), "File not found")

	mockServer.AssertRequestCount(2)
}

func TestFileService_DownloadStream_PartialRead(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	testFileContent := fileRaw("This is a test file with some content")

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: len(testFileContent),
		},
		testFileContent,
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/file.txt")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer reader.Close()

	// Read only part of the file
	buffer := make([]byte, 10)
	n, err := reader.Read(buffer)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, 10, n)
	assert.Equal(t, []byte(testFileContent)[:10], buffer)
}

func TestFileService_DownloadStream_CloseConnection(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	testFileContent := fileRaw("Test content")

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: len(testFileContent),
		},
		testFileContent,
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/file.txt")
	require.NoError(t, err)
	require.NotNil(t, reader)

	// Close the reader immediately without reading
	err = reader.Close()

	// ASSERT
	require.NoError(t, err)

	mockServer.AssertRequestCount(1)
}

func TestFileService_DownloadStream_ReadAfterClose(t *testing.T) {
	// ARRANGE
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	testFileContent := fileRaw("Test content")

	mockServer.Responses = []any{
		&binnapi.BaseResponseMessage{
			Code: binnapi.StatusCodeReadyToTransfer,
			Info: "Ready",
			Data: len(testFileContent),
		},
		testFileContent,
	}

	mockServer.Start()

	fileService, node := setupFileServiceTest(t, mockServer)

	// ACT
	ctx := context.Background()
	reader, err := fileService.DownloadStream(ctx, node, "/srv/gameap/file.txt")
	require.NoError(t, err)
	require.NotNil(t, reader)

	// Close the reader
	err = reader.Close()
	require.NoError(t, err)

	// Try to read after close
	buffer := make([]byte, 10)
	_, err = reader.Read(buffer)

	// ASSERT
	require.Error(t, err)
}
