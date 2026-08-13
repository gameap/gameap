package getloaded

import (
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/pluginsync"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
)

type httpRouteResponse struct {
	Path    string   `json:"path"`
	Methods []string `json:"methods"`
}

type serverAbilityResponse struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

type loadedPluginResponse struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Version           string                  `json:"version"`
	Description       string                  `json:"description,omitempty"`
	Source            string                  `json:"source,omitempty"`
	SourceType        string                  `json:"source_type"`
	Enabled           bool                    `json:"enabled"`
	HTTPRoutes        []httpRouteResponse     `json:"http_routes,omitempty"`
	ServerAbilities   []serverAbilityResponse `json:"server_abilities,omitempty"`
	HasFrontendBundle bool                    `json:"has_frontend_bundle"`

	// The fields below describe this instance only. Plugin state is
	// reconciled per instance, so a plugin can be healthy on one replica and
	// retrying on another.
	DBStatus      string     `json:"db_status,omitempty"`
	SyncState     string     `json:"sync_state,omitempty"`
	SyncError     string     `json:"sync_error,omitempty"`
	SyncFailures  int        `json:"sync_failures,omitempty"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	LoadedAt      *time.Time `json:"loaded_at,omitempty"`
}

type listResponse struct {
	Data []*loadedPluginResponse `json:"data"`
}

func newLoadedPluginResponse(
	loaded *pkgplugin.LoadedPlugin,
	dbPlugin *domain.Plugin,
	syncStatus pluginsync.Status,
) *loadedPluginResponse {
	var source string
	if dbPlugin != nil && dbPlugin.Source != nil {
		source = *dbPlugin.Source
	}

	resp := &loadedPluginResponse{
		ID:                pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(loaded.Info.Id)),
		Name:              loaded.Info.Name,
		Version:           loaded.Info.Version,
		Description:       loaded.Info.Description,
		Source:            source,
		SourceType:        determineSourceType(source),
		Enabled:           loaded.IsEnabled(),
		HasFrontendBundle: len(loaded.FrontendBundle) > 0,
	}

	if len(loaded.HTTPRoutes) > 0 {
		resp.HTTPRoutes = convertHTTPRoutes(loaded.HTTPRoutes)
	}

	if len(loaded.ServerAbilities) > 0 {
		resp.ServerAbilities = convertServerAbilities(loaded.ServerAbilities)
	}

	applySyncStatus(resp, dbPlugin, syncStatus)

	return resp
}

// newUnloadedPluginResponse describes a plugin the database wants active that
// this instance does not have running.
func newUnloadedPluginResponse(dbPlugin *domain.Plugin, syncStatus pluginsync.Status) *loadedPluginResponse {
	var source string
	if dbPlugin.Source != nil {
		source = *dbPlugin.Source
	}

	resp := &loadedPluginResponse{
		ID:          pkgplugin.CompactPluginID(dbPlugin.ID),
		Name:        dbPlugin.Name,
		Version:     dbPlugin.Version,
		Description: dbPlugin.Description,
		Source:      source,
		SourceType:  determineSourceType(source),
		Enabled:     false,
	}

	applySyncStatus(resp, dbPlugin, syncStatus)

	// A plugin with no recorded attempt is not in trouble yet, it is simply
	// waiting for the next pass.
	if resp.SyncState == "" {
		resp.SyncState = string(pluginsync.SyncStatePending)
	}

	return resp
}

func applySyncStatus(resp *loadedPluginResponse, dbPlugin *domain.Plugin, syncStatus pluginsync.Status) {
	if dbPlugin != nil {
		resp.DBStatus = string(dbPlugin.Status)
	} else {
		// Loaded with nothing in the database to explain it: the reconciler
		// leaves such a module alone rather than guessing it was removed.
		resp.SyncState = string(pluginsync.SyncStateOrphan)

		return
	}

	if syncStatus.State == "" {
		return
	}

	resp.SyncState = string(syncStatus.State)
	resp.SyncError = syncStatus.LastError
	resp.SyncFailures = syncStatus.Failures

	if !syncStatus.NextAttempt.IsZero() {
		resp.NextAttemptAt = &syncStatus.NextAttempt
	}

	if !syncStatus.LoadedAt.IsZero() {
		resp.LoadedAt = &syncStatus.LoadedAt
	}
}

func convertHTTPRoutes(routes []*proto.HTTPRoute) []httpRouteResponse {
	result := make([]httpRouteResponse, 0, len(routes))
	for _, route := range routes {
		result = append(result, httpRouteResponse{
			Path:    route.Path,
			Methods: route.Methods,
		})
	}

	return result
}

func convertServerAbilities(abilities []*proto.ServerAbility) []serverAbilityResponse {
	result := make([]serverAbilityResponse, 0, len(abilities))
	for _, ability := range abilities {
		result = append(result, serverAbilityResponse{
			Name:  ability.Name,
			Title: ability.Title,
		})
	}

	return result
}

func determineSourceType(source string) string {
	if strings.HasPrefix(source, "file://") {
		return "file"
	}

	return "store"
}
