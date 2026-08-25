// Behavioural tests of the node path policy in the host libraries: a refused
// path answers a stable error, is audited and counted, and never reaches the
// daemon.
package hostlibrary

import (
	"context"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedWorkPathNode(r *inmemory.NodeRepository) {
	_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux, WorkPath: "/home/servers"})
}

// refusingFileService fails the test when any daemon operation is reached.
func refusingFileService(t *testing.T) *mockFileService {
	t.Helper()

	fail := func(string) {
		t.Helper()
		t.Fatal("daemon must not be reached for a path the policy refuses")
	}

	return &mockFileService{
		readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
			fail("read_dir")

			return nil, nil
		},
		copyFunc: func(_ context.Context, _ *domain.Node, _, _ string) error {
			fail("copy")

			return nil
		},
		uploadFunc: func(context.Context, *domain.Node, string, []byte, os.FileMode) error {
			fail("upload")

			return nil
		},
	}
}

func workPathPolicy(t *testing.T) *PathPolicy {
	t.Helper()

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath}, nil)
	require.NoError(t, err)

	return policy
}

func TestNodeFSService_path_policy_denies_audits_and_does_not_forward(t *testing.T) {
	t.Parallel()

	repo := setupNodeFSRepo(seedWorkPathNode)
	recorder := &auditRecorder{}
	denials := &denialRecorder{}
	guard := NewGuard(grantSet{domain.PluginPermissionFiles: true},
		WithGuardAudit(recorder), WithGuardObserver(denials)).For(testPluginID)

	svc := NewNodeFSService(testPluginID, refusingFileService(t), repo, newMockArchiveService(), &mockRegistrar{},
		guard, WithNodeFSPathPolicy(workPathPolicy(t)))

	const outside = "path policy: path outside allowed roots (node_workpath): /etc/passwd"

	readDir, err := svc.ReadDir(context.Background(), &nodefs.ReadDirRequest{NodeId: 1, Path: "/etc/passwd"})
	require.NoError(t, err)
	require.NotNil(t, readDir.Error)
	assert.Equal(t, outside, *readDir.Error)

	copied, err := svc.Copy(context.Background(), &nodefs.CopyRequest{
		NodeId: 1, Source: "/home/servers/cs/map.bsp", Destination: "/etc/passwd",
	})
	require.NoError(t, err)
	assert.False(t, copied.Success)
	assert.Equal(t, outside, *copied.Error)

	uploaded, err := svc.Upload(context.Background(), &nodefs.UploadRequest{
		NodeId: 1, Path: "/home/servers/../../etc/passwd", Content: []byte("x"),
	})
	require.NoError(t, err)
	assert.False(t, uploaded.Success)
	assert.Equal(t, `path policy: path contains ".." segment: /home/servers/../../etc/passwd`, *uploaded.Error)

	hashed, err := svc.Hash(context.Background(), &nodefs.HashRequest{
		NodeId: 1, Paths: []string{"/home/servers/cs/a", "/srv/other/b"},
	})
	require.NoError(t, err)
	assert.False(t, hashed.Success)
	assert.Contains(t, *hashed.Error, "/srv/other/b")

	archive, err := svc.StartCreateArchive(context.Background(), &nodefs.CreateArchiveRequest{
		NodeId: 1, ArchivePath: "/home/servers/backup.zip", BasePath: "/home/servers", Sources: []string{"/var/log"},
	})
	require.NoError(t, err)
	assert.False(t, archive.Success)
	assert.Contains(t, *archive.Error, "/var/log")

	extract, err := svc.StartExtractArchive(context.Background(), &nodefs.ExtractArchiveRequest{
		NodeId: 1, ArchivePath: "/home/servers/backup.zip", Destination: "/opt",
	})
	require.NoError(t, err)
	assert.False(t, extract.Success)
	assert.Contains(t, *extract.Error, "/opt")

	assert.Equal(t, []string{
		"gameap-nodefs.read_dir:path_policy",
		"gameap-nodefs.copy:path_policy",
		"gameap-nodefs.upload:path_policy",
		"gameap-nodefs.hash:path_policy",
		"gameap-nodefs.start_create_archive:path_policy",
		"gameap-nodefs.start_extract_archive:path_policy",
	}, denials.all())

	events := recorder.all()
	require.Len(t, events, 6)
	for _, event := range events {
		assert.Equal(t, audit.EventAccessDenied, event.Type)
		assert.Equal(t, audit.OutcomeDenied, event.Outcome)
		assert.Equal(t, "plugin_path_policy", event.Reason)
		assert.Equal(t, "node_workpath", attrValue(event, "mode"))
		assert.NotEmpty(t, attrValue(event, "path"))
	}
}

func TestNodeFSService_path_policy_allows_paths_inside_the_work_path(t *testing.T) {
	t.Parallel()

	repo := setupNodeFSRepo(seedWorkPathNode)
	var seen string
	fs := &mockFileService{
		readDirFunc: func(_ context.Context, _ *domain.Node, path string) ([]*daemon.FileInfo, error) {
			seen = path

			return nil, nil
		},
	}

	svc := NewNodeFSService(testPluginID, fs, repo, newMockArchiveService(), &mockRegistrar{},
		allowAllGuard(testPluginID), WithNodeFSPathPolicy(workPathPolicy(t)))

	resp, err := svc.ReadDir(context.Background(), &nodefs.ReadDirRequest{NodeId: 1, Path: "cs/maps"})
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.Equal(t, "cs/maps", seen, "the path is forwarded as the plugin sent it")
}

func TestNodeCmdService_work_dir_policy(t *testing.T) {
	t.Parallel()

	repo := setupNodeFSRepo(seedWorkPathNode)
	recorder := &auditRecorder{}
	executed := 0
	cmd := &mockCommandService{executeFunc: func(
		context.Context, *domain.Node, string, ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error) {
		executed++

		return &daemon.CommandResult{}, nil
	}}
	guard := NewGuard(grantSet{domain.PluginPermissionNodeCommands: true}, WithGuardAudit(recorder)).For(testPluginID)
	svc := NewNodeCmdService(testPluginID, cmd, repo, guard, WithNodeCmdPathPolicy(workPathPolicy(t)))

	resp, err := svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
		NodeId: 1, Command: "ls", WorkDir: new("/etc"),
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "path policy: path outside allowed roots (node_workpath): /etc", *resp.Error)

	resp, err = svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
		NodeId: 1, Command: "ls", WorkDir: new("cs"),
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "path policy: path must be absolute: cs", *resp.Error)
	assert.Equal(t, 0, executed)

	events := recorder.all()
	require.Len(t, events, 1, "the second denial within a minute is throttled")
	assert.Equal(t, audit.EventAccessDenied, events[0].Type)
	assert.Equal(t, "plugin_path_policy", events[0].Reason)

	resp, err = svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
		NodeId: 1, Command: "ls", WorkDir: new("/home/servers/cs"),
	})
	require.NoError(t, err)
	assert.Nil(t, resp.Error)

	resp, err = svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{NodeId: 1, Command: "ls"})
	require.NoError(t, err)
	assert.Nil(t, resp.Error, "no working directory: the daemon's default is not subject to the policy")
	assert.Equal(t, 2, executed)
}

// An empty base_path asks for the entries to keep their full paths; it names
// no location, so checking it as one resolves to the node work path and every
// restricted mode refuses a call over a path the plugin never sent.
func TestNodeFSService_archive_without_a_base_path_is_not_refused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	nodes := setupNodeFSRepo(seedWorkPathNode)

	servers := inmemory.NewServerRepository()
	require.NoError(t, servers.Save(ctx, &domain.Server{Name: "cs", DSID: 1, Dir: "servers/cs"}))

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyServerDirs}, servers)
	require.NoError(t, err)

	archive := newMockArchiveService()
	svc := NewNodeFSService(testPluginID, refusingFileService(t), nodes, archive, &mockRegistrar{},
		allowAllGuard(testPluginID), WithNodeFSPathPolicy(policy))

	resp, err := svc.StartCreateArchive(ctx, &nodefs.CreateArchiveRequest{
		NodeId:      1,
		ArchivePath: "/home/servers/servers/cs/backup.zip",
		Sources:     []string{"/home/servers/servers/cs/cfg"},
	})
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.True(t, resp.Success)

	calls := archive.StartCalls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].create)
	assert.Empty(t, calls[0].create.BasePath, "the empty base path reaches the daemon unchanged")
}

// The production wiring reaches gameap-nodefs through the factory, not through
// NewNodeFSService, and the factory silently defaults to the unrestricted
// policy when the option is not passed. That is exactly how the policy came to
// be enforced on gameap-nodecmd and on file references but not on gameap-nodefs
// itself, so the propagation is pinned here rather than left to the direct
// constructor the other tests use.
func TestNodeFSHostLibraryFactory_propagates_the_path_policy(t *testing.T) {
	t.Parallel()

	repo := setupNodeFSRepo(seedWorkPathNode)
	fs := refusingFileService(t)

	factory := NewNodeFSHostLibraryFactory(fs, repo, newMockArchiveService(), &mockRegistrar{},
		NewGuard(grantSet{domain.PluginPermissionFiles: true}),
		WithNodeFSPathPolicy(workPathPolicy(t)))

	library, ok := factory.Create(testPluginID).(*NodeFSHostLibrary)
	require.True(t, ok, "the factory builds a NodeFSHostLibrary")

	resp, err := library.impl.ReadDir(context.Background(),
		&nodefs.ReadDirRequest{NodeId: 1, Path: "/etc/shadow"})
	require.NoError(t, err)
	require.NotNil(t, resp.Error, "a path outside the work path must be refused")
	assert.Contains(t, *resp.Error, "path policy")
}
