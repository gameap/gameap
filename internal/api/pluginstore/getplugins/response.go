package getplugins

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/pluginstore"
)

type labelResponse struct {
	ID    int    `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type categoryResponse struct {
	ID   int    `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type pluginResponse struct {
	ID                    string           `json:"id"`
	URL                   string           `json:"url"`
	Name                  string           `json:"name"`
	Summary               string           `json:"summary"`
	IconURL               string           `json:"icon_url"`
	Category              categoryResponse `json:"category"`
	Labels                []labelResponse  `json:"labels"`
	DownloadCount         int              `json:"download_count"`
	RatingAvg             float64          `json:"rating_avg"`
	RatingCount           int              `json:"rating_count"`
	LatestVersion         string           `json:"latest_version"`
	RequiresSubscription  bool             `json:"requires_subscription"`
	SubscriptionURL       string           `json:"subscription_url,omitempty"`
	CreatedAt             time.Time        `json:"created_at"`
	UpdatedAt             time.Time        `json:"updated_at"`
	Installed             bool             `json:"installed"`
	InstalledVersion      *string          `json:"installed_version,omitempty"`
	HasSubscription       *bool            `json:"has_subscription,omitempty"`
	SubscriptionExpiresAt *time.Time       `json:"subscription_expires_at,omitempty"`
}

type paginatedPluginsResponse struct {
	CurrentPage int              `json:"current_page"`
	Data        []pluginResponse `json:"data"`
	From        int              `json:"from"`
	LastPage    int              `json:"last_page"`
	PerPage     int              `json:"per_page"`
	Total       int              `json:"total"`
}

func newPluginsResponse(
	storePlugins *pluginstore.PaginatedResponse[pluginstore.Plugin],
	installedMap map[domain.Uint64ID]string,
	licenseValidation *pluginstore.LicenseValidation,
) *paginatedPluginsResponse {
	subscriptionMap := buildSubscriptionMap(licenseValidation)
	data := make([]pluginResponse, 0, len(storePlugins.Data))

	for _, p := range storePlugins.Data {
		labels := make([]labelResponse, 0, len(p.Labels))
		for _, l := range p.Labels {
			labels = append(labels, labelResponse{
				ID:    l.ID,
				Slug:  l.Slug,
				Name:  l.Name,
				Color: l.Color,
			})
		}

		hasSubscription, subscriptionExpiresAt := getSubscriptionInfo(p.ID, p.RequiresSubscription, subscriptionMap)

		data = append(data, pluginResponse{
			ID:      p.ID,
			URL:     p.URL,
			Name:    p.Name,
			Summary: p.Summary,
			IconURL: panelIconURL(p.ID, p.IconURL),
			Category: categoryResponse{
				ID:   p.Category.ID,
				Slug: p.Category.Slug,
				Name: p.Category.Name,
			},
			Labels:                labels,
			DownloadCount:         p.DownloadCount,
			RatingAvg:             p.RatingAvg,
			RatingCount:           p.RatingCount,
			LatestVersion:         p.LatestVersion,
			RequiresSubscription:  p.RequiresSubscription,
			SubscriptionURL:       p.SubscriptionURL,
			CreatedAt:             p.CreatedAt,
			UpdatedAt:             p.UpdatedAt,
			Installed:             isInstalled(p.ID, installedMap),
			InstalledVersion:      getInstalledVersion(p.ID, installedMap),
			HasSubscription:       hasSubscription,
			SubscriptionExpiresAt: subscriptionExpiresAt,
		})
	}

	return &paginatedPluginsResponse{
		CurrentPage: storePlugins.CurrentPage,
		Data:        data,
		From:        storePlugins.From,
		LastPage:    storePlugins.LastPage,
		PerPage:     storePlugins.PerPage,
		Total:       storePlugins.Total,
	}
}

// panelIconURL replaces the external store icon URL with the panel's proxy
// endpoint so the browser loads icons same-origin (external hosts are blocked
// by the CSP img-src directive).
func panelIconURL(pluginID, storeIconURL string) string {
	if storeIconURL == "" {
		return ""
	}

	return "/api/plugin-store/plugins/" + pluginID + "/icon"
}

func buildSubscriptionMap(licenseValidation *pluginstore.LicenseValidation) map[string]time.Time {
	if licenseValidation == nil || !licenseValidation.Valid {
		return nil
	}

	subscriptionMap := make(map[string]time.Time)
	for _, sub := range licenseValidation.Subscriptions {
		subscriptionMap[sub.PluginID] = sub.ExpiresAt
	}

	return subscriptionMap
}

func getSubscriptionInfo(
	pluginID string,
	requiresSubscription bool,
	subscriptionMap map[string]time.Time,
) (*bool, *time.Time) {
	if !requiresSubscription {
		return nil, nil
	}

	if subscriptionMap == nil {
		return nil, nil
	}

	if expiresAt, ok := subscriptionMap[pluginID]; ok {
		hasSubscription := true

		return &hasSubscription, &expiresAt
	}

	hasSubscription := false

	return &hasSubscription, nil
}
