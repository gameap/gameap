package hostlibrary

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodes"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

// nodesPermissionDeniedMessage is what a plugin without the grant sees on a
// mutating call. It names the missing permission so plugin authors know what
// to declare in their manifest.
const nodesPermissionDeniedMessage = "plugin permission " +
	string(domain.PluginPermissionManageNodes) + " required"

const enrollmentUnavailableMessage = "enrollment is not available"

// errInvalidConnectHost is reported when a plugin supplies a connect host the
// panel would refuse to put into a connect URL.
var errInvalidConnectHost = errors.New("connect_host is not a valid address or hostname")

type NodesServiceImpl struct {
	pluginID   uint64
	nodeRepo   repositories.NodeRepository
	nodes      NodeWriter
	tickets    NodeEnrollment
	connect    ConnectTargetResolver
	checker    PluginPermissionChecker
	auditLoger audit.Logger
}

func NewNodesService(
	pluginID uint64,
	nodeRepo repositories.NodeRepository,
	nodeWriter NodeWriter,
	tickets NodeEnrollment,
	connect ConnectTargetResolver,
	checker PluginPermissionChecker,
	auditLogger audit.Logger,
) *NodesServiceImpl {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &NodesServiceImpl{
		pluginID:   pluginID,
		nodeRepo:   nodeRepo,
		nodes:      nodeWriter,
		tickets:    tickets,
		connect:    connect,
		checker:    checker,
		auditLoger: auditLogger,
	}
}

// authorize gates the mutating methods of this module on the "manage_nodes"
// grant. Reads stay open: every plugin could already list nodes before the
// gate existed, and narrowing that would break installed plugins. Plugin ID 0
// (transient dry-run loads) is never granted anything.
func (s *NodesServiceImpl) authorize(ctx context.Context) (bool, string) {
	allowed, err := s.checker.Has(ctx, s.pluginID, domain.PluginPermissionManageNodes)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check plugin manage_nodes permission",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("error", err.Error()))

		return false, "failed to check plugin permission: " + err.Error()
	}

	if !allowed {
		slog.WarnContext(ctx, "plugin denied a write to the nodes host library",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("permission", string(domain.PluginPermissionManageNodes)))

		return false, nodesPermissionDeniedMessage
	}

	return true, ""
}

// auditActor identifies this plugin as the actor of an audited operation,
// the same identity the guard files for the other host libraries.
func (s *NodesServiceImpl) auditActor() audit.PluginActor {
	return audit.PluginActor{
		ID:   s.pluginID,
		Name: pkgplugin.CompactPluginID(domain.Uint64ID(s.pluginID)),
	}
}

// owner identifies this plugin as the issuer of an enrollment ticket, so one
// plugin cannot inspect or revoke another's.
func (s *NodesServiceImpl) owner() string {
	return "plugin:" + strconv.FormatUint(s.pluginID, 10)
}

func (s *NodesServiceImpl) FindNodes(
	ctx context.Context,
	req *nodes.FindNodesRequest,
) (*nodes.FindNodesResponse, error) {
	var filter *filters.FindNode
	if req.Filter != nil {
		filter = &filters.FindNode{
			IDs:     uintsFromUint64s(req.Filter.Ids),
			Enabled: req.Filter.Enabled,
		}

		if req.Filter.Os != nil {
			os := domain.ParseNodeOS(*req.Filter.Os)
			filter.OS = &os
		}
	}

	var pagination *filters.Pagination
	if req.Pagination != nil {
		pagination = &filters.Pagination{
			Limit:  uint64(req.Pagination.Limit),
			Offset: uint64(req.Pagination.Offset),
		}
	}

	sorting := convertSorting(req.Sorting)

	result, err := s.nodeRepo.Find(ctx, filter, sorting, pagination)
	if err != nil {
		return nil, err
	}

	return &nodes.FindNodesResponse{
		Nodes: convertNodesToProto(result),
		Total: int32(len(result)), //nolint:gosec
	}, nil
}

func (s *NodesServiceImpl) GetNode(
	ctx context.Context,
	req *nodes.GetNodeRequest,
) (*nodes.GetNodeResponse, error) {
	result, err := s.nodeRepo.Find(ctx, filters.FindNodeByIDs(uint(req.Id)), nil, nil)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return &nodes.GetNodeResponse{Found: false}, nil
	}

	return &nodes.GetNodeResponse{
		Node:  convertNodeToProto(&result[0]),
		Found: true,
	}, nil
}

// UpdateNode applies a partial update. Expected failures go into the response
// fields: returning an error from a host function traps the whole guest call.
func (s *NodesServiceImpl) UpdateNode(
	ctx context.Context,
	req *nodes.UpdateNodeRequest,
) (*nodes.UpdateNodeResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodes.UpdateNodeResponse{Error: new(msg)}, nil
	}

	patch := domain.NodePatch{
		Enabled:            req.Enabled,
		Name:               req.Name,
		Location:           req.Location,
		Provider:           req.Provider,
		WorkPath:           req.WorkPath,
		SteamcmdPath:       req.SteamcmdPath,
		IPs:                req.Ips,
		Metadata:           metadataFromStringMap(req.Metadata),
		RemoveMetadataKeys: req.RemoveMetadataKeys,
	}

	node, err := s.nodes.Patch(ctx, uint(req.Id), patch)
	if err != nil {
		return &nodes.UpdateNodeResponse{Error: new(err.Error())}, nil
	}

	audit.PluginOp(ctx, s.auditLoger, audit.EventNodeUpdate, audit.CategoryNodeOp,
		audit.OutcomeSuccess, s.auditActor(),
		"node", strconv.FormatUint(req.Id, 10), "update", "")

	return &nodes.UpdateNodeResponse{
		Success: true,
		Node:    convertNodeToProto(node),
	}, nil
}

// DeleteNode soft-deletes a node, refusing while game servers still reference
// it — the same guard the admin API applies.
func (s *NodesServiceImpl) DeleteNode(
	ctx context.Context,
	req *nodes.DeleteNodeRequest,
) (*nodes.DeleteNodeResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodes.DeleteNodeResponse{Error: new(msg)}, nil
	}

	if err := s.nodes.SoftDelete(ctx, uint(req.Id)); err != nil {
		return &nodes.DeleteNodeResponse{Error: new(err.Error())}, nil
	}

	audit.PluginOp(ctx, s.auditLoger, audit.EventNodeDelete, audit.CategoryNodeOp,
		audit.OutcomeSuccess, s.auditActor(),
		"node", strconv.FormatUint(req.Id, 10), "delete", "")

	return &nodes.DeleteNodeResponse{Success: true}, nil
}

// CreateSetupKey mints a single-use enrollment key plus the installer to run
// on the target machine.
func (s *NodesServiceImpl) CreateSetupKey(
	ctx context.Context,
	req *nodes.CreateSetupKeyRequest,
) (*nodes.CreateSetupKeyResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodes.CreateSetupKeyResponse{Error: new(msg)}, nil
	}

	if s.tickets == nil || s.connect == nil {
		return &nodes.CreateSetupKeyResponse{Error: new(enrollmentUnavailableMessage)}, nil
	}

	fallbackHost, err := connectHostOverride(req.ConnectHost)
	if err != nil {
		return &nodes.CreateSetupKeyResponse{Error: new(err.Error())}, nil
	}

	target, err := s.connect.Resolve(fallbackHost)
	if err != nil {
		return &nodes.CreateSetupKeyResponse{Error: new(err.Error())}, nil
	}

	if req.ConnectPort != nil && *req.ConnectPort > 0 {
		if *req.ConnectPort > 65535 {
			return &nodes.CreateSetupKeyResponse{Error: new("connect_port is out of range")}, nil
		}
		target.Port = uint16(*req.ConnectPort)
	}

	ticket, setupKey, err := s.tickets.Create(ctx, enrollment.CreateTicketInput{
		Owner:   s.owner(),
		Presets: presetsFromProto(req.Presets),
		TTL:     time.Duration(req.TtlSeconds) * time.Second,
	})
	if err != nil {
		return &nodes.CreateSetupKeyResponse{Error: new(err.Error())}, nil
	}

	connectURL := enrollment.FormatConnectURL(target.Host, target.Port, setupKey)
	scriptOpts := scriptOptionsFromProto(req.InstallScript)

	audit.PluginOp(ctx, s.auditLoger, audit.EventNodeSetupKeyCreate, audit.CategoryNodeOp,
		audit.OutcomeSuccess, s.auditActor(),
		"enroll_ticket", ticket.ID, "create", "",
		slog.Int64("ttl_seconds", int64(req.TtlSeconds)))

	return &nodes.CreateSetupKeyResponse{
		Success:        true,
		SetupKey:       setupKey,
		TicketId:       ticket.ID,
		ConnectUrl:     connectURL,
		InstallScript:  enrollment.BuildSetupScript(connectURL, scriptOpts),
		InstallCommand: enrollment.BuildInstallCommand(scriptOpts),
		ExpiresAt:      ticket.ExpiresAt.Unix(),
		Warnings:       target.Warnings,
	}, nil
}

// GetSetupKey reports the state of a ticket this plugin issued. Tickets of
// other issuers answer found=false rather than confirming their existence.
func (s *NodesServiceImpl) GetSetupKey(
	ctx context.Context,
	req *nodes.GetSetupKeyRequest,
) (*nodes.GetSetupKeyResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodes.GetSetupKeyResponse{Error: new(msg)}, nil
	}

	ticket, ok := s.ownedTicket(ctx, req.TicketId)
	if !ok {
		return &nodes.GetSetupKeyResponse{Success: true, Found: false}, nil
	}

	response := &nodes.GetSetupKeyResponse{
		Success:   true,
		Found:     true,
		Status:    nodes.SetupKeyStatus_SETUP_KEY_STATUS_PENDING,
		ExpiresAt: ticket.ExpiresAt.Unix(),
	}

	if ticket.Status == enrollment.TicketStatusConsumed {
		response.Status = nodes.SetupKeyStatus_SETUP_KEY_STATUS_ENROLLED
		response.NodeId = new(uint64(ticket.NodeID))
		if ticket.ConsumedAt != nil {
			response.EnrolledAt = ticket.ConsumedAt.Unix()
		}
	}

	return response, nil
}

func (s *NodesServiceImpl) RevokeSetupKey(
	ctx context.Context,
	req *nodes.RevokeSetupKeyRequest,
) (*nodes.RevokeSetupKeyResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &nodes.RevokeSetupKeyResponse{Error: new(msg)}, nil
	}

	if _, ok := s.ownedTicket(ctx, req.TicketId); !ok {
		return &nodes.RevokeSetupKeyResponse{Error: new("setup key not found")}, nil
	}

	if err := s.tickets.Revoke(ctx, req.TicketId); err != nil {
		return &nodes.RevokeSetupKeyResponse{Error: new(err.Error())}, nil
	}

	audit.PluginOp(ctx, s.auditLoger, audit.EventNodeSetupKeyRevoke, audit.CategoryNodeOp,
		audit.OutcomeSuccess, s.auditActor(),
		"enroll_ticket", req.TicketId, "revoke", "")

	return &nodes.RevokeSetupKeyResponse{Success: true}, nil
}

// ownedTicket loads a ticket only when this plugin issued it.
func (s *NodesServiceImpl) ownedTicket(ctx context.Context, ticketID string) (*enrollment.Ticket, bool) {
	if s.tickets == nil {
		return nil, false
	}

	ticket, err := s.tickets.Get(ctx, ticketID)
	if err != nil || ticket.Owner != s.owner() {
		return nil, false
	}

	return ticket, true
}

func connectHostOverride(host *string) (string, error) {
	if host == nil || *host == "" {
		return "", nil
	}

	normalized := enrollment.NormalizeHost(*host)
	if !validation.IsValidIPOrHostname(normalized) {
		return "", errInvalidConnectHost
	}

	return normalized, nil
}

func presetsFromProto(presets *nodes.NodePresets) enrollment.NodePresets {
	if presets == nil {
		return enrollment.NodePresets{}
	}

	return enrollment.NodePresets{
		Enabled:      presets.Enabled,
		Name:         presets.Name,
		Location:     presets.Location,
		Provider:     presets.Provider,
		WorkPath:     presets.WorkPath,
		SteamcmdPath: presets.SteamcmdPath,
		Metadata:     metadataFromStringMap(presets.Metadata),
	}
}

func scriptOptionsFromProto(opts *nodes.InstallScriptOptions) enrollment.SetupScriptOptions {
	if opts == nil {
		return enrollment.SetupScriptOptions{}
	}

	result := enrollment.SetupScriptOptions{GitHub: opts.Github}
	if opts.Config != nil {
		result.Config = *opts.Config
	}
	if opts.Branch != nil {
		result.Branch = *opts.Branch
	}

	return result
}

func metadataFromStringMap(in map[string]string) domain.Metadata {
	if len(in) == 0 {
		return nil
	}

	metadata := make(domain.Metadata, len(in))
	for key, value := range in {
		metadata[key] = value
	}

	return metadata
}

// metadataToStringMap renders the free-form bag for the wire. Plugins receive
// strings only: numbers and booleans are formatted, richer values fall back to
// their JSON text, so a plugin never has to unpack a dynamic type.
func metadataToStringMap(metadata domain.Metadata) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		switch v := value.(type) {
		case string:
			result[key] = v
		case nil:
			result[key] = ""
		default:
			encoded, err := json.Marshal(v)
			if err != nil {
				continue
			}
			result[key] = string(encoded)
		}
	}

	return result
}

func convertNodesToProto(nds []domain.Node) []*proto.Node {
	return lo.Map(nds, func(n domain.Node, _ int) *proto.Node {
		return convertNodeToProto(&n)
	})
}

func convertNodeToProto(n *domain.Node) *proto.Node {
	node := &proto.Node{
		Id:          uint64(n.ID),
		Name:        n.Name,
		Enabled:     n.Enabled,
		Os:          string(n.OS),
		Location:    n.Location,
		Provider:    n.Provider,
		Ips:         n.IPs,
		WorkPath:    n.WorkPath,
		GdaemonHost: n.GdaemonHost,
		GdaemonPort: int32(n.GdaemonPort), //nolint:gosec
		Metadata:    metadataToStringMap(n.Metadata),
	}

	if n.CreatedAt != nil {
		node.CreatedAt = new(n.CreatedAt.Unix())
	}
	if n.UpdatedAt != nil {
		node.UpdatedAt = new(n.UpdatedAt.Unix())
	}

	return node
}

type NodesHostLibrary struct {
	impl *NodesServiceImpl
}

func (l *NodesHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return nodes.Instantiate(ctx, r, l.impl)
}

// NodesHostLibraryFactory builds a gameap-nodes library bound to each plugin's
// ID: the writes are gated on the plugin's own manage_nodes grant and
// enrollment tickets are owned by the plugin that issued them.
type NodesHostLibraryFactory struct {
	nodeRepo    repositories.NodeRepository
	nodes       NodeWriter
	tickets     NodeEnrollment
	connect     ConnectTargetResolver
	checker     PluginPermissionChecker
	auditLogger audit.Logger
}

func NewNodesHostLibraryFactory(
	nodeRepo repositories.NodeRepository,
	nodeWriter NodeWriter,
	tickets NodeEnrollment,
	connect ConnectTargetResolver,
	checker PluginPermissionChecker,
	auditLogger audit.Logger,
) *NodesHostLibraryFactory {
	return &NodesHostLibraryFactory{
		nodeRepo:    nodeRepo,
		nodes:       nodeWriter,
		tickets:     tickets,
		connect:     connect,
		checker:     checker,
		auditLogger: auditLogger,
	}
}

func (f *NodesHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &NodesHostLibrary{
		impl: NewNodesService(
			pluginID,
			f.nodeRepo,
			f.nodes,
			f.tickets,
			f.connect,
			f.checker,
			f.auditLogger,
		),
	}
}
