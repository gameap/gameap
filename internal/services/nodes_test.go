package services

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nodeEventCall is one dispatched node event. The service passes the live node
// it goes on to mutate, so the fields that matter are snapshotted at call time
// instead of being read back through a shared pointer afterwards.
type nodeEventCall struct {
	eventType    pluginproto.EventType
	nodeID       uint
	nodeName     string
	deletedAtSet bool
}

// fakeNodeDispatcher stands in for the plugin host: the only external
// collaborator of NodeService that cannot be run for real in a unit test.
type fakeNodeDispatcher struct {
	veto NodeEventVeto

	mu    sync.Mutex
	calls []nodeEventCall
}

func (d *fakeNodeDispatcher) DispatchNodeEvent(
	_ context.Context,
	eventType pluginproto.EventType,
	node *domain.Node,
	_ map[string]string,
) NodeEventVeto {
	d.record(eventType, node)

	return d.veto
}

func (d *fakeNodeDispatcher) DispatchNodeEventAsync(
	_ context.Context,
	eventType pluginproto.EventType,
	node *domain.Node,
	_ map[string]string,
) {
	d.record(eventType, node)
}

func (d *fakeNodeDispatcher) record(eventType pluginproto.EventType, node *domain.Node) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.calls = append(d.calls, nodeEventCall{
		eventType:    eventType,
		nodeID:       node.ID,
		nodeName:     node.Name,
		deletedAtSet: node.DeletedAt != nil,
	})
}

func (d *fakeNodeDispatcher) snapshot() []nodeEventCall {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]nodeEventCall(nil), d.calls...)
}

func (d *fakeNodeDispatcher) eventTypes() []pluginproto.EventType {
	calls := d.snapshot()

	types := make([]pluginproto.EventType, 0, len(calls))
	for _, call := range calls {
		types = append(types, call.eventType)
	}

	return types
}

type nodeServiceFixture struct {
	service *NodeService
	nodes   *inmemory.NodeRepository
	servers *inmemory.ServerRepository
	// node is the seeded node, with the id the repository assigned it.
	node *domain.Node
}

// setupNodeService builds a NodeService over real in-memory repositories and seeds one
// node, so every case starts from a node that exists.
func setupNodeService(t *testing.T, opts ...NodeServiceOption) *nodeServiceFixture {
	t.Helper()

	nodes := inmemory.NewNodeRepository()
	servers := inmemory.NewServerRepository()

	node := &domain.Node{
		Enabled:       true,
		Name:          "Frankfurt",
		OS:            domain.NodeOSLinux,
		Location:      "Germany",
		Provider:      new("Hetzner"),
		IPs:           domain.IPList{"192.0.2.10", "192.0.2.11"},
		WorkPath:      "/srv/gameap",
		SteamcmdPath:  new("/srv/gameap/steamcmd"),
		GdaemonHost:   "192.0.2.10",
		GdaemonPort:   31717,
		GdaemonAPIKey: "api-key",
		Metadata:      domain.Metadata{"rack": "a1"},
	}
	require.NoError(t, nodes.Save(context.Background(), node))
	require.NotZero(t, node.ID, "the repository must assign an id to the seeded node")

	return &nodeServiceFixture{
		service: NewNodeService(nodes, servers, opts...),
		nodes:   nodes,
		servers: servers,
		node:    node,
	}
}

// unknownNodeID is an id no seeded node has.
func (f *nodeServiceFixture) unknownNodeID() uint {
	return f.node.ID + 1000
}

// storedNode reads the node straight from the repository, soft-deleted ones
// included, so a test can tell "marked deleted" from "row dropped".
func (f *nodeServiceFixture) storedNode(t *testing.T) domain.Node {
	t.Helper()

	found, err := f.nodes.Find(
		context.Background(),
		&filters.FindNode{IDs: []uint{f.node.ID}, WithDeleted: true},
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, found, 1, "the node row must still be stored")

	return found[0]
}

// generateMetadata builds a bag of distinctly named keys.
func generateMetadata(prefix string, keys int) domain.Metadata {
	metadata := make(domain.Metadata, keys)
	for i := range keys {
		metadata[prefix+strconv.Itoa(i)] = i
	}

	return metadata
}

func (f *nodeServiceFixture) seedServerOnNode(t *testing.T) {
	t.Helper()

	server := &domain.Server{
		Enabled:   true,
		Name:      "Public #1",
		GameID:    "cstrike",
		DSID:      f.node.ID,
		GameModID: 1,
	}
	require.NoError(t, f.servers.Save(context.Background(), server))
}

func TestNodeService_Get(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		useUnknownID bool
		wantSentinel error
		wantError    string
	}{
		{
			name: "returns_the_stored_node",
		},
		{
			name:         "reports_an_unknown_id_as_not_found",
			useUnknownID: true,
			wantSentinel: ErrNodeNotFound,
			wantError:    "node not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := setupNodeService(t)
			id := f.node.ID
			if tt.useUnknownID {
				id = f.unknownNodeID()
			}

			// ACT
			got, err := f.service.Get(context.Background(), id)

			// ASSERT
			if tt.wantError != "" {
				require.ErrorIs(t, err, tt.wantSentinel)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "no node must be returned alongside an error")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, f.node.ID, got.ID)
			assert.Equal(t, "Frankfurt", got.Name)
			assert.Equal(t, "Germany", got.Location)
			assert.Equal(t, new("Hetzner"), got.Provider)
			assert.Equal(t, domain.IPList{"192.0.2.10", "192.0.2.11"}, got.IPs)
			assert.Equal(t, "/srv/gameap", got.WorkPath)
			assert.True(t, got.Enabled)
			assert.Equal(t, domain.Metadata{"rack": "a1"}, got.Metadata)
		})
	}
}

func TestNodeService_Patch_RejectsInvalidPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		patch        domain.NodePatch
		wantSentinel error
		wantError    string
	}{
		{
			name:         "rejects_an_empty_name",
			patch:        domain.NodePatch{Name: new("")},
			wantSentinel: domain.ErrNodeNameRequired,
			wantError:    "name must not be empty",
		},
		{
			name:         "rejects_a_blank_name",
			patch:        domain.NodePatch{Name: new("   ")},
			wantSentinel: domain.ErrNodeNameRequired,
			wantError:    "name must not be empty",
		},
		{
			name:         "rejects_a_name_wider_than_the_column",
			patch:        domain.NodePatch{Name: new(strings.Repeat("a", domain.NodeNameMaxLength+1))},
			wantSentinel: domain.ErrNodeNameTooLong,
			wantError:    "name is too long",
		},
		{
			name:         "rejects_an_empty_location",
			patch:        domain.NodePatch{Location: new("")},
			wantSentinel: domain.ErrNodeLocationRequired,
			wantError:    "location must not be empty",
		},
		{
			name:         "rejects_an_empty_work_path",
			patch:        domain.NodePatch{WorkPath: new("")},
			wantSentinel: domain.ErrNodeWorkPathRequired,
			wantError:    "work_path must not be empty",
		},
		{
			name:         "rejects_an_address_that_is_neither_ip_nor_hostname",
			patch:        domain.NodePatch{IPs: []string{"not a host"}},
			wantSentinel: domain.ErrNodeIPInvalid,
			wantError:    "ip is not a valid address or hostname",
		},
		{
			name:         "rejects_an_empty_metadata_key",
			patch:        domain.NodePatch{Metadata: domain.Metadata{"": "value"}},
			wantSentinel: domain.ErrNodeMetadataKeyEmpty,
			wantError:    "metadata key must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			f := setupNodeService(t)
			before := f.storedNode(t)

			// ACT
			got, err := f.service.Patch(context.Background(), f.node.ID, tt.patch)

			// ASSERT
			require.ErrorIs(t, err, tt.wantSentinel)
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			assert.Nil(t, got, "no node must be returned alongside an error")
			assert.Equal(t, before, f.storedNode(t), "a rejected patch must leave the stored node untouched")
		})
	}
}

func TestNodeService_Patch(t *testing.T) {
	t.Parallel()

	t.Run("applies_the_given_fields_and_keeps_the_rest", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		f := setupNodeService(t)
		patch := domain.NodePatch{
			Enabled:  new(false),
			Name:     new("Berlin"),
			Location: new("Berlin, Germany"),
		}

		// ACT
		got, err := f.service.Patch(context.Background(), f.node.ID, patch)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "Berlin", got.Name)
		assert.Equal(t, "Berlin, Germany", got.Location)
		assert.False(t, got.Enabled)
		assert.Equal(t, "/srv/gameap", got.WorkPath, "a field the patch omits keeps its stored value")
		assert.Equal(t, domain.IPList{"192.0.2.10", "192.0.2.11"}, got.IPs)
		assert.Equal(t, new("Hetzner"), got.Provider)

		stored := f.storedNode(t)
		assert.Equal(t, "Berlin", stored.Name, "the new name must be persisted")
		assert.Equal(t, "Berlin, Germany", stored.Location)
		assert.False(t, stored.Enabled)
		assert.Equal(t, "/srv/gameap", stored.WorkPath, "a field the patch omits stays as stored")
		assert.Equal(t, domain.IPList{"192.0.2.10", "192.0.2.11"}, stored.IPs)
		assert.Equal(t, new("/srv/gameap/steamcmd"), stored.SteamcmdPath)
		assert.Equal(t, domain.Metadata{"rack": "a1"}, stored.Metadata)
		assert.Nil(t, stored.DeletedAt, "a patch must not delete the node")
	})

	t.Run("trims_the_patched_strings_and_replaces_the_address_list", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		f := setupNodeService(t)
		patch := domain.NodePatch{
			Name:     new("  Munich  "),
			WorkPath: new(" /srv/games "),
			IPs:      []string{"198.51.100.7"},
		}

		// ACT
		got, err := f.service.Patch(context.Background(), f.node.ID, patch)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "Munich", got.Name)
		assert.Equal(t, "/srv/games", got.WorkPath)

		stored := f.storedNode(t)
		assert.Equal(t, "Munich", stored.Name)
		assert.Equal(t, "/srv/games", stored.WorkPath)
		assert.Equal(t, domain.IPList{"198.51.100.7"}, stored.IPs, "a non-empty address list replaces the stored one")
	})

	t.Run("merges_metadata_and_applies_the_removal_list", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		f := setupNodeService(t)
		patch := domain.NodePatch{
			Metadata:           domain.Metadata{"zone": "eu-central"},
			RemoveMetadataKeys: []string{"rack"},
		}

		// ACT
		got, err := f.service.Patch(context.Background(), f.node.ID, patch)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, domain.Metadata{"zone": "eu-central"}, got.Metadata)
		assert.Equal(t, domain.Metadata{"zone": "eu-central"}, f.storedNode(t).Metadata)
	})

	// The patch alone stays inside the key limit; only the merge with what is
	// already stored crosses it, which is what the bound has to catch.
	t.Run("rejects_a_metadata_merge_that_crosses_the_key_limit", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		f := setupNodeService(t)
		f.node.Metadata = generateMetadata("stored", domain.NodeMetadataMaxKeys/2+1)
		require.NoError(t, f.nodes.Save(context.Background(), f.node))
		before := f.storedNode(t)

		patch := domain.NodePatch{Metadata: generateMetadata("patched", domain.NodeMetadataMaxKeys/2+1)}

		// ACT
		got, err := f.service.Patch(context.Background(), f.node.ID, patch)

		// ASSERT
		require.ErrorIs(t, err, domain.ErrNodeMetadataTooLarge)
		assert.Nil(t, got, "no node must be returned alongside an error")
		assert.Equal(t, before, f.storedNode(t), "an over-large merge must not be persisted")
	})

	t.Run("leaves_every_field_as_stored_for_an_empty_patch", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		f := setupNodeService(t)
		before := f.storedNode(t)

		// ACT
		got, err := f.service.Patch(context.Background(), f.node.ID, domain.NodePatch{})

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, before.Name, got.Name)
		assert.Equal(t, before.Location, got.Location)
		assert.Equal(t, before.WorkPath, got.WorkPath)
		assert.Equal(t, before.IPs, got.IPs)
		assert.Equal(t, before.Metadata, got.Metadata)
	})

	t.Run("reports_an_unknown_id_as_not_found", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		f := setupNodeService(t)

		// ACT
		got, err := f.service.Patch(
			context.Background(), f.unknownNodeID(), domain.NodePatch{Name: new("Berlin")},
		)

		// ASSERT
		require.ErrorIs(t, err, ErrNodeNotFound)
		assert.Nil(t, got, "no node must be returned alongside an error")
		assert.Equal(t, "Frankfurt", f.storedNode(t).Name, "an unrelated node must not be patched")
	})
}

func TestNodeService_SoftDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		useUnknownID   bool
		seedServer     bool
		withDispatcher bool
		veto           NodeEventVeto
		wantSentinel   error
		wantError      string
		wantDeleted    bool
		wantEvents     []pluginproto.EventType
	}{
		{
			name:           "reports_an_unknown_id_as_not_found",
			useUnknownID:   true,
			withDispatcher: true,
			wantSentinel:   ErrNodeNotFound,
			wantError:      "node not found",
		},
		{
			name:           "refuses_to_delete_a_node_that_still_hosts_a_server",
			seedServer:     true,
			withDispatcher: true,
			wantSentinel:   ErrNodeHasServers,
			wantError:      "cannot delete node with existing game servers",
		},
		{
			name:        "marks_the_node_deleted_without_a_plugin_dispatcher",
			wantDeleted: true,
		},
		{
			name:           "marks_the_node_deleted_and_notifies_plugins",
			withDispatcher: true,
			wantDeleted:    true,
			wantEvents: []pluginproto.EventType{
				pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE,
				pluginproto.EventType_EVENT_TYPE_NODE_DELETED,
			},
		},
		{
			name:           "keeps_the_node_when_a_plugin_vetoes_the_deletion",
			withDispatcher: true,
			veto:           NodeEventVeto{Cancelled: true, CancelledBy: "plugin-x", CancelMessage: "busy"},
			wantSentinel:   ErrNodeDeleteCancelledByPlugin,
			wantError:      "cancelled by plugin-x: busy",
			wantEvents:     []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE},
		},
		{
			// A veto without a message must not leave an empty reason segment
			// behind: "cancelled by plugin-x: : node deletion..." would.
			name:           "names_only_the_plugin_when_a_veto_carries_no_message",
			withDispatcher: true,
			veto:           NodeEventVeto{Cancelled: true, CancelledBy: "plugin-x"},
			wantSentinel:   ErrNodeDeleteCancelledByPlugin,
			wantError:      "cancelled by plugin-x: node deletion cancelled by plugin",
			wantEvents:     []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE},
		},
		{
			// A dispatcher that answers "not cancelled" must not be mistaken for
			// a veto just because it filled in the other fields.
			name:           "deletes_the_node_when_the_dispatcher_reports_no_veto",
			withDispatcher: true,
			veto:           NodeEventVeto{CancelledBy: "plugin-x", CancelMessage: "busy"},
			wantDeleted:    true,
			wantEvents: []pluginproto.EventType{
				pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE,
				pluginproto.EventType_EVENT_TYPE_NODE_DELETED,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			var (
				dispatcher *fakeNodeDispatcher
				opts       []NodeServiceOption
			)
			if tt.withDispatcher {
				dispatcher = &fakeNodeDispatcher{veto: tt.veto}
				opts = append(opts, WithNodePluginEvents(dispatcher))
			}

			f := setupNodeService(t, opts...)
			if tt.seedServer {
				f.seedServerOnNode(t)
			}

			id := f.node.ID
			if tt.useUnknownID {
				id = f.unknownNodeID()
			}

			deleteStartedAt := time.Now()

			// ACT
			err := f.service.SoftDelete(context.Background(), id)

			// ASSERT
			if tt.wantError != "" {
				require.ErrorIs(t, err, tt.wantSentinel)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
			}

			stored := f.storedNode(t)
			if tt.wantDeleted {
				require.NotNil(t, stored.DeletedAt, "the node must be marked deleted")
				assert.False(t, stored.DeletedAt.Before(deleteStartedAt), "deleted_at must be stamped by this call")

				_, getErr := f.service.Get(context.Background(), f.node.ID)
				require.ErrorIs(t, getErr, ErrNodeNotFound, "a soft-deleted node must no longer be readable")
			} else {
				assert.Nil(t, stored.DeletedAt, "the node must keep its live state")
			}

			if !tt.withDispatcher {
				return
			}

			assert.Equal(t, tt.wantEvents, nonEmptyEventTypes(dispatcher), "dispatched node events mismatch")
			for _, call := range dispatcher.snapshot() {
				assert.Equal(t, f.node.ID, call.nodeID, "every event must carry the node being deleted")
				assert.Equal(t, "Frankfurt", call.nodeName, "every event must carry the stored node data")
			}
		})
	}
}

// nonEmptyEventTypes keeps the table's nil "no events expected" comparable to
// what the fake collected.
func nonEmptyEventTypes(dispatcher *fakeNodeDispatcher) []pluginproto.EventType {
	types := dispatcher.eventTypes()
	if len(types) == 0 {
		return nil
	}

	return types
}

func TestNodeService_SoftDelete_EventOrder(t *testing.T) {
	t.Parallel()

	// ARRANGE
	dispatcher := &fakeNodeDispatcher{}
	f := setupNodeService(t, WithNodePluginEvents(dispatcher))

	// ACT
	err := f.service.SoftDelete(context.Background(), f.node.ID)

	// ASSERT
	require.NoError(t, err)

	calls := dispatcher.snapshot()
	require.Len(t, calls, 2)
	assert.Equal(t, pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE, calls[0].eventType)
	assert.False(t, calls[0].deletedAtSet, "the veto is asked before the node is marked deleted")
	assert.Equal(t, pluginproto.EventType_EVENT_TYPE_NODE_DELETED, calls[1].eventType)
	assert.True(t, calls[1].deletedAtSet, "the notification carries the already deleted node")
}
