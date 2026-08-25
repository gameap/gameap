package compatrust

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"slices"
	"strings"
	"sync"

	"github.com/gameap/gameap/pkg/plugin/sdk/cache"
	"github.com/gameap/gameap/pkg/plugin/sdk/crypto"
	"github.com/gameap/gameap/pkg/plugin/sdk/daemontasks"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/http"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodes"
	"github.com/gameap/gameap/pkg/plugin/sdk/servercontrol"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/gameap/gameap/pkg/plugin/sdk/serversettings"
	"github.com/gameap/gameap/pkg/plugin/sdk/storage"
	"github.com/gameap/gameap/pkg/plugin/sdk/users"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/tetratelabs/wazero"
)

// This file stubs the host services of every SDK module that exists on panel
// v4.3.5. It must keep compiling there: only use API available on v4.3.5.

// hostLibFunc adapts a function to the plugin.HostLibrary interface.
type hostLibFunc func(ctx context.Context, r wazero.Runtime) error

func (f hostLibFunc) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return f(ctx, r)
}

// callRecorder tracks which host functions the guest plugin invoked.
type callRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *callRecorder) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
}

// Calls returns the recorded host function names in invocation order.
func (r *callRecorder) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.calls...)
}

// Called reports whether the named host function was invoked at least once.
func (r *callRecorder) Called(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Contains(r.calls, name)
}

// logEntry is a single line emitted by the plugin through the log host library.
type logEntry struct {
	Level   string
	Message string
}

// stubLogService satisfies log.LogService and records every emitted line.
type stubLogService struct {
	mu      sync.Mutex
	entries []logEntry
}

func (s *stubLogService) Log(_ context.Context, req *log.LogRequest) (*log.LogResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, logEntry{Level: req.Level, Message: req.Message})

	return &log.LogResponse{}, nil
}

// Entries returns the recorded log lines in emission order.
func (s *stubLogService) Entries() []logEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]logEntry(nil), s.entries...)
}

// stubCacheService satisfies cache.CacheService with an in-memory map.
type stubCacheService struct {
	callRecorder

	mu   sync.Mutex
	data map[string][]byte
}

func (s *stubCacheService) Get(_ context.Context, req *cache.CacheGetRequest) (*cache.CacheGetResponse, error) {
	s.record("Get")
	s.mu.Lock()
	defer s.mu.Unlock()

	value, found := s.data[req.Key]
	if !found {
		return &cache.CacheGetResponse{Found: false}, nil
	}

	return &cache.CacheGetResponse{Value: value, Found: true}, nil
}

func (s *stubCacheService) Set(_ context.Context, req *cache.CacheSetRequest) (*cache.CacheSetResponse, error) {
	s.record("Set")
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[req.Key] = req.Value

	return &cache.CacheSetResponse{Success: true}, nil
}

func (s *stubCacheService) Delete(
	_ context.Context,
	req *cache.CacheDeleteRequest,
) (*cache.CacheDeleteResponse, error) {
	s.record("Delete")
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, req.Key)

	return &cache.CacheDeleteResponse{Success: true}, nil
}

// stubCryptoService satisfies crypto.CryptoService with deterministic output.
type stubCryptoService struct {
	callRecorder

	mu      sync.Mutex
	counter uint64
}

func (s *stubCryptoService) RandomUint64(
	_ context.Context,
	req *crypto.RandomUint64Request,
) (*crypto.RandomUint64Response, error) {
	s.record("RandomUint64")
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	value := s.counter
	if req.Max > 0 {
		value %= req.Max
	}

	return &crypto.RandomUint64Response{Value: value}, nil
}

func (s *stubCryptoService) RandomString(
	_ context.Context,
	req *crypto.RandomStringRequest,
) (*crypto.RandomStringResponse, error) {
	s.record("RandomString")

	charset := "abcdefghijklmnopqrstuvwxyz0123456789"
	if req.Charset != nil && *req.Charset != "" {
		charset = *req.Charset
	}

	length := int(req.Length)
	length = max(length, 0)

	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteByte(charset[i%len(charset)])
	}

	return &crypto.RandomStringResponse{Value: b.String()}, nil
}

func (s *stubCryptoService) Argon2Hash(
	_ context.Context,
	req *crypto.Argon2HashRequest,
) (*crypto.Argon2HashResponse, error) {
	s.record("Argon2Hash")

	return &crypto.Argon2HashResponse{Hash: stubArgon2Hash(req.Password)}, nil
}

func (s *stubCryptoService) Argon2Verify(
	_ context.Context,
	req *crypto.Argon2VerifyRequest,
) (*crypto.Argon2VerifyResponse, error) {
	s.record("Argon2Verify")
	match := subtle.ConstantTimeCompare([]byte(stubArgon2Hash(req.Password)), []byte(req.Hash)) == 1

	return &crypto.Argon2VerifyResponse{Match: match}, nil
}

// stubArgon2Hash derives a deterministic stand-in hash from the password.
func stubArgon2Hash(password string) string {
	sum := sha256.Sum256([]byte(password))

	return "$argon2id$v=19$stub$" + hex.EncodeToString(sum[:])
}

// stubHTTPService satisfies http.HTTPService; Fetch answers a fixed 200 JSON response.
type stubHTTPService struct {
	callRecorder
}

func (s *stubHTTPService) Fetch(_ context.Context, _ *http.HTTPFetchRequest) (*http.HTTPFetchResponse, error) {
	s.record("Fetch")

	return &http.HTTPFetchResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"ok":true}`),
	}, nil
}

// stubStorageService satisfies storage.StorageService with an in-memory map.
type stubStorageService struct {
	callRecorder

	mu   sync.Mutex
	data map[string][]byte
}

func (s *stubStorageService) Get(
	_ context.Context,
	req *storage.StorageGetRequest,
) (*storage.StorageGetResponse, error) {
	s.record("Get")
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, found := s.data[req.Key]
	if !found {
		return &storage.StorageGetResponse{Found: false}, nil
	}

	return &storage.StorageGetResponse{Payload: payload, Found: true}, nil
}

func (s *stubStorageService) Set(
	_ context.Context,
	req *storage.StorageSetRequest,
) (*storage.StorageSetResponse, error) {
	s.record("Set")
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[req.Key] = req.Payload

	return &storage.StorageSetResponse{Success: true}, nil
}

func (s *stubStorageService) Delete(
	_ context.Context,
	req *storage.StorageDeleteRequest,
) (*storage.StorageDeleteResponse, error) {
	s.record("Delete")
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, req.Key)

	return &storage.StorageDeleteResponse{Success: true}, nil
}

func (s *stubStorageService) List(
	_ context.Context,
	req *storage.StorageListRequest,
) (*storage.StorageListResponse, error) {
	s.record("List")
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(s.data))
	for key := range s.data {
		if req.KeyPrefix != nil && !strings.HasPrefix(key, *req.KeyPrefix) {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)

	entries := make([]*storage.StorageEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, &storage.StorageEntry{Key: key, Payload: s.data[key]})
	}

	return &storage.StorageListResponse{Entries: entries}, nil
}

// stubServersService satisfies servers.ServersService with fake responses.
type stubServersService struct {
	callRecorder
}

func stubServer(id uint64) *domainproto.Server {
	return &domainproto.Server{Id: id, Name: "test-server"}
}

func (s *stubServersService) FindServers(
	_ context.Context,
	_ *servers.FindServersRequest,
) (*servers.FindServersResponse, error) {
	s.record("FindServers")

	return &servers.FindServersResponse{Servers: []*domainproto.Server{stubServer(1)}, Total: 1}, nil
}

func (s *stubServersService) GetServer(
	_ context.Context,
	req *servers.GetServerRequest,
) (*servers.GetServerResponse, error) {
	s.record("GetServer")

	return &servers.GetServerResponse{Server: stubServer(req.Id), Found: true}, nil
}

func (s *stubServersService) SaveServer(
	_ context.Context,
	req *servers.SaveServerRequest,
) (*servers.SaveServerResponse, error) {
	s.record("SaveServer")

	return &servers.SaveServerResponse{Success: true, Id: req.Server.GetId()}, nil
}

func (s *stubServersService) DeleteServer(
	_ context.Context,
	_ *servers.DeleteServerRequest,
) (*servers.DeleteServerResponse, error) {
	s.record("DeleteServer")

	return &servers.DeleteServerResponse{Success: true}, nil
}

// stubUsersService satisfies users.UsersService with fake responses.
type stubUsersService struct {
	callRecorder
}

func (s *stubUsersService) FindUsers(
	_ context.Context,
	_ *users.FindUsersRequest,
) (*users.FindUsersResponse, error) {
	s.record("FindUsers")

	return &users.FindUsersResponse{Users: []*domainproto.User{{Id: 1, Login: "admin"}}, Total: 1}, nil
}

func (s *stubUsersService) GetUser(
	_ context.Context,
	req *users.GetUserRequest,
) (*users.GetUserResponse, error) {
	s.record("GetUser")

	return &users.GetUserResponse{User: &domainproto.User{Id: req.Id, Login: "admin"}, Found: true}, nil
}

// stubGamesService satisfies games.GamesService with fake responses.
type stubGamesService struct {
	callRecorder
}

func (s *stubGamesService) FindGames(
	_ context.Context,
	_ *games.FindGamesRequest,
) (*games.FindGamesResponse, error) {
	s.record("FindGames")

	return &games.FindGamesResponse{
		Games: []*domainproto.Game{{Code: "cs", Name: "Counter-Strike"}},
		Total: 1,
	}, nil
}

func (s *stubGamesService) GetGame(
	_ context.Context,
	req *games.GetGameRequest,
) (*games.GetGameResponse, error) {
	s.record("GetGame")

	return &games.GetGameResponse{
		Game:  &domainproto.Game{Code: req.Code, Name: "Counter-Strike"},
		Found: true,
	}, nil
}

// stubGameModsService satisfies gamemods.GameModsService with fake responses.
type stubGameModsService struct {
	callRecorder
}

func (s *stubGameModsService) FindGameMods(
	_ context.Context,
	_ *gamemods.FindGameModsRequest,
) (*gamemods.FindGameModsResponse, error) {
	s.record("FindGameMods")

	return &gamemods.FindGameModsResponse{
		GameMods: []*domainproto.GameMod{{Id: 1, GameCode: "cs", Name: "Public"}},
		Total:    1,
	}, nil
}

func (s *stubGameModsService) GetGameMod(
	_ context.Context,
	req *gamemods.GetGameModRequest,
) (*gamemods.GetGameModResponse, error) {
	s.record("GetGameMod")

	return &gamemods.GetGameModResponse{
		GameMod: &domainproto.GameMod{Id: req.Id, GameCode: "cs", Name: "Public"},
		Found:   true,
	}, nil
}

// stubNodesService satisfies nodes.NodesService with fake responses.
type stubNodesService struct {
	callRecorder
}

func (s *stubNodesService) FindNodes(
	_ context.Context,
	_ *nodes.FindNodesRequest,
) (*nodes.FindNodesResponse, error) {
	s.record("FindNodes")

	return &nodes.FindNodesResponse{Nodes: []*domainproto.Node{{Id: 1, Name: "node-1"}}, Total: 1}, nil
}

func (s *stubNodesService) GetNode(
	_ context.Context,
	req *nodes.GetNodeRequest,
) (*nodes.GetNodeResponse, error) {
	s.record("GetNode")

	return &nodes.GetNodeResponse{Node: &domainproto.Node{Id: req.Id, Name: "node-1"}, Found: true}, nil
}

// stubServerSettingsService satisfies serversettings.ServerSettingsService with fake responses.
type stubServerSettingsService struct {
	callRecorder
}

func (s *stubServerSettingsService) FindServerSettings(
	_ context.Context,
	req *serversettings.FindServerSettingsRequest,
) (*serversettings.FindServerSettingsResponse, error) {
	s.record("FindServerSettings")

	return &serversettings.FindServerSettingsResponse{
		Settings: []*domainproto.ServerSetting{{Id: 1, ServerId: req.ServerId, Name: "hostname", Value: "test-server"}},
	}, nil
}

func (s *stubServerSettingsService) SaveServerSetting(
	_ context.Context,
	_ *serversettings.SaveServerSettingRequest,
) (*serversettings.SaveServerSettingResponse, error) {
	s.record("SaveServerSetting")

	return &serversettings.SaveServerSettingResponse{Success: true}, nil
}

// stubDaemonTasksService satisfies daemontasks.DaemonTasksService with fake responses.
type stubDaemonTasksService struct {
	callRecorder
}

func (s *stubDaemonTasksService) FindDaemonTasks(
	_ context.Context,
	_ *daemontasks.FindDaemonTasksRequest,
) (*daemontasks.FindDaemonTasksResponse, error) {
	s.record("FindDaemonTasks")

	return &daemontasks.FindDaemonTasksResponse{}, nil
}

func (s *stubDaemonTasksService) CreateDaemonTask(
	_ context.Context,
	_ *daemontasks.CreateDaemonTaskRequest,
) (*daemontasks.CreateDaemonTaskResponse, error) {
	s.record("CreateDaemonTask")

	return &daemontasks.CreateDaemonTaskResponse{Success: true, TaskId: 1}, nil
}

// stubServerControlService satisfies servercontrol.ServerControlService; every
// control call succeeds and returns the same daemon task id.
type stubServerControlService struct {
	callRecorder
}

func (s *stubServerControlService) success(method string) *servercontrol.ServerControlResponse {
	s.record(method)

	return &servercontrol.ServerControlResponse{Success: true, TaskId: new(uint64(1))}
}

func (s *stubServerControlService) StartServer(
	_ context.Context,
	_ *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.success("StartServer"), nil
}

func (s *stubServerControlService) StopServer(
	_ context.Context,
	_ *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.success("StopServer"), nil
}

func (s *stubServerControlService) RestartServer(
	_ context.Context,
	_ *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.success("RestartServer"), nil
}

func (s *stubServerControlService) UpdateServer(
	_ context.Context,
	_ *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.success("UpdateServer"), nil
}

func (s *stubServerControlService) InstallServer(
	_ context.Context,
	_ *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.success("InstallServer"), nil
}

func (s *stubServerControlService) ReinstallServer(
	_ context.Context,
	_ *servercontrol.ServerControlRequest,
) (*servercontrol.ServerControlResponse, error) {
	return s.success("ReinstallServer"), nil
}

// stubNodeFSService satisfies nodefs.NodeFSService with fake filesystem responses.
type stubNodeFSService struct {
	callRecorder
}

func (s *stubNodeFSService) ReadDir(
	_ context.Context,
	_ *nodefs.ReadDirRequest,
) (*nodefs.ReadDirResponse, error) {
	s.record("ReadDir")

	return &nodefs.ReadDirResponse{
		Files: []*nodefs.FileInfo{{
			Name:        "server.properties",
			Size:        12,
			Type:        nodefs.FileType_FILE_TYPE_FILE,
			Permissions: 0o644,
		}},
	}, nil
}

func (s *stubNodeFSService) MkDir(
	_ context.Context,
	_ *nodefs.MkDirRequest,
) (*nodefs.MkDirResponse, error) {
	s.record("MkDir")

	return &nodefs.MkDirResponse{Success: true}, nil
}

func (s *stubNodeFSService) Copy(
	_ context.Context,
	_ *nodefs.CopyRequest,
) (*nodefs.CopyResponse, error) {
	s.record("Copy")

	return &nodefs.CopyResponse{Success: true}, nil
}

func (s *stubNodeFSService) Move(
	_ context.Context,
	_ *nodefs.MoveRequest,
) (*nodefs.MoveResponse, error) {
	s.record("Move")

	return &nodefs.MoveResponse{Success: true}, nil
}

func (s *stubNodeFSService) Download(
	_ context.Context,
	_ *nodefs.DownloadRequest,
) (*nodefs.DownloadResponse, error) {
	s.record("Download")

	return &nodefs.DownloadResponse{Content: []byte("stub file content")}, nil
}

func (s *stubNodeFSService) Upload(
	_ context.Context,
	_ *nodefs.UploadRequest,
) (*nodefs.UploadResponse, error) {
	s.record("Upload")

	return &nodefs.UploadResponse{Success: true}, nil
}

func (s *stubNodeFSService) Remove(
	_ context.Context,
	_ *nodefs.RemoveRequest,
) (*nodefs.RemoveResponse, error) {
	s.record("Remove")

	return &nodefs.RemoveResponse{Success: true}, nil
}

func (s *stubNodeFSService) GetFileInfo(
	_ context.Context,
	_ *nodefs.GetFileInfoRequest,
) (*nodefs.GetFileInfoResponse, error) {
	s.record("GetFileInfo")

	return &nodefs.GetFileInfoResponse{
		File: &nodefs.FileDetails{
			Name:        "server.properties",
			Size:        12,
			Type:        nodefs.FileType_FILE_TYPE_FILE,
			Permissions: 0o644,
		},
		Found: true,
	}, nil
}

func (s *stubNodeFSService) Chmod(
	_ context.Context,
	_ *nodefs.ChmodRequest,
) (*nodefs.ChmodResponse, error) {
	s.record("Chmod")

	return &nodefs.ChmodResponse{Success: true}, nil
}

// stubNodeCmdService satisfies nodecmd.NodeCmdService; every command exits 0.
type stubNodeCmdService struct {
	callRecorder
}

func (s *stubNodeCmdService) ExecuteCommand(
	_ context.Context,
	_ *nodecmd.ExecuteCommandRequest,
) (*nodecmd.ExecuteCommandResponse, error) {
	s.record("ExecuteCommand")

	return &nodecmd.ExecuteCommandResponse{ExitCode: 0, Output: "ok"}, nil
}
