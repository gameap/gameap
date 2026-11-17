package daemon

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfineToServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workPath  string
		serverDir string
		wantRoot  string
	}{
		{
			name:      "relative_server_dir",
			workPath:  "/srv/gameap",
			serverDir: "servers/tenant-a",
			wantRoot:  "servers/tenant-a",
		},
		{
			name:      "server_dir_with_leading_and_trailing_slashes",
			workPath:  "/srv/gameap",
			serverDir: "/servers/tenant-a/",
			wantRoot:  "servers/tenant-a",
		},
		{
			name:      "work_path_with_trailing_slash",
			workPath:  "/srv/gameap/",
			serverDir: "servers/tenant-a",
			wantRoot:  "servers/tenant-a",
		},
		{
			name:      "server_dir_is_cleaned",
			workPath:  "/srv/gameap",
			serverDir: "servers/./shared/../tenant-a",
			wantRoot:  "servers/tenant-a",
		},
		{
			name:      "windows_node",
			workPath:  `C:\gameap`,
			serverDir: `servers\tenant-a`,
			wantRoot:  "servers/tenant-a",
		},
		{
			name:      "server_in_the_work_path_itself",
			workPath:  "/srv/gameap",
			serverDir: "",
			wantRoot:  ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := ConfineToServer(
				context.Background(),
				&domain.Node{WorkPath: tt.workPath},
				&domain.Server{Dir: tt.serverDir},
			)

			assert.Equal(t, tt.wantRoot, ConfinementRoot(ctx))
		})
	}
}

// The root is spelled exactly like the paths FileService sends, so a daemon
// comparing the two finds every path of the server inside the root.
func TestConfineToServer_RootIsAPrefixOfTheServerPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workPath  string
		serverDir string
		fullPath  string
	}{
		{
			name:      "linux_node",
			workPath:  "/srv/gameap",
			serverDir: "servers/tenant-a",
			fullPath:  "/srv/gameap/servers/tenant-a/cstrike/server.cfg",
		},
		{
			name:      "windows_node",
			workPath:  `C:\gameap`,
			serverDir: `servers\tenant-a`,
			fullPath:  `C:\gameap/servers\tenant-a/cstrike/server.cfg`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			node := &domain.Node{WorkPath: tt.workPath}
			root := ConfinementRoot(ConfineToServer(context.Background(), node, &domain.Server{Dir: tt.serverDir}))

			assert.Equal(t, root+"/cstrike/server.cfg", stripWorkPath(node.WorkPath, tt.fullPath))
		})
	}
}

func TestConfinementRoot_ContextWithoutRoot(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ConfinementRoot(context.Background()))

	ctx := context.Background()
	assert.Equal(t, ctx, WithConfinementRoot(ctx, ""), "an empty root leaves the choice to the daemon")
}

func TestWithConfinementRoot_InnermostWins(t *testing.T) {
	t.Parallel()

	ctx := WithConfinementRoot(context.Background(), "servers")
	ctx = WithConfinementRoot(ctx, "servers/tenant-a")

	assert.Equal(t, "servers/tenant-a", ConfinementRoot(ctx))
}

// A request dispatched to the instance that owns the daemon session is executed
// there on a context of its own: the root has to cross pub/sub with it.
func TestFileDispatcher_CarriesTheConfinementRootToTheOwningInstance(t *testing.T) {
	t.Parallel()

	const nodeID uint64 = 21

	operations := []struct {
		name     string
		dispatch func(ctx context.Context, d FileDispatcher) error
	}{
		{
			name: "file_list",
			dispatch: func(ctx context.Context, d FileDispatcher) error {
				_, err := d.DispatchFileList(ctx, nodeID, "servers/tenant-a/cstrike", false, "")

				return err
			},
		},
		{
			name: "file_read",
			dispatch: func(ctx context.Context, d FileDispatcher) error {
				_, err := d.DispatchFileRead(ctx, nodeID, "servers/tenant-a/server.cfg", 0, 0)

				return err
			},
		},
		{
			name: "file_write",
			dispatch: func(ctx context.Context, d FileDispatcher) error {
				return d.DispatchFileWrite(
					ctx, nodeID, "servers/tenant-a/server.cfg", []byte("x"), 0o644, true, OwnerOptions{},
				)
			},
		},
		{
			name: "file_operation",
			dispatch: func(ctx context.Context, d FileDispatcher) error {
				_, err := d.DispatchFileOperation(ctx, nodeID, &proto.FileOperationRequest{
					Operation: proto.FileOperationType_FILE_OPERATION_TYPE_DELETE,
					Parameters: &proto.FileOperationRequest_DeleteParams{
						DeleteParams: &proto.DeleteParams{Path: "servers/tenant-a/old.cfg"},
					},
				})

				return err
			},
		},
		{
			name: "upload_task",
			dispatch: func(ctx context.Context, d FileDispatcher) error {
				return d.DispatchUploadTask(ctx, nodeID, "xfer-up", "servers/tenant-a/map.bsp", 0o644, OwnerOptions{})
			},
		},
		{
			name: "download_task",
			dispatch: func(ctx context.Context, d FileDispatcher) error {
				return d.DispatchDownloadTask(ctx, nodeID, "xfer-down", "servers/tenant-a/map.bsp")
			},
		},
	}

	roots := []struct {
		name string
		root string
	}{
		{name: "confined_to_the_server", root: "servers/tenant-a"},
		{name: "no_root_named", root: ""},
	}

	for _, op := range operations {
		for _, rt := range roots {
			t.Run(op.name+"/"+rt.name, func(t *testing.T) {
				t.Parallel()

				ps := memory.New()
				t.Cleanup(func() { _ = ps.Close() })

				storage := files.NewInMemoryFileManager()
				logger := slog.New(slog.DiscardHandler)

				initiatorRegistry := newFakeConnectionChecker()
				initiator := NewFileDispatcher(ps, &fakeFileGateway{}, initiatorRegistry, storage, "inst-a", logger)

				ownerGateway := &fakeFileGateway{}
				ownerRegistry := newFakeConnectionChecker()
				ownerRegistry.setConnected(nodeID, true)
				owner := NewFileDispatcher(ps, ownerGateway, ownerRegistry, storage, "inst-b", logger)

				startCtx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				require.NoError(t, initiator.Start(startCtx))
				require.NoError(t, owner.Start(startCtx))

				seen := make(chan string, 1)
				capture := func(ctx context.Context) { seen <- ConfinementRoot(ctx) }

				ownerGateway.requestFileList = func(
					ctx context.Context, _ uint64, _ string, _ bool, _ string,
				) (*proto.FileListResponse, error) {
					capture(ctx)

					return &proto.FileListResponse{Success: true}, nil
				}
				ownerGateway.requestFileRead = func(
					ctx context.Context, _ uint64, _ string, _ int64, _ int64,
				) (*proto.FileReadResponse, error) {
					capture(ctx)

					return &proto.FileReadResponse{Success: true}, nil
				}
				ownerGateway.requestFileWrite = func(
					ctx context.Context, _ uint64, _ string, _ []byte, _ int32, _ bool,
				) error {
					capture(ctx)

					return nil
				}
				ownerGateway.requestFileOp = func(
					ctx context.Context, _ uint64, _ *proto.FileOperationRequest,
				) (*proto.FileOperationResponse, error) {
					capture(ctx)

					return &proto.FileOperationResponse{Success: true}, nil
				}
				ownerGateway.requestUploadTask = func(ctx context.Context, _ uint64, _, _, _ string, _ int64) error {
					capture(ctx)

					return nil
				}
				ownerGateway.requestDownloadTask = func(ctx context.Context, _ uint64, _, _ string) error {
					capture(ctx)

					return nil
				}

				require.NoError(t, storage.Write(testContext(t), transferPrefix+"xfer-up/data", []byte("uploaded")))

				err := op.dispatch(WithConfinementRoot(testContext(t), rt.root), initiator)
				require.NoError(t, err)

				select {
				case got := <-seen:
					assert.Equal(t, rt.root, got)
				case <-time.After(2 * time.Second):
					require.FailNow(t, "the owning instance did not call its gateway")
				}
			})
		}
	}
}

// An archive operation outlives the request that started it and reaches the
// gateway on a context of its own, so the root is put into the request itself,
// where it also survives the hop to the instance owning the daemon session.
func TestArchiveService_StampsTheConfinementRoot(t *testing.T) {
	t.Parallel()

	const nodeID uint64 = 31

	starts := []struct {
		name  string
		start func(ctx context.Context, svc *ArchiveService) (string, error)
	}{
		{
			name: "create",
			start: func(ctx context.Context, svc *ArchiveService) (string, error) {
				return svc.StartCreate(ctx, testNode(uint(nodeID), "/srv"), CreateArchiveParams{
					ArchivePath: "/srv/servers/tenant-a/backup.zip",
					BasePath:    "/srv/servers/tenant-a",
					Sources:     []string{"/srv/servers/tenant-a/cstrike"},
					Format:      proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
				})
			},
		},
		{
			name: "extract",
			start: func(ctx context.Context, svc *ArchiveService) (string, error) {
				return svc.StartExtract(ctx, testNode(uint(nodeID), "/srv"), ExtractArchiveParams{
					ArchivePath: "/srv/servers/tenant-a/backup.zip",
					Destination: "/srv/servers/tenant-a",
				})
			},
		},
	}

	routes := []struct {
		name string
		// setup returns the service that starts the operation and the gateway
		// of the instance that owns the daemon session.
		setup func(t *testing.T) (*ArchiveService, *fakeArchiveGateway)
	}{
		{
			name: "session_on_this_instance",
			setup: func(t *testing.T) (*ArchiveService, *fakeArchiveGateway) {
				t.Helper()

				ps := memory.New()
				t.Cleanup(func() { _ = ps.Close() })

				s := setupArchiveService(t, ps, "inst-a")
				s.registry.setConnected(nodeID, true)
				s.registry.setArchiveCapability(nodeID)

				return s.service, s.gateway
			},
		},
		{
			name: "session_on_another_instance",
			setup: func(t *testing.T) (*ArchiveService, *fakeArchiveGateway) {
				t.Helper()

				ps := memory.New()
				t.Cleanup(func() { _ = ps.Close() })

				a := setupArchiveService(t, ps, "inst-a")
				b := setupArchiveService(t, ps, "inst-b")
				a.registry.connectedAnywhere[nodeID] = true
				b.registry.setConnected(nodeID, true)
				b.registry.setArchiveCapability(nodeID)

				return a.service, b.gateway
			},
		},
	}

	roots := []struct {
		name string
		root string
	}{
		{name: "confined_to_the_server", root: "servers/tenant-a"},
		{name: "no_root_named", root: ""},
	}

	for _, st := range starts {
		for _, route := range routes {
			for _, rt := range roots {
				t.Run(st.name+"/"+route.name+"/"+rt.name, func(t *testing.T) {
					t.Parallel()

					svc, ownerGateway := route.setup(t)

					opID, err := st.start(WithConfinementRoot(testContext(t), rt.root), svc)
					require.NoError(t, err)

					waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()

					_, err = svc.WaitCompletion(waitCtx, opID)
					require.NoError(t, err)

					requests := ownerGateway.Requests()
					require.Len(t, requests, 1)
					assert.Equal(t, opID, requests[0].GetRequestId())
					assert.Equal(t, rt.root, requests[0].GetRoot())
				})
			}
		}
	}
}
