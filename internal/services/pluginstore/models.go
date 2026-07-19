package pluginstore

import "time"

type Category struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	SortOrder   int    `json:"sort_order"`
}

type Label struct {
	ID    int    `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Author struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type Screenshot struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`
	SortOrder int    `json:"sort_order"`
}

type Plugin struct {
	ID                   string    `json:"id"`
	URL                  string    `json:"url"`
	Name                 string    `json:"name"`
	Summary              string    `json:"summary"`
	IconURL              string    `json:"icon_url"`
	Category             Category  `json:"category"`
	Labels               []Label   `json:"labels"`
	DownloadCount        int       `json:"download_count"`
	RatingAvg            float64   `json:"rating_avg"`
	RatingCount          int       `json:"rating_count"`
	LatestVersion        string    `json:"latest_version"`
	RequiresSubscription bool      `json:"requires_subscription"`
	SubscriptionURL      string    `json:"subscription_url"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type PluginDetails struct {
	ID                   string    `json:"id"`
	URL                  string    `json:"url"`
	Name                 string    `json:"name"`
	Summary              string    `json:"summary"`
	Description          string    `json:"description"`
	IconURL              string    `json:"icon_url"`
	License              string    `json:"license"`
	RepositoryURL        string    `json:"repository_url"`
	MinGameAPVersion     string    `json:"min_gameap_version"`
	MinPluginAPIVersion  string    `json:"min_plugin_api_version"`
	Author               Author    `json:"author"`
	Category             Category  `json:"category"`
	Labels               []Label   `json:"labels"`
	DownloadCount        int       `json:"download_count"`
	RatingAvg            float64   `json:"rating_avg"`
	RatingCount          int       `json:"rating_count"`
	LatestVersion        string    `json:"latest_version"`
	RequiresSubscription bool      `json:"requires_subscription"`
	SubscriptionURL      string    `json:"subscription_url"`
	PublishedAt          time.Time `json:"published_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type PluginIcon struct {
	Data        []byte `json:"data"`
	ContentType string `json:"content_type"`
}

type PluginVersion struct {
	ID                  int          `json:"id"`
	Version             string       `json:"version"`
	Changelog           string       `json:"changelog"`
	FileSize            int64        `json:"file_size"`
	FileHash            string       `json:"file_hash"`
	SignURL             string       `json:"sign_url"`
	MinGameAPVersion    string       `json:"min_gameap_version"`
	MinPluginAPIVersion string       `json:"min_plugin_api_version"`
	IsStable            bool         `json:"is_stable"`
	Screenshots         []Screenshot `json:"screenshots"`
	DownloadCount       int          `json:"download_count"`
	CreatedAt           time.Time    `json:"created_at"`
}

type PaginatedResponse[T any] struct {
	CurrentPage int `json:"current_page"`
	Data        []T `json:"data"`
	From        int `json:"from"`
	LastPage    int `json:"last_page"`
	PerPage     int `json:"per_page"`
	Total       int `json:"total"`
}

type LicenseSubscription struct {
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type LicenseValidation struct {
	Valid         bool                  `json:"valid"`
	ExpiresAt     *time.Time            `json:"expires_at,omitempty"`
	Subscriptions []LicenseSubscription `json:"subscriptions"`
	Message       string                `json:"message,omitempty"`
}
