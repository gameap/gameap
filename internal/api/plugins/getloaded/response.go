package getloaded

import (
	"strings"

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
}

type listResponse struct {
	Data []*loadedPluginResponse `json:"data"`
}

func newLoadedPluginResponse(
	loaded *pkgplugin.LoadedPlugin,
	source string,
) *loadedPluginResponse {
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

	return resp
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
