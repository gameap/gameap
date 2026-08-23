package dryrun

import (
	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/configschema"
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

type dryRunResponse struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Version             string   `json:"version"`
	Description         string   `json:"description,omitempty"`
	Author              string   `json:"author,omitempty"`
	APIVersion          string   `json:"api_version,omitempty"`
	RequiredPermissions []string `json:"required_permissions,omitempty"`
	// UsedPermissions is derived from the module's host imports and event
	// subscriptions; UndeclaredPermissions are the used ones the manifest
	// does not declare — installing grants only the declared ones, so those
	// calls would be refused.
	UsedPermissions       []string                `json:"used_permissions,omitempty"`
	UndeclaredPermissions []string                `json:"undeclared_permissions,omitempty"`
	HTTPRoutes            []httpRouteResponse     `json:"http_routes,omitempty"`
	ServerAbilities       []serverAbilityResponse `json:"server_abilities,omitempty"`
	SubscribedEvents      []string                `json:"subscribed_events,omitempty"`
	HasFrontendBundle     bool                    `json:"has_frontend_bundle"`
	FrontendBundleSize    int                     `json:"frontend_bundle_size,omitempty"`
	HasFrontendStyles     bool                    `json:"has_frontend_styles"`
	// ConfigSchema summarises the manifest's configuration schema; absent
	// for a plugin with free-form configuration.
	ConfigSchema *configschema.SummaryInfo `json:"config_schema,omitempty"`
	IsValid      bool                      `json:"is_valid"`
	Errors       []string                  `json:"errors"`
}

func newDryRunResponse(loaded *pkgplugin.LoadedPlugin, subscribedEvents []proto.EventType) *dryRunResponse {
	resp := &dryRunResponse{
		ID:                 pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(loaded.Info.Id)),
		Name:               loaded.Info.Name,
		Version:            loaded.Info.Version,
		Description:        loaded.Info.Description,
		Author:             loaded.Info.Author,
		APIVersion:         loaded.Info.ApiVersion,
		HasFrontendBundle:  len(loaded.FrontendBundle) > 0,
		FrontendBundleSize: len(loaded.FrontendBundle),
		HasFrontendStyles:  len(loaded.FrontendStyles) > 0,
		IsValid:            true,
		Errors:             []string{},
	}

	if loaded.Info.RequiredPermissions != nil {
		resp.RequiredPermissions = loaded.Info.RequiredPermissions
	}

	if loaded.Info.ConfigSchema != "" {
		summary := configschema.Summary(loaded.Info.ConfigSchema)
		resp.ConfigSchema = &summary

		if !summary.Valid {
			// The plugin still loads with free-form configuration; the
			// author is told before installing.
			resp.Errors = append(resp.Errors, "config_schema is invalid and will be ignored: "+summary.Error)
		}
	}

	used := internalplugin.UsedPermissions(loaded.HostImports, subscribedEvents)
	if len(used) > 0 {
		resp.UsedPermissions = internalplugin.PermissionNames(used)
		resp.UndeclaredPermissions = internalplugin.PermissionNames(internalplugin.MissingPermissions(
			used, domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)))
	}

	if len(loaded.HTTPRoutes) > 0 {
		resp.HTTPRoutes = make([]httpRouteResponse, 0, len(loaded.HTTPRoutes))
		for _, route := range loaded.HTTPRoutes {
			resp.HTTPRoutes = append(resp.HTTPRoutes, httpRouteResponse{
				Path:    route.Path,
				Methods: route.Methods,
			})
		}
	}

	if len(loaded.ServerAbilities) > 0 {
		resp.ServerAbilities = make([]serverAbilityResponse, 0, len(loaded.ServerAbilities))
		for _, ability := range loaded.ServerAbilities {
			resp.ServerAbilities = append(resp.ServerAbilities, serverAbilityResponse{
				Name:  ability.Name,
				Title: ability.Title,
			})
		}
	}

	if len(subscribedEvents) > 0 {
		resp.SubscribedEvents = make([]string, 0, len(subscribedEvents))
		for _, event := range subscribedEvents {
			resp.SubscribedEvents = append(resp.SubscribedEvents, proto.EventType_name[int32(event)])
		}
	}

	return resp
}
