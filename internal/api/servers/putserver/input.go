package putserver

import (
	"fmt"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/validation"
)

const (
	maxNameLength = 128
	minPort       = 1
	maxPort       = 65535
)

var (
	ErrNameIsRequired    = api.NewValidationError("name is required")
	ErrGameIDIsRequired  = api.NewValidationError("game_id is required")
	ErrDSIDIsRequired    = api.NewValidationError("ds_id is required")
	ErrGameModIDRequired = api.NewValidationError("game_mod_id is required")
	ErrServerIPRequired  = api.NewValidationError("server_ip is required")
	ErrNameTooLong       = api.NewValidationError(
		fmt.Sprintf("name must not exceed %d characters", maxNameLength),
	)
	ErrInvalidServerIP   = api.NewValidationError("server_ip is not a valid IP address or hostname")
	ErrInvalidInstalled  = api.NewValidationError("installed has an invalid value, must be 0, 1 or 2")
	ErrInvalidServerPort = api.NewValidationError(
		fmt.Sprintf("server_port must be between %d and %d", minPort, maxPort),
	)
	ErrInvalidQueryPort = api.NewValidationError(
		fmt.Sprintf("query_port must be between %d and %d", minPort, maxPort),
	)
	ErrInvalidRconPort = api.NewValidationError(
		fmt.Sprintf("rcon_port must be between %d and %d", minPort, maxPort),
	)
)

type updateServerInput struct {
	Enabled      *flexible.Bool `json:"enabled,omitempty"`
	Installed    *flexible.Int  `json:"installed,omitempty"`
	Blocked      *flexible.Bool `json:"blocked,omitempty"`
	Name         string         `json:"name"`
	GameID       string         `json:"game_id"`
	DSID         flexible.Int   `json:"ds_id"`
	GameModID    flexible.Int   `json:"game_mod_id"`
	ServerIP     string         `json:"server_ip"`
	ServerPort   flexible.Int   `json:"server_port"`
	QueryPort    *flexible.Int  `json:"query_port,omitempty"`
	RconPort     *flexible.Int  `json:"rcon_port,omitempty"`
	Rcon         *string        `json:"rcon,omitempty"`
	StartCommand *string        `json:"start_command,omitempty"`
	Dir          *string        `json:"dir,omitempty"`
	SuUser       *string        `json:"su_user,omitempty"`
}

func (in *updateServerInput) Validate() error {
	if in.Name == "" {
		return ErrNameIsRequired
	}

	if len(in.Name) > maxNameLength {
		return ErrNameTooLong
	}

	if in.GameID == "" {
		return ErrGameIDIsRequired
	}

	if in.DSID.Int() <= 0 {
		return ErrDSIDIsRequired
	}

	if in.GameModID.Int() <= 0 {
		return ErrGameModIDRequired
	}

	if in.ServerIP == "" {
		return ErrServerIPRequired
	}

	if !validation.IsValidIPOrHostname(in.ServerIP) {
		return ErrInvalidServerIP
	}

	if !domain.ServerInstalledStatus(in.Installed.Int()).Valid() {
		return ErrInvalidInstalled
	}

	if in.ServerPort.Int() < minPort || in.ServerPort.Int() > maxPort {
		return ErrInvalidServerPort
	}

	if in.QueryPort != nil && (in.QueryPort.Int() < minPort || in.QueryPort.Int() > maxPort) {
		return ErrInvalidQueryPort
	}

	if in.RconPort != nil && (in.RconPort.Int() < minPort || in.RconPort.Int() > maxPort) {
		return ErrInvalidRconPort
	}

	return nil
}

func (in *updateServerInput) Apply(server *domain.Server) error {
	server.Name = in.Name

	if in.Enabled != nil {
		server.Enabled = in.Enabled.Bool()
	}

	if in.Installed != nil {
		server.Installed = domain.ServerInstalledStatus(in.Installed.Int())
	}

	if in.Blocked != nil {
		server.Blocked = in.Blocked.Bool()
	}

	server.GameID = in.GameID
	server.DSID = uint(in.DSID.Int())           //nolint:gosec // We check it in Validate
	server.GameModID = uint(in.GameModID.Int()) //nolint:gosec // We check it in Validate
	server.ServerIP = in.ServerIP
	server.ServerPort = in.ServerPort.Int()

	if in.QueryPort != nil {
		qp := in.QueryPort.Int()
		server.QueryPort = &qp
	}

	if in.RconPort != nil {
		rp := in.RconPort.Int()
		server.RconPort = &rp
	}

	if in.Rcon != nil {
		server.Rcon = in.Rcon
	}

	if in.StartCommand != nil {
		server.StartCommand = in.StartCommand
	}

	if in.Dir != nil {
		server.Dir = *in.Dir
	}

	if in.SuUser != nil {
		server.SuUser = in.SuUser
	}

	return nil
}
