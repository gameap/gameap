package hostlibrary

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nodesWriteEnv struct {
	service *NodesServiceImpl
	nodes   *inmemory.NodeRepository
	servers *inmemory.ServerRepository
	tickets *enrollment.TicketStore
	audit   *auditCapture
}

// auditCapture records the events a host library emits so the tests can assert
// the plugin is named as the actor.
type auditCapture struct {
	events []audit.Event
}

func (c *auditCapture) Record(_ context.Context, event audit.Event) {
	c.events = append(c.events, event)
}

func newNodesWriteEnv(t *testing.T, checker PluginPermissionChecker, resolver ConnectTargetResolver) nodesWriteEnv {
	t.Helper()

	nodeRepo := inmemory.NewNodeRepository()
	serverRepo := inmemory.NewServerRepository()
	tickets := enrollment.NewTicketStore(cache.NewInMemory())
	auditLogger := &auditCapture{}

	return nodesWriteEnv{
		service: NewNodesService(
			nodesTestPluginID,
			nodeRepo,
			services.NewNodeService(nodeRepo, serverRepo),
			tickets,
			resolver,
			checker,
			auditLogger,
		),
		nodes:   nodeRepo,
		servers: serverRepo,
		tickets: tickets,
		audit:   auditLogger,
	}
}

func seedNode(t *testing.T, repo *inmemory.NodeRepository) *domain.Node {
	t.Helper()

	node := &domain.Node{
		Enabled:             true,
		Name:                "node-1",
		OS:                  domain.NodeOSLinux,
		Location:            "fsn1",
		IPs:                 domain.IPList{"203.0.113.10"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "203.0.113.10",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "hashed-api-key",
		GdaemonAPIToken:     new("hashed-api-token"),
		GdaemonPassword:     new("enc:secret"),
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 3,
		PreferInstallMethod: domain.NodePreferInstallMethodAuto,
		ScriptStart:         new("start {id}"),
		Metadata:            domain.Metadata{"region": "fsn1"},
	}
	require.NoError(t, repo.Save(context.Background(), node))

	return node
}

// stubConnectResolver stands in for the panel's gRPC address configuration.
type stubConnectResolver struct {
	target enrollment.ConnectTarget
	err    error
}

func (r stubConnectResolver) Resolve(fallbackHost string) (enrollment.ConnectTarget, error) {
	if r.err != nil {
		return enrollment.ConnectTarget{}, r.err
	}

	target := r.target
	if target.Host == "" {
		target.Host = fallbackHost
	}

	return target, nil
}

func testResolver() stubConnectResolver {
	return stubConnectResolver{target: enrollment.ConnectTarget{Host: "panel.example.com", Port: 31718}}
}

// TestNodesService_WritesRequireManageNodes: the grant is the only thing
// standing between an installed plugin and someone else's infrastructure, so
// every mutating entry point must consult it.
func TestNodesService_WritesRequireManageNodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	env := newNodesWriteEnv(t, stubPermissionChecker{allowed: false}, testResolver())
	node := seedNode(t, env.nodes)

	t.Run("update_node", func(t *testing.T) {
		t.Parallel()
		resp, err := env.service.UpdateNode(ctx, &nodes.UpdateNodeRequest{Id: uint64(node.ID), Name: new("hacked")})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "plugin permission manage_nodes required")
	})

	t.Run("delete_node", func(t *testing.T) {
		t.Parallel()
		resp, err := env.service.DeleteNode(ctx, &nodes.DeleteNodeRequest{Id: uint64(node.ID)})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "manage_nodes")
	})

	t.Run("create_setup_key", func(t *testing.T) {
		t.Parallel()
		resp, err := env.service.CreateSetupKey(ctx, &nodes.CreateSetupKeyRequest{})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "manage_nodes")
		assert.Empty(t, resp.SetupKey)
	})

	t.Run("get_setup_key", func(t *testing.T) {
		t.Parallel()
		resp, err := env.service.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: "whatever"})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
	})

	t.Run("revoke_setup_key", func(t *testing.T) {
		t.Parallel()
		resp, err := env.service.RevokeSetupKey(ctx, &nodes.RevokeSetupKeyRequest{TicketId: "whatever"})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
	})

	// Reads stay open for backward compatibility.
	found, err := env.service.GetNode(ctx, &nodes.GetNodeRequest{Id: uint64(node.ID)})
	require.NoError(t, err)
	assert.True(t, found.Found, "reads must keep working without the grant")

	stored, err := env.nodes.Find(ctx, filters.FindNodeByIDs(node.ID), nil, nil)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "node-1", stored[0].Name, "a denied update must not touch the record")
}

func TestNodesService_UpdateNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name      string
		request   func(nodeID uint) *nodes.UpdateNodeRequest
		wantError string
		assert    func(t *testing.T, node *domain.Node)
	}{
		{
			name: "renames_and_keeps_credentials",
			request: func(id uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{Id: uint64(id), Name: new("renamed"), Enabled: new(false)}
			},
			assert: func(t *testing.T, node *domain.Node) {
				t.Helper()
				assert.Equal(t, "renamed", node.Name)
				assert.False(t, node.Enabled)
				assert.Equal(t, "hashed-api-key", node.GdaemonAPIKey, "credentials are not writable by plugins")
				require.NotNil(t, node.GdaemonPassword)
				assert.Equal(t, "enc:secret", *node.GdaemonPassword)
				assert.Equal(t, "certs/root.crt", node.GdaemonServerCert)
				assert.Equal(t, uint(3), node.ClientCertificateID)
				require.NotNil(t, node.ScriptStart)
				assert.Equal(t, "start {id}", *node.ScriptStart)
			},
		},
		{
			name: "merges_metadata_and_keeps_untouched_keys",
			request: func(id uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{
					Id:       uint64(id),
					Metadata: map[string]string{"hetzner.server_id": "42"},
				}
			},
			assert: func(t *testing.T, node *domain.Node) {
				t.Helper()
				assert.Equal(t, "42", node.Metadata["hetzner.server_id"])
				assert.Equal(t, "fsn1", node.Metadata["region"], "keys the plugin did not name must survive")
			},
		},
		{
			name: "removes_named_metadata_keys",
			request: func(id uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{Id: uint64(id), RemoveMetadataKeys: []string{"region"}}
			},
			assert: func(t *testing.T, node *domain.Node) {
				t.Helper()
				assert.Empty(t, node.Metadata)
			},
		},
		{
			name: "replaces_ip_list",
			request: func(id uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{Id: uint64(id), Ips: []string{"203.0.113.20"}}
			},
			assert: func(t *testing.T, node *domain.Node) {
				t.Helper()
				assert.Equal(t, domain.IPList{"203.0.113.20"}, node.IPs)
			},
		},
		{
			name: "rejects_empty_name",
			request: func(id uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{Id: uint64(id), Name: new("  ")}
			},
			wantError: "name must not be empty",
		},
		{
			name: "rejects_invalid_ip",
			request: func(id uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{Id: uint64(id), Ips: []string{"not a host!"}}
			},
			wantError: "ip is not a valid address or hostname",
		},
		{
			name: "unknown_node",
			request: func(uint) *nodes.UpdateNodeRequest {
				return &nodes.UpdateNodeRequest{Id: 9999, Name: new("x")}
			},
			wantError: "node not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, testResolver())
			node := seedNode(t, env.nodes)

			resp, err := env.service.UpdateNode(ctx, tt.request(node.ID))
			require.NoError(t, err)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			require.True(t, resp.Success, resp.Error)
			require.NotNil(t, resp.Node)

			stored, err := env.nodes.Find(ctx, filters.FindNodeByIDs(node.ID), nil, nil)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			tt.assert(t, &stored[0])

			require.Len(t, env.audit.events, 1)
			assert.Equal(t, audit.EventNodeUpdate, env.audit.events[0].Type)
			assert.Equal(t, audit.AuthMethodPlugin, env.audit.events[0].AuthMethod)
			assert.Equal(t, "plugin:7", env.audit.events[0].ActorLogin)
		})
	}
}

func TestNodesService_DeleteNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("soft_deletes_and_audits", func(t *testing.T) {
		t.Parallel()
		env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, testResolver())
		node := seedNode(t, env.nodes)

		resp, err := env.service.DeleteNode(ctx, &nodes.DeleteNodeRequest{Id: uint64(node.ID)})
		require.NoError(t, err)
		require.True(t, resp.Success, resp.Error)

		visible, err := env.nodes.Find(ctx, filters.FindNodeByIDs(node.ID), nil, nil)
		require.NoError(t, err)
		assert.Empty(t, visible)

		withDeleted, err := env.nodes.Find(ctx, &filters.FindNode{IDs: []uint{node.ID}, WithDeleted: true}, nil, nil)
		require.NoError(t, err)
		require.Len(t, withDeleted, 1)
		assert.NotNil(t, withDeleted[0].DeletedAt, "the row must survive as a soft delete")

		require.Len(t, env.audit.events, 1)
		assert.Equal(t, audit.EventNodeDelete, env.audit.events[0].Type)
	})

	t.Run("refuses_while_game_servers_remain", func(t *testing.T) {
		t.Parallel()
		env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, testResolver())
		node := seedNode(t, env.nodes)

		require.NoError(t, env.servers.Save(ctx, &domain.Server{
			Enabled: true,
			Name:    "server-1",
			GameID:  "cstrike",
			DSID:    node.ID,
			Dir:     "servers/1",
		}))

		resp, err := env.service.DeleteNode(ctx, &nodes.DeleteNodeRequest{Id: uint64(node.ID)})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "existing game servers")

		visible, err := env.nodes.Find(ctx, filters.FindNodeByIDs(node.ID), nil, nil)
		require.NoError(t, err)
		require.Len(t, visible, 1, "the node must stay alive while it hosts servers")
	})

	t.Run("unknown_node", func(t *testing.T) {
		t.Parallel()
		env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, testResolver())

		resp, err := env.service.DeleteNode(ctx, &nodes.DeleteNodeRequest{Id: 4242})
		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Contains(t, *resp.Error, "node not found")
	})
}

func TestNodesService_SetupKeyLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, testResolver())

	created, err := env.service.CreateSetupKey(ctx, &nodes.CreateSetupKeyRequest{
		Presets: &nodes.NodePresets{
			Name:     new("hz-fsn1-1"),
			Location: new("fsn1"),
			Metadata: map[string]string{"hetzner.server_id": "42"},
		},
		TtlSeconds: 3600,
	})
	require.NoError(t, err)
	require.True(t, created.Success, created.Error)

	assert.NotEmpty(t, created.SetupKey)
	assert.NotEmpty(t, created.TicketId)
	assert.Equal(t, "grpc://panel.example.com:31718/"+created.SetupKey, created.ConnectUrl)
	assert.Contains(t, created.InstallScript, "CONNECT_URL='grpc://panel.example.com:31718/")
	assert.Contains(t, created.InstallCommand, "daemon install --connect=")
	assert.Positive(t, created.ExpiresAt)

	pending, err := env.service.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	require.True(t, pending.Found)
	assert.Equal(t, nodes.SetupKeyStatus_SETUP_KEY_STATUS_PENDING, pending.Status)
	assert.Nil(t, pending.NodeId)

	// A daemon enrolls with the key; the ticket now points at its node.
	ticket, err := env.tickets.Resolve(ctx, created.SetupKey)
	require.NoError(t, err)
	require.NoError(t, env.tickets.Consume(ctx, ticket, 17))

	enrolled, err := env.service.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	require.True(t, enrolled.Found)
	assert.Equal(t, nodes.SetupKeyStatus_SETUP_KEY_STATUS_ENROLLED, enrolled.Status)
	require.NotNil(t, enrolled.NodeId)
	assert.Equal(t, uint64(17), *enrolled.NodeId)

	revoked, err := env.service.RevokeSetupKey(ctx, &nodes.RevokeSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	assert.True(t, revoked.Success, revoked.Error)

	gone, err := env.service.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	assert.False(t, gone.Found)
}

// TestNodesService_SetupKeyIsolation: a ticket belongs to the plugin that
// issued it. Another plugin must not be able to read its state or revoke it,
// and must not even learn that it exists.
func TestNodesService_SetupKeyIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, testResolver())

	created, err := env.service.CreateSetupKey(ctx, &nodes.CreateSetupKeyRequest{TtlSeconds: 3600})
	require.NoError(t, err)
	require.True(t, created.Success, created.Error)

	other := NewNodesService(
		nodesTestPluginID+1,
		env.nodes,
		services.NewNodeService(env.nodes, env.servers),
		env.tickets,
		testResolver(),
		stubPermissionChecker{allowed: true},
		nil,
	)

	got, err := other.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	assert.False(t, got.Found, "another plugin's ticket must be invisible")

	revoked, err := other.RevokeSetupKey(ctx, &nodes.RevokeSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	assert.False(t, revoked.Success)

	still, err := env.service.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: created.TicketId})
	require.NoError(t, err)
	assert.True(t, still.Found, "the owner's ticket must survive a foreign revoke")
}

func TestNodesService_CreateSetupKeyValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name      string
		resolver  ConnectTargetResolver
		request   *nodes.CreateSetupKeyRequest
		wantError string
	}{
		{
			name:      "unresolved_connect_host",
			resolver:  stubConnectResolver{err: enrollment.ErrConnectHostUnresolved},
			request:   &nodes.CreateSetupKeyRequest{TtlSeconds: 3600},
			wantError: "gRPC connect host is not configured",
		},
		{
			name:      "invalid_connect_host_override",
			resolver:  testResolver(),
			request:   &nodes.CreateSetupKeyRequest{TtlSeconds: 3600, ConnectHost: new("not a host!")},
			wantError: "connect_host is not a valid address or hostname",
		},
		{
			name:      "ttl_above_the_cap",
			resolver:  testResolver(),
			request:   &nodes.CreateSetupKeyRequest{TtlSeconds: uint32((48 * time.Hour).Seconds())},
			wantError: "ttl out of range",
		},
		{
			name:     "invalid_preset",
			resolver: testResolver(),
			request: &nodes.CreateSetupKeyRequest{
				TtlSeconds: 3600,
				Presets:    &nodes.NodePresets{Name: new(" ")},
			},
			wantError: "name must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, tt.resolver)

			resp, err := env.service.CreateSetupKey(ctx, tt.request)

			require.NoError(t, err)
			assert.False(t, resp.Success)
			require.NotNil(t, resp.Error)
			assert.Contains(t, *resp.Error, tt.wantError)
			assert.Empty(t, resp.SetupKey)
		})
	}
}

// TestNodesService_CreateSetupKeySurfacesCertificateWarning: an operator whose
// gRPC certificate does not cover the connect host would see daemons fail TLS,
// so the warning must reach the plugin instead of only the panel log.
func TestNodesService_CreateSetupKeySurfacesCertificateWarning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	resolver := stubConnectResolver{target: enrollment.ConnectTarget{
		Host:     "panel.example.com",
		Port:     31718,
		Warnings: []string{enrollment.CertHostWarning("panel.example.com")},
	}}
	env := newNodesWriteEnv(t, stubPermissionChecker{allowed: true}, resolver)

	resp, err := env.service.CreateSetupKey(ctx, &nodes.CreateSetupKeyRequest{TtlSeconds: 3600})

	require.NoError(t, err)
	require.True(t, resp.Success, resp.Error)
	require.Len(t, resp.Warnings, 1)
	assert.Contains(t, resp.Warnings[0], "not covered by the panel gRPC TLS certificate")
}
