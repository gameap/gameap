package updatepermissions

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// maxBody bounds the JSON body: a permission list is a few hundred bytes.
const maxBody = 64 << 10

var errPluginNotInstalled = errors.New("plugin is not installed")

type input struct {
	AllowedPermissions []string `json:"allowed_permissions"`
}

// Handler replaces the permissions an installed plugin holds. The host
// libraries read grants on every call, so the change is effective at once
// on every instance; event subscriptions are refreshed locally because
// listen_events is applied when they are rebuilt.
type Handler struct {
	pluginRepo repositories.PluginRepository
	manager    PluginManager
	resolver   DBIDResolver
	refresher  plugininstall.SubscriptionRefresher
	announcer  SubscriptionsAnnouncer
	responder  base.Responder
	audit      audit.Logger
}

func NewHandler(
	pluginRepo repositories.PluginRepository,
	manager PluginManager,
	resolver DBIDResolver,
	refresher plugininstall.SubscriptionRefresher,
	announcer SubscriptionsAnnouncer,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		pluginRepo: pluginRepo,
		manager:    manager,
		resolver:   resolver,
		refresher:  refresher,
		announcer:  announcer,
		responder:  responder,
		audit:      auditLogger,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	if id == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("plugin ID is required"))

		return
	}

	var in input
	if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, maxBody)).Decode(&in); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	permissions, unknown := parsePermissions(in.AllowedPermissions)
	if len(unknown) > 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.Errorf("unknown permissions: %s", strings.Join(unknown, ", ")),
			http.StatusUnprocessableEntity,
		))

		return
	}

	dbID := h.resolveDBID(id)

	record, err := h.findPlugin(ctx, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	previous := record.AllowedPermissions
	record.AllowedPermissions = permissions

	if err := h.pluginRepo.Save(ctx, record); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin permissions"))

		return
	}

	granted, revoked := diffPermissions(previous, permissions)

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginPermissionsUpdate, audit.CategoryPluginOp,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "update",
		slog.Any("granted", granted), slog.Any("revoked", revoked))

	if slices.Contains(granted, string(domain.PluginPermissionListenEvents)) ||
		slices.Contains(revoked, string(domain.PluginPermissionListenEvents)) {
		plugininstall.RefreshSubscriptions(ctx, h.refresher)
		h.announceSubscriptionChange(ctx, uint64(dbID))
	}

	h.responder.Write(ctx, rw, newPermissionsResponse(record, h.loadedPlugin(dbID)))
}

// announceSubscriptionChange lets the other instances rebuild their
// subscription maps. Delivery is advisory: they also re-check the grant
// before every event they deliver, so a failure delays the map cleanup
// rather than leaking events.
func (h *Handler) announceSubscriptionChange(ctx context.Context, pluginID uint64) {
	if h.announcer == nil {
		return
	}

	if err := h.announcer.PublishRefresh(context.WithoutCancel(ctx), pluginID); err != nil {
		slog.ErrorContext(ctx, "failed to announce plugin subscription change",
			slog.Uint64("plugin_id", pluginID),
			slog.String("error", err.Error()))
	}
}

// parsePermissions validates the names; duplicates collapse, unknown names
// are reported so the operator sees a typo instead of a silent drop.
func parsePermissions(names []string) ([]domain.PluginPermission, []string) {
	permissions := make([]domain.PluginPermission, 0, len(names))
	unknown := make([]string, 0)

	for _, name := range names {
		permission, ok := domain.ParsePluginPermission(name)
		if !ok {
			unknown = append(unknown, name)

			continue
		}

		if !slices.Contains(permissions, permission) {
			permissions = append(permissions, permission)
		}
	}

	if len(permissions) == 0 {
		// nil keeps the column NULL rather than an empty list, as install does.
		permissions = nil
	}

	return permissions, unknown
}

func diffPermissions(previous, current []domain.PluginPermission) (granted, revoked []string) {
	granted, revoked = []string{}, []string{}

	for _, permission := range current {
		if !slices.Contains(previous, permission) {
			granted = append(granted, string(permission))
		}
	}

	for _, permission := range previous {
		if !slices.Contains(current, permission) {
			revoked = append(revoked, string(permission))
		}
	}

	return granted, revoked
}

func (h *Handler) findPlugin(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, error) {
	plugins, err := h.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return nil, api.WrapHTTPError(errPluginNotInstalled, http.StatusNotFound)
	}

	return &plugins[0], nil
}

// resolveDBID prefers the loader's mapping for plugins loaded on this
// instance and falls back to decoding the ID, which covers plugins that are
// installed but not loaded.
func (h *Handler) resolveDBID(id string) domain.Uint64ID {
	if h.resolver != nil {
		if dbID, found := h.resolver.GetDBPluginID(id); found {
			return dbID
		}
	}

	return pkgplugin.ParsePluginID(id)
}

// loadedPlugin finds the module on this instance, if any, to report what it
// can exercise.
func (h *Handler) loadedPlugin(dbID domain.Uint64ID) *pkgplugin.LoadedPlugin {
	if h.manager == nil {
		return nil
	}

	managerID := pkgplugin.CompactPluginID(dbID)
	if h.resolver != nil {
		if id, found := h.resolver.GetPluginManagerID(dbID); found {
			managerID = id
		}
	}

	loaded, ok := h.manager.GetPlugin(managerID)
	if !ok {
		return nil
	}

	return loaded
}
