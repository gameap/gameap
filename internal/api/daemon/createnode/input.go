package createnode

import (
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/samber/lo"
)

const (
	minPort     = 1
	maxPort     = 65535
	defaultPort = 31717
)

var (
	ErrGdaemonServerCertFileRequired = api.NewValidationError("gdaemon_server_cert file is required")
	ErrInvalidGdaemonServerCertFile  = api.NewValidationError("invalid gdaemon_server_cert file")
	ErrInvalidGdaemonPort            = api.NewValidationError("gdaemon_port must be between 1 and 65535")
)

type nodeInput struct {
	Name                string
	IP                  []string
	GdaemonHost         string
	GdaemonPort         string
	Location            string
	Provider            string
	OS                  string
	WorkPath            string
	SteamcmdPath        string
	PreferInstallMethod string
	ScriptInstall       string
	ScriptReinstall     string
	ScriptUpdate        string
	ScriptStart         string
	ScriptPause         string
	ScriptUnpause       string
	ScriptStop          string
	ScriptKill          string
	ScriptRestart       string
	ScriptStatus        string
	ScriptStats         string
	ScriptGetConsole    string
	ScriptSendCommand   string
	ScriptDelete        string
	GdaemonServerCert   []*multipart.FileHeader
}

func newNodeInputFromRequest(r *http.Request) *nodeInput {
	return &nodeInput{
		Name:                strings.TrimSpace(r.FormValue("name")),
		IP:                  r.MultipartForm.Value["ip[]"],
		GdaemonHost:         strings.TrimSpace(r.FormValue("gdaemon_host")),
		GdaemonPort:         strings.TrimSpace(r.FormValue("gdaemon_port")),
		Location:            strings.TrimSpace(r.FormValue("location")),
		Provider:            strings.TrimSpace(r.FormValue("provider")),
		OS:                  strings.TrimSpace(r.FormValue("os")),
		WorkPath:            strings.TrimSpace(r.FormValue("work_path")),
		SteamcmdPath:        strings.TrimSpace(r.FormValue("steamcmd_path")),
		PreferInstallMethod: strings.TrimSpace(r.FormValue("prefer_install_method")),
		ScriptInstall:       strings.TrimSpace(r.FormValue("script_install")),
		ScriptReinstall:     strings.TrimSpace(r.FormValue("script_reinstall")),
		ScriptUpdate:        strings.TrimSpace(r.FormValue("script_update")),
		ScriptStart:         strings.TrimSpace(r.FormValue("script_start")),
		ScriptPause:         strings.TrimSpace(r.FormValue("script_pause")),
		ScriptUnpause:       strings.TrimSpace(r.FormValue("script_unpause")),
		ScriptStop:          strings.TrimSpace(r.FormValue("script_stop")),
		ScriptKill:          strings.TrimSpace(r.FormValue("script_kill")),
		ScriptRestart:       strings.TrimSpace(r.FormValue("script_restart")),
		ScriptStatus:        strings.TrimSpace(r.FormValue("script_status")),
		ScriptStats:         strings.TrimSpace(r.FormValue("script_stats")),
		ScriptGetConsole:    strings.TrimSpace(r.FormValue("script_get_console")),
		ScriptSendCommand:   strings.TrimSpace(r.FormValue("script_send_command")),
		ScriptDelete:        strings.TrimSpace(r.FormValue("script_delete")),
		GdaemonServerCert:   r.MultipartForm.File["gdaemon_server_cert"],
	}
}

func (in *nodeInput) Validate() error {
	if len(in.GdaemonServerCert) == 0 {
		return ErrGdaemonServerCertFileRequired
	}

	if in.GdaemonServerCert[0].Size == 0 {
		return ErrInvalidGdaemonServerCertFile
	}

	if in.GdaemonPort != "" {
		port, err := strconv.Atoi(in.GdaemonPort)
		if err != nil || port < minPort || port > maxPort {
			return ErrInvalidGdaemonPort
		}
	}

	return nil
}

func (in *nodeInput) ToDomain(apiKey string, serverCert string) *domain.Node {
	name := in.Name
	if name == "" {
		var err error
		name, err = pkgstrings.CryptoRandomString(5)
		if err != nil {
			slog.Warn("Failed to generate random string", "error", err)
			name = "Unnamed Node"
		}
	}

	gdaemonHost := in.GdaemonHost
	if gdaemonHost == "" && len(in.IP) > 0 {
		gdaemonHost = in.IP[0]
	}

	gdaemonPort := defaultPort
	if in.GdaemonPort != "" {
		if port, err := strconv.Atoi(in.GdaemonPort); err == nil {
			gdaemonPort = port
		}
	}

	var steamcmdPath *string
	if in.SteamcmdPath != "" {
		steamcmdPath = &in.SteamcmdPath
	}

	return &domain.Node{
		Enabled:             true,
		Name:                name,
		OS:                  domain.ParseNodeOS(in.OS),
		Location:            lo.CoalesceOrEmpty(in.Location, "Unknown"),
		Provider:            lo.ToPtr(lo.CoalesceOrEmpty(in.Provider, "Unknown")),
		IPs:                 in.IP,
		WorkPath:            lo.CoalesceOrEmpty(in.WorkPath, "/srv/gameap"),
		SteamcmdPath:        steamcmdPath,
		GdaemonHost:         gdaemonHost,
		GdaemonPort:         gdaemonPort,
		GdaemonAPIKey:       apiKey,
		GdaemonServerCert:   serverCert,
		ClientCertificateID: 0,
		PreferInstallMethod: lo.CoalesceOrEmpty(
			domain.NodePreferInstallMethod(in.PreferInstallMethod),
			domain.NodePreferInstallMethodAuto,
		),
		ScriptInstall:     nilIfEmptyString(in.ScriptInstall),
		ScriptReinstall:   nilIfEmptyString(in.ScriptReinstall),
		ScriptUpdate:      nilIfEmptyString(in.ScriptUpdate),
		ScriptStart:       nilIfEmptyString(in.ScriptStart),
		ScriptPause:       nilIfEmptyString(in.ScriptPause),
		ScriptUnpause:     nilIfEmptyString(in.ScriptUnpause),
		ScriptStop:        nilIfEmptyString(in.ScriptStop),
		ScriptKill:        nilIfEmptyString(in.ScriptKill),
		ScriptRestart:     nilIfEmptyString(in.ScriptRestart),
		ScriptStatus:      nilIfEmptyString(in.ScriptStatus),
		ScriptStats:       nilIfEmptyString(in.ScriptStats),
		ScriptGetConsole:  nilIfEmptyString(in.ScriptGetConsole),
		ScriptSendCommand: nilIfEmptyString(in.ScriptSendCommand),
		ScriptDelete:      nilIfEmptyString(in.ScriptDelete),
	}
}

func nilIfEmptyString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
