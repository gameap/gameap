// API Security Tests for OWASP API Security Top 10:2023.
// Category: API1:2023 — Broken Object Level Authorization (BOLA).
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/
//
// The file manager authorizes a request against one game server, while the
// files live on a node shared with the servers of other tenants. A path that
// looks like it stays below the server directory still leaves it through a
// symlink, and a tenant plants symlinks at will from inside the game process.
// Symlinks are resolved on the node, so the panel cannot validate its way out:
// everything it asks the daemon to do for a server has to name the directory
// the daemon must keep the request in (daemon.ConfineToServer).
//
// These tests run the real router, handlers and file services against a
// gateway that records what would go to the daemon, and pin that every file
// manager endpoint confines its requests to the directory of its own server.

package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/files"
	pubsubmemory "github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/services/filemanager/archiver"
	"github.com/gameap/gameap/internal/transfers"
	"github.com/gameap/gameap/internal/upload"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	confinedServerDir  = "servers/tenant-a"
	neighbourServerDir = "servers/tenant-b"

	uploadChunkSize = 1 << 20
)

type daemonCall struct {
	operation string
	root      string
}

// recordingGateway stands in for the gRPC gateway: it answers like a daemon
// would and keeps the confinement root every request was sent with.
type recordingGateway struct {
	mu    sync.Mutex
	calls []daemonCall
}

func (g *recordingGateway) record(operation, root string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.calls = append(g.calls, daemonCall{operation: operation, root: root})
}

func (g *recordingGateway) recorded() []daemonCall {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]daemonCall(nil), g.calls...)
}

func (g *recordingGateway) RequestFileList(
	ctx context.Context, _ uint64, dir string, _ bool, _ string,
) (*proto.FileListResponse, error) {
	g.record("list", daemon.ConfinementRoot(ctx))

	return &proto.FileListResponse{
		Success: true,
		Files: []*proto.FileStat{
			{Name: "sub", Path: path.Join(dir, "sub"), Type: proto.FileType_FILE_TYPE_DIRECTORY, Mode: 0o755},
			{Name: "a.txt", Path: path.Join(dir, "a.txt"), Type: proto.FileType_FILE_TYPE_REGULAR, Size: 3, Mode: 0o644},
		},
	}, nil
}

func (g *recordingGateway) RequestFileRead(
	ctx context.Context, _ uint64, _ string, _ int64, _ int64,
) (*proto.FileReadResponse, error) {
	g.record("read", daemon.ConfinementRoot(ctx))

	return &proto.FileReadResponse{Success: true, Content: []byte("abc")}, nil
}

func (g *recordingGateway) RequestFileWrite(
	ctx context.Context, _ uint64, _ string, _ []byte, _ int32, _ bool, _ daemon.OwnerOptions,
) error {
	g.record("write", daemon.ConfinementRoot(ctx))

	return nil
}

func (g *recordingGateway) RequestFileOperation(
	ctx context.Context, _ uint64, req *proto.FileOperationRequest,
) (*proto.FileOperationResponse, error) {
	operation := strings.ToLower(strings.TrimPrefix(req.GetOperation().String(), "FILE_OPERATION_TYPE_"))
	g.record(operation, daemon.ConfinementRoot(ctx))

	resp := &proto.FileOperationResponse{Success: true}

	switch req.GetOperation() {
	case proto.FileOperationType_FILE_OPERATION_TYPE_STAT:
		statPath := req.GetStatParams().GetPath()

		// A name without an extension is answered as a directory, which is all
		// the endpoints under test need to tell the two apart.
		fileType := proto.FileType_FILE_TYPE_DIRECTORY
		if strings.Contains(path.Base(statPath), ".") {
			fileType = proto.FileType_FILE_TYPE_REGULAR
		}

		resp.Result = &proto.FileOperationResponse_StatResult{StatResult: &proto.StatResult{
			Stat: &proto.FileStat{Name: path.Base(statPath), Path: statPath, Size: 3, Type: fileType, Mode: 0o644},
		}}
	case proto.FileOperationType_FILE_OPERATION_TYPE_HASH:
		resp.Result = &proto.FileOperationResponse_HashResult{HashResult: &proto.HashResult{}}
	default:
	}

	return resp, nil
}

func (g *recordingGateway) RequestFileUploadTask(
	ctx context.Context, _ uint64, _, _, _ string, _ int64, _ int32, _ daemon.OwnerOptions,
) error {
	g.record("upload_task", daemon.ConfinementRoot(ctx))

	return nil
}

// RequestFileDownloadTask refuses: a daemon would answer by streaming parts
// into the panel, which is of no interest here once the request is recorded.
func (g *recordingGateway) RequestFileDownloadTask(ctx context.Context, _ uint64, _, _ string) error {
	g.record("download_task", daemon.ConfinementRoot(ctx))

	return errors.New("no daemon behind the recording gateway")
}

// RequestArchive reads the root off the request itself: an archive operation
// outlives the HTTP request, so its root cannot travel in the context.
func (g *recordingGateway) RequestArchive(
	_ context.Context, _ uint64, req *proto.ArchiveRequest,
) (*proto.ArchiveResponse, error) {
	operation := "archive_create"
	if req.GetExtract() != nil {
		operation = "archive_extract"
	}

	g.record(operation, req.GetRoot())

	return &proto.ArchiveResponse{RequestId: req.GetRequestId(), Success: true}, nil
}

func (g *recordingGateway) RequestArchiveCancel(_ uint64, _, _ string) error {
	return nil
}

type connectedNode struct{}

func (connectedNode) IsConnected(uint64) bool           { return true }
func (connectedNode) IsConnectedAnywhere(uint64) bool   { return true }
func (connectedNode) HasCapability(uint64, string) bool { return true }

// fileManagerContainer puts real file services, wired to the recording
// gateway, in place of the ones the in-memory container leaves out.
type fileManagerContainer struct {
	*testcontainer.InmemoryContainer

	daemonFiles   *daemon.FileService
	daemonArchive *daemon.ArchiveService
	archiver      *archiver.Archiver
	uploads       *upload.Service
}

func (c *fileManagerContainer) DaemonFiles() *daemon.FileService        { return c.daemonFiles }
func (c *fileManagerContainer) DaemonArchive() *daemon.ArchiveService   { return c.daemonArchive }
func (c *fileManagerContainer) FileManagerArchiver() *archiver.Archiver { return c.archiver }
func (c *fileManagerContainer) UploadSessionService() *upload.Service   { return c.uploads }

// setupFileManagerConfinementTest gives the regular user the file manager of a
// server that shares its node with a neighbour.
func setupFileManagerConfinementTest(t *testing.T) (*securityTestEnv, *recordingGateway) {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)

	ctx := context.Background()

	fixtures, err := testcontainer.SetupFixtures(ctx, c)
	require.NoError(t, err)

	fixtures.Server1.DSID = fixtures.Node1.ID
	fixtures.Server1.Dir = confinedServerDir
	require.NoError(t, c.ServerRepository().Save(ctx, fixtures.Server1))

	fixtures.Server2.DSID = fixtures.Node1.ID
	fixtures.Server2.Dir = neighbourServerDir
	require.NoError(t, c.ServerRepository().Save(ctx, fixtures.Server2))

	// The chunk endpoint caps its body by the configured chunk size, which the
	// in-memory container leaves at zero.
	c.Config().Files.Upload.ChunkSize = uploadChunkSize

	gateway := &recordingGateway{}
	storage := files.NewInMemoryFileManager()

	daemonFiles := daemon.NewFileService(gateway, connectedNode{}, nil, storage, transfers.NewRegistry(), nil)

	daemonArchive := daemon.NewArchiveService(
		pubsubmemory.New(), gateway, connectedNode{}, "test-instance", daemon.ArchiveLimits{}, nil,
	)
	require.NoError(t, daemonArchive.Start(ctx))

	wrapped := &fileManagerContainer{
		InmemoryContainer: c,
		daemonFiles:       daemonFiles,
		daemonArchive:     daemonArchive,
		archiver:          archiver.NewArchiver(daemonFiles, daemonFiles, nil),
		uploads: upload.NewService(storage, daemonFiles, upload.RealClock(), nil, upload.Config{
			ChunkSize:  uploadChunkSize,
			SessionTTL: time.Hour,
			MaxChunks:  10,
		}),
	}

	return &securityTestEnv{
		container: c,
		fixtures:  fixtures,
		router:    api.CreateRouter(wrapped),
		ctx:       ctx,
	}, gateway
}

func jsonPost(target, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return req
}

func multipartRequest(t *testing.T, target, fileField, fileName string, fields map[string]string) *http.Request {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value))
	}

	part, err := writer.CreateFormFile(fileField, fileName)
	require.NoError(t, err)

	_, err = part.Write([]byte("hostname test"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req
}

// fileManagerEndpoints is every file manager endpoint that reaches the node,
// with a request the handler accepts and the daemon operations it must issue.
//
//nolint:gochecknoglobals
var fileManagerEndpoints = []struct {
	name           string
	request        func(t *testing.T, base string) *http.Request
	wantOperations []string
}{
	{
		name: "content",
		request: func(_ *testing.T, base string) *http.Request {
			return httptest.NewRequest(http.MethodGet, base+"/content?disk=server&path=cstrike", nil)
		},
		wantOperations: []string{"list"},
	},
	{
		name: "tree",
		request: func(_ *testing.T, base string) *http.Request {
			return httptest.NewRequest(http.MethodGet, base+"/tree?disk=server&path=cstrike", nil)
		},
		wantOperations: []string{"list", "list"},
	},
	{
		name: "download",
		request: func(_ *testing.T, base string) *http.Request {
			return httptest.NewRequest(http.MethodGet, base+"/download?disk=server&path=cstrike/server.cfg", nil)
		},
		wantOperations: []string{"stat", "download_task"},
	},
	{
		name: "stream_file",
		request: func(_ *testing.T, base string) *http.Request {
			return httptest.NewRequest(http.MethodGet, base+"/stream-file?disk=server&path=cstrike/server.cfg", nil)
		},
		wantOperations: []string{"stat", "download_task"},
	},
	{
		name: "download_archive",
		request: func(_ *testing.T, base string) *http.Request {
			return httptest.NewRequest(http.MethodGet, base+"/download-archive?disk=server&path=cstrike", nil)
		},
		wantOperations: []string{"stat", "list", "download_task"},
	},
	{
		name: "hash",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/hash",
				`{"disk":"server","algorithm":"sha256","paths":["cstrike/server.cfg"]}`)
		},
		wantOperations: []string{"hash"},
	},
	{
		name: "delete",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/delete",
				`{"disk":"server","items":[{"path":"cstrike/old.cfg","type":"file"}]}`)
		},
		wantOperations: []string{"delete"},
	},
	{
		name: "chmod",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/chmod",
				`{"disk":"server","mode":420,"items":[{"path":"cstrike/server.cfg"}]}`)
		},
		wantOperations: []string{"chmod"},
	},
	{
		name: "rename",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/rename",
				`{"disk":"server","oldName":"cstrike/old.cfg","newName":"cstrike/new.cfg","type":"file"}`)
		},
		wantOperations: []string{"move"},
	},
	{
		name: "paste_copy",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/paste",
				`{"disk":"server","path":"backup","clipboard":{"type":"copy","disk":"server","files":["cstrike/server.cfg"]}}`)
		},
		wantOperations: []string{"copy"},
	},
	{
		name: "paste_cut",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/paste",
				`{"disk":"server","path":"backup","clipboard":{"type":"cut","disk":"server","files":["cstrike/server.cfg"]}}`)
		},
		wantOperations: []string{"move"},
	},
	{
		name: "create_directory",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/create-directory",
				`{"disk":"server","path":"cstrike","name":"addons"}`)
		},
		wantOperations: []string{"mkdir", "stat"},
	},
	{
		name: "create_file",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/create-file",
				`{"disk":"server","path":"cstrike","name":"motd.txt"}`)
		},
		wantOperations: []string{"write", "stat"},
	},
	{
		name: "update_file",
		request: func(t *testing.T, base string) *http.Request {
			t.Helper()

			return multipartRequest(t, base+"/update-file", "file", "server.cfg",
				map[string]string{"disk": "server", "path": "cstrike"})
		},
		wantOperations: []string{"write", "stat"},
	},
	{
		name: "upload",
		request: func(t *testing.T, base string) *http.Request {
			t.Helper()

			return multipartRequest(t, base+"/upload", "files[]", "server.cfg",
				map[string]string{"disk": "server", "path": "cstrike", "overwrite": "1"})
		},
		wantOperations: []string{"write"},
	},
	{
		name: "archive",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/archive",
				`{"disk":"server","path":"","name":"backup.zip","sources":["cstrike"]}`)
		},
		wantOperations: []string{"archive_create"},
	},
	{
		name: "extract",
		request: func(_ *testing.T, base string) *http.Request {
			return jsonPost(base+"/extract",
				`{"disk":"server","path":"backup.zip","destination":"","conflict_policy":"skip"}`)
		},
		wantOperations: []string{"archive_extract"},
	},
}

// assertConfinedCalls waits for the daemon operations an endpoint must issue
// (some start after the response, in goroutines of their own) and checks the
// root of everything that went out.
func assertConfinedCalls(t *testing.T, gateway *recordingGateway, wantOperations []string, wantRoot string) {
	t.Helper()

	require.Eventually(t, func() bool {
		return containsOperations(gateway.recorded(), wantOperations)
	}, 5*time.Second, 5*time.Millisecond,
		"daemon operations %v were expected, recorded: %v", wantOperations, gateway.recorded())

	for _, call := range gateway.recorded() {
		assert.Equal(t, wantRoot, call.root, "operation %q went to the daemon with the wrong confinement root", call.operation)
	}
}

func containsOperations(calls []daemonCall, wantOperations []string) bool {
	counts := make(map[string]int, len(calls))
	for _, call := range calls {
		counts[call.operation]++
	}

	for _, operation := range wantOperations {
		if counts[operation] == 0 {
			return false
		}

		counts[operation]--
	}

	return true
}

// OWASP API1:2023 — Broken Object Level Authorization.
// A tenant with file manager access to one game server must not reach the files
// of another one through a symlink, so every request the file manager sends to
// the node is confined to the directory of the server it was authorized for.
func TestRouterSecurity_API1_BOLA_FileManagerConfinesDaemonRequestsToServerDir(t *testing.T) {
	t.Parallel()

	for _, endpoint := range fileManagerEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			t.Parallel()

			env, gateway := setupFileManagerConfinementTest(t)
			token := issuePASETOToken(t, env, env.fixtures.RegularUser)

			req := endpoint.request(t, fmt.Sprintf("/api/file-manager/%d", env.fixtures.Server1.ID))
			req.Header.Set("Authorization", "Bearer "+token)

			resp := doRequestRaw(t, env, req)

			require.NotContains(t,
				[]int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
				resp.Code, "the request must get past validation and authorization: %s", resp.Body.String())

			assertConfinedCalls(t, gateway, endpoint.wantOperations, confinedServerDir)
		})
	}
}

// OWASP API1:2023 — Broken Object Level Authorization.
// The root follows the server the request names, not the user: an administrator
// working in the neighbour's file manager is confined to the neighbour.
func TestRouterSecurity_API1_BOLA_FileManagerConfinementFollowsTheServer(t *testing.T) {
	t.Parallel()

	env, gateway := setupFileManagerConfinementTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)

	for serverID, wantRoot := range map[uint]string{
		env.fixtures.Server1.ID: confinedServerDir,
		env.fixtures.Server2.ID: neighbourServerDir,
	} {
		target := fmt.Sprintf("/api/file-manager/%d/content?disk=server&path=.", serverID)

		resp := doRequest(t, env, http.MethodGet, target, token)
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

		calls := gateway.recorded()
		require.NotEmpty(t, calls)
		assert.Equal(t, daemonCall{operation: "list", root: wantRoot}, calls[len(calls)-1])
	}
}

// OWASP API1:2023 — Broken Object Level Authorization.
// A chunked upload reaches the node only when it is completed, and the session
// carries the destination from the request that created it. The dispatch is
// confined to the server of the completing request, which also keeps a session
// created for one server from being completed through another.
func TestRouterSecurity_API1_BOLA_FileManagerChunkedUploadIsConfinedToServerDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// completeOnServer picks the server whose endpoint completes the session
		// created for Server1.
		completeOnServer func(fixtures *testcontainer.TestFixtures) uint
		wantRoot         string
	}{
		{
			name:             "completed_on_its_own_server",
			completeOnServer: func(fixtures *testcontainer.TestFixtures) uint { return fixtures.Server1.ID },
			wantRoot:         confinedServerDir,
		},
		{
			name:             "completed_through_the_neighbour_stays_confined_to_the_neighbour",
			completeOnServer: func(fixtures *testcontainer.TestFixtures) uint { return fixtures.Server2.ID },
			wantRoot:         neighbourServerDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env, gateway := setupFileManagerConfinementTest(t)
			token := issuePASETOToken(t, env, env.fixtures.AdminUser)

			content := []byte("hostname test")
			checksum := sha256.Sum256(content)

			sessionsURL := fmt.Sprintf("/api/file-manager/%d/upload/sessions", env.fixtures.Server1.ID)

			created := doRequestWithBody(t, env, http.MethodPost, sessionsURL, token, fmt.Appendf(nil,
				`{"path":"cstrike","filename":"server.cfg","total_size":%d,"expected_checksum":%q}`,
				len(content), hex.EncodeToString(checksum[:]),
			))
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

			var session struct {
				UploadID string `json:"upload_id"`
			}
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &session))
			require.NotEmpty(t, session.UploadID)

			chunk := httptest.NewRequest(http.MethodPut, sessionsURL+"/"+session.UploadID+"/chunks/0", bytes.NewReader(content))
			chunk.Header.Set("Authorization", "Bearer "+token)
			require.Equal(t, http.StatusNoContent, doRequestRaw(t, env, chunk).Code)

			require.Empty(t, gateway.recorded(), "nothing reaches the node before the upload is completed")

			completeURL := fmt.Sprintf("/api/file-manager/%d/upload/sessions/%s/complete",
				tt.completeOnServer(env.fixtures), session.UploadID)

			completed := doRequestWithBody(t, env, http.MethodPost, completeURL, token, nil)
			require.Equal(t, http.StatusOK, completed.Code, completed.Body.String())

			assert.Equal(t, []daemonCall{{operation: "upload_task", root: tt.wantRoot}}, gateway.recorded())
		})
	}
}
