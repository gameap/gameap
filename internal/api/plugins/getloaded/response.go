package getloaded

import (
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	SourceType  string `json:"source_type"`
	// Enabled and Loaded describe this panel instance; Status, Error and
	// ErrorAt come from the shared plugin record.
	Enabled           bool                    `json:"enabled"`
	Loaded            bool                    `json:"loaded"`
	Status            string                  `json:"status"`
	Error             *string                 `json:"error"`
	ErrorAt           *time.Time              `json:"error_at"`
	MemoryBytes       *uint64                 `json:"memory_bytes"`
	HTTPRoutes        []httpRouteResponse     `json:"http_routes,omitempty"`
	ServerAbilities   []serverAbilityResponse `json:"server_abilities,omitempty"`
	HasFrontendBundle bool                    `json:"has_frontend_bundle"`
}

type listResponse struct {
	Data []*loadedPluginResponse `json:"data"`
}

// newLoadedPluginResponse describes a plugin loaded on this instance; record
// is its database row when one exists.
func newLoadedPluginResponse(
	loaded *pkgplugin.LoadedPlugin,
	record *domain.Plugin,
) *loadedPluginResponse {
	var source string
	if record != nil && record.Source != nil {
		source = *record.Source
	}

	resp := &loadedPluginResponse{
		ID:                pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(loaded.Info.Id)),
		Name:              loaded.Info.Name,
		Version:           loaded.Info.Version,
		Description:       loaded.Info.Description,
		Source:            source,
		SourceType:        determineSourceType(source),
		Enabled:           loaded.IsEnabled(),
		Loaded:            true,
		HasFrontendBundle: len(loaded.FrontendBundle) > 0,
	}

	resp.Status, resp.Error, resp.ErrorAt = runtimeState(loaded, record)

	if size, ok := loaded.MemorySize(); ok {
		resp.MemoryBytes = new(size)
	}

	if len(loaded.HTTPRoutes) > 0 {
		resp.HTTPRoutes = convertHTTPRoutes(loaded.HTTPRoutes)
	}

	if len(loaded.ServerAbilities) > 0 {
		resp.ServerAbilities = convertServerAbilities(loaded.ServerAbilities)
	}

	return resp
}

// runtimeState reconciles the in-memory state with the record: a plugin
// that is loaded and enabled is active whatever the row says (the row may
// lag behind on another instance); one disabled at runtime reports its own
// reason, falling back to the recorded one.
func runtimeState(loaded *pkgplugin.LoadedPlugin, record *domain.Plugin) (string, *string, *time.Time) {
	if loaded.IsEnabled() {
		return string(domain.PluginStatusActive), nil, nil
	}

	var (
		status  = domain.PluginStatusError
		reason  *string
		errorAt *time.Time
	)

	if record != nil {
		if record.Status == domain.PluginStatusDisabled || record.Status == domain.PluginStatusUpdating {
			status = record.Status
		}

		reason = record.LastError
		errorAt = record.LastErrorAt
	}

	if text, ok := loaded.DisabledReason(); ok {
		reason = new(text)
	}

	return string(status), reason, errorAt
}

// newInstalledPluginResponse describes an installed plugin that is not
// loaded on this instance (failed to load, disabled, updating).
func newInstalledPluginResponse(record *domain.Plugin) *loadedPluginResponse {
	var source string
	if record.Source != nil {
		source = *record.Source
	}

	return &loadedPluginResponse{
		ID:          pkgplugin.CompactPluginID(record.ID),
		Name:        record.Name,
		Version:     record.Version,
		Description: record.Description,
		Source:      source,
		SourceType:  determineSourceType(source),
		Enabled:     false,
		Loaded:      false,
		Status:      string(record.Status),
		Error:       record.LastError,
		ErrorAt:     record.LastErrorAt,
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
