package getversion

import (
	"github.com/gameap/gameap/internal/services/releases"
	"github.com/gameap/gameap/pkg/version"
)

type panelVersion struct {
	Current         string `json:"current"`
	BuildDate       string `json:"build_date"`
	IsRelease       bool   `json:"is_release"`
	LatestStable    string `json:"latest_stable,omitempty"`
	LatestStableURL string `json:"latest_stable_url,omitempty"`
	LatestBeta      string `json:"latest_beta,omitempty"`
	LatestBetaURL   string `json:"latest_beta_url,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

type daemonVersion struct {
	LatestStable    string `json:"latest_stable,omitempty"`
	LatestStableURL string `json:"latest_stable_url,omitempty"`
	LatestBeta      string `json:"latest_beta,omitempty"`
	LatestBetaURL   string `json:"latest_beta_url,omitempty"`
}

type versionResponse struct {
	Panel              panelVersion  `json:"panel"`
	Daemon             daemonVersion `json:"daemon"`
	UpdateCheckEnabled bool          `json:"update_check_enabled"`
}

func newVersionResponse(
	current, buildDate string,
	panelInfo, daemonInfo releases.Info,
	updateCheckEnabled bool,
) versionResponse {
	return versionResponse{
		Panel: panelVersion{
			Current:         current,
			BuildDate:       buildDate,
			IsRelease:       version.IsRelease(current),
			LatestStable:    panelInfo.LatestStable,
			LatestStableURL: panelInfo.LatestStableURL,
			LatestBeta:      panelInfo.LatestBeta,
			LatestBetaURL:   panelInfo.LatestBetaURL,
			UpdateAvailable: version.IsNewer(current, panelInfo.LatestStable),
		},
		Daemon: daemonVersion{
			LatestStable:    daemonInfo.LatestStable,
			LatestStableURL: daemonInfo.LatestStableURL,
			LatestBeta:      daemonInfo.LatestBeta,
			LatestBetaURL:   daemonInfo.LatestBetaURL,
		},
		UpdateCheckEnabled: updateCheckEnabled,
	}
}
