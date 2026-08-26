package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePathPolicyMode(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]PathPolicyMode{
		"":              PathPolicyUnrestricted,
		"unrestricted":  PathPolicyUnrestricted,
		"Node_WorkPath": PathPolicyNodeWorkPath,
		"SERVER_DIRS":   PathPolicyServerDirs,
	} {
		mode, err := ParsePathPolicyMode(input)
		require.NoError(t, err, input)
		assert.Equal(t, want, mode, input)
	}

	mode, err := ParsePathPolicyMode("  server_dirs\n")
	require.NoError(t, err)
	assert.Equal(t, PathPolicyServerDirs, mode, "surrounding whitespace is ignored")

	_, err = ParsePathPolicyMode("everything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown path policy "everything"`)
}

func TestNewPathPolicy_validates_extra_roots(t *testing.T) {
	t.Parallel()

	policy, err := NewPathPolicy(PathPolicyConfig{
		Mode:         PathPolicyNodeWorkPath,
		AllowedPaths: []string{" /opt/shared/ ", "", `D:\Data\`},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"/opt/shared", "D:/Data"}, policy.extraRoots)

	_, err = NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath, AllowedPaths: []string{"relative/dir"}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be absolute")

	_, err = NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath, AllowedPaths: []string{"/opt/../etc"}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `".." segment`)

	_, err = NewPathPolicy(PathPolicyConfig{Mode: PathPolicyServerDirs}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server repository")

	assert.Equal(t, PathPolicyUnrestricted, DefaultPathPolicy().Mode())

	var nilPolicy *PathPolicy
	assert.Equal(t, PathPolicyUnrestricted, nilPolicy.Mode())
}

func TestNormalizeNodePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		workPath string
		want     string
		wantErr  string
	}{
		{name: "clean_absolute", raw: "/home/servers/cs", want: "/home/servers/cs"},
		{name: "trailing_slash_dropped", raw: "/home/servers/", want: "/home/servers"},
		{name: "root_stays_root", raw: "/", want: "/"},
		{name: "double_slashes_collapsed", raw: "/home//servers/./cs", want: "/home/servers/cs"},
		{name: "relative_under_work_path", raw: "cs/maps", workPath: "/home/servers", want: "/home/servers/cs/maps"},
		{name: "relative_without_work_path", raw: "cs/maps", want: "cs/maps"},
		{name: "backslashes", raw: `C:\gameap\servers\cs`, want: "C:/gameap/servers/cs"},
		{name: "drive_letter_upper_cased", raw: `c:/gameap/`, want: "C:/gameap"},
		{name: "dotdot_leading", raw: "../etc/passwd", workPath: "/home/servers", wantErr: `".." segment`},
		{name: "dotdot_middle", raw: "/home/servers/../../etc/passwd", wantErr: `".." segment`},
		{name: "dotdot_non_escaping", raw: "/home/servers/a/../b", wantErr: `".." segment`},
		{name: "dotdot_backslash", raw: `C:\gameap\..\Windows`, wantErr: `".." segment`},
		{name: "dots_in_name_are_fine", raw: "/home/servers/ok..ok/..hidden", want: "/home/servers/ok..ok/..hidden"},
		{name: "null_byte", raw: "/home/servers/a\x00b", wantErr: "null byte"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeNodePath(tt.raw, tt.workPath)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func linuxNode() *domain.Node {
	return &domain.Node{ID: 1, OS: domain.NodeOSLinux, WorkPath: "/home/servers"}
}

func TestPathScope_unrestricted_refuses_only_traversal(t *testing.T) {
	t.Parallel()

	scope, err := DefaultPathPolicy().ScopeFor(context.Background(), linuxNode(), testPluginID)
	require.NoError(t, err)

	assert.Nil(t, scope.Check("/etc/passwd"))
	assert.Nil(t, scope.Check("relative/path"))
	assert.Nil(t, scope.CheckWorkDir("relative/path"))

	denial := scope.Check("/home/servers/../etc/passwd")
	require.NotNil(t, denial)
	assert.Equal(t, `path policy: path contains ".." segment: /home/servers/../etc/passwd`, denial.Error())
	assert.Equal(t, PathPolicyUnrestricted, denial.Mode)
}

func TestPathScope_node_workpath(t *testing.T) {
	t.Parallel()

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath, AllowedPaths: []string{"/opt/shared"}}, nil)
	require.NoError(t, err)

	scope, err := policy.ScopeFor(context.Background(), linuxNode(), testPluginID)
	require.NoError(t, err)

	tests := []struct {
		raw  string
		deny bool
	}{
		{raw: "/home/servers"},
		{raw: "/home/servers/"},
		{raw: "/home/servers/cs/maps/de_dust2.bsp"},
		{raw: "cs/maps", deny: false},
		{raw: "/opt/shared/steamcmd"},
		{raw: "/home/servers2/cs", deny: true},
		{raw: "/home", deny: true},
		{raw: "/etc/passwd", deny: true},
		{raw: "/opt/shared-not", deny: true},
	}

	for _, tt := range tests {
		denial := scope.Check(tt.raw)
		if tt.deny {
			require.NotNil(t, denial, tt.raw)
			assert.Equal(t, "path policy: path outside allowed roots (node_workpath): "+tt.raw, denial.Error())

			continue
		}

		assert.Nil(t, denial, tt.raw)
	}

	workDir := scope.CheckWorkDir("cs/maps")
	require.NotNil(t, workDir)
	assert.Contains(t, workDir.Error(), "path must be absolute")
	assert.Nil(t, scope.CheckWorkDir("/home/servers/cs"))
}

func TestPathScope_windows_nodes_compare_case_insensitively(t *testing.T) {
	t.Parallel()

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath}, nil)
	require.NoError(t, err)

	scope, err := policy.ScopeFor(context.Background(), &domain.Node{ID: 2, OS: domain.NodeOSWindows, WorkPath: `C:\GameAP`}, testPluginID)
	require.NoError(t, err)

	assert.Nil(t, scope.Check(`c:\gameap\servers\cs`))
	assert.Nil(t, scope.Check(`C:/GameAP/servers`))
	assert.Nil(t, scope.Check(`servers\cs`))
	assert.NotNil(t, scope.Check(`D:\GameAP\servers`))
	assert.NotNil(t, scope.Check(`C:\GameAP2\servers`))
}

func TestPathScope_empty_work_path_fails_closed(t *testing.T) {
	t.Parallel()

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath}, nil)
	require.NoError(t, err)

	_, err = policy.ScopeFor(context.Background(), &domain.Node{ID: 3, OS: domain.NodeOSLinux}, testPluginID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "work path is not configured")
}

func TestPathScope_server_dirs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	servers := inmemory.NewServerRepository()
	require.NoError(t, servers.Save(ctx, &domain.Server{Name: "cs", DSID: 1, Dir: "servers/cs"}))
	require.NoError(t, servers.Save(ctx, &domain.Server{Name: "no-dir", DSID: 1, Dir: "  "}))
	require.NoError(t, servers.Save(ctx, &domain.Server{Name: "elsewhere", DSID: 9, Dir: "servers/other"}))

	deleted := &domain.Server{Name: "gone", DSID: 1, Dir: "servers/gone"}
	require.NoError(t, servers.Save(ctx, deleted))
	require.NoError(t, servers.Delete(ctx, deleted.ID))

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyServerDirs}, servers)
	require.NoError(t, err)

	scope, err := policy.ScopeFor(ctx, linuxNode(), testPluginID)
	require.NoError(t, err)

	assert.Nil(t, scope.Check("/home/servers/servers/cs/cfg/server.cfg"))
	assert.Nil(t, scope.Check("servers/cs/cfg/server.cfg"))
	assert.NotNil(t, scope.Check("/home/servers"), "the work path itself is not a server directory")
	assert.NotNil(t, scope.Check("/home/servers/servers/other/x"), "a server on another node does not open a root")
	assert.NotNil(t, scope.Check("/home/servers/servers/gone/x"), "a soft-deleted server keeps nothing open")
	assert.NotNil(t, scope.Check("/home/servers/servers/cs-backup"))
}

type failingServerLister struct{}

func (failingServerLister) Find(
	context.Context, *filters.FindServer, []filters.Sorting, *filters.Pagination,
) ([]domain.Server, error) {
	return nil, errors.New("database is down")
}

func TestPathScope_server_dirs_repository_failure_fails_closed(t *testing.T) {
	t.Parallel()

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyServerDirs}, failingServerLister{})
	require.NoError(t, err)

	_, err = policy.ScopeFor(context.Background(), linuxNode(), testPluginID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list the node's servers")
}

func TestPathScope_nil_node_or_policy_is_unrestricted(t *testing.T) {
	t.Parallel()

	var nilPolicy *PathPolicy

	scope, err := nilPolicy.ScopeFor(context.Background(), linuxNode(), testPluginID)
	require.NoError(t, err)
	assert.Nil(t, scope.Check("/anything"))

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyNodeWorkPath}, nil)
	require.NoError(t, err)

	scope, err = policy.ScopeFor(context.Background(), nil, testPluginID)
	require.NoError(t, err)
	assert.Nil(t, scope.Check("/anything"))
}

func TestPluginServiceDir(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/home/servers/.plugins/a4", PluginServiceDir("/home/servers", testPluginID))
	assert.Equal(t, "/home/servers/.plugins/fi", PluginServiceDir("/home/servers/", 42),
		"a trailing separator on the work path changes nothing")

	// A transient load (the upload dry-run) owns no directory, and neither
	// does a node with no work path configured: both answer empty so the
	// caller adds no root rather than one rooted at "/".
	assert.Empty(t, PluginServiceDir("/home/servers", 0))
	assert.Empty(t, PluginServiceDir("", testPluginID))
	assert.Empty(t, PluginServiceDir("   ", testPluginID))
}

// server_dirs is the strictest mode and names only the game servers' own
// directories. Without the service directory a plugin could not stage so much
// as a request file there, so every restricted mode keeps its own open.
func TestPathScope_server_dirs_keeps_the_plugins_own_service_dir_open(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerRepository()
	require.NoError(t, repo.Save(context.Background(),
		&domain.Server{Name: "cs2", DSID: 1, Dir: "servers/cs2"}))

	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyServerDirs}, repo)
	require.NoError(t, err)

	scope, err := policy.ScopeFor(context.Background(), linuxNode(), testPluginID)
	require.NoError(t, err)

	assert.Nil(t, scope.Check("/home/servers/.plugins/a4"))
	assert.Nil(t, scope.Check("/home/servers/.plugins/a4/req/req-1.json"))
	assert.Nil(t, scope.Check("/home/servers/servers/cs2/cfg/server.cfg"),
		"the server directories are still the point of the mode")

	// One plugin's scratch space is not another's.
	require.NotNil(t, scope.Check("/home/servers/.plugins/fi/req/req-1.json"))
	// And the root above them is nobody's.
	require.NotNil(t, scope.Check("/home/servers/.plugins"))
	require.NotNil(t, scope.Check("/home/servers/backups"))
}

// A transient load has no id, so it gets no service directory — it must not
// fall back to something broader.
func TestPathScope_a_transient_load_gets_no_service_dir(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewServerRepository()
	policy, err := NewPathPolicy(PathPolicyConfig{Mode: PathPolicyServerDirs}, repo)
	require.NoError(t, err)

	scope, err := policy.ScopeFor(context.Background(), linuxNode(), 0)
	require.NoError(t, err)

	assert.NotNil(t, scope.Check("/home/servers/.plugins/a4/req/req-1.json"))
	assert.NotNil(t, scope.Check("/home/servers"))
}
