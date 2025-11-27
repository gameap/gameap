package postserver

import (
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/validation"
	"github.com/google/uuid"
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
	ErrNameTooLong       = api.NewValidationError("name must not exceed 128 characters")
	ErrInvalidServerIP   = api.NewValidationError("server_ip is not a valid IP address or hostname")
	ErrInvalidServerPort = api.NewValidationError("server_port must be between 1 and 65535")
	ErrInvalidQueryPort  = api.NewValidationError("query_port must be between 1 and 65535")
	ErrInvalidRconPort   = api.NewValidationError("rcon_port must be between 1 and 65535")
)

type serverInput struct {
	Install      *flexible.Bool `json:"install,omitempty"`
	Name         string         `json:"name"`
	DSID         flexible.Int   `json:"ds_id"`
	GameID       string         `json:"game_id"`
	GameModID    flexible.Int   `json:"game_mod_id"`
	ServerIP     string         `json:"server_ip"`
	ServerPort   flexible.Int   `json:"server_port"`
	QueryPort    *flexible.Int  `json:"query_port,omitempty"`
	RconPort     *flexible.Int  `json:"rcon_port,omitempty"`
	Rcon         *string        `json:"rcon,omitempty"`
	Dir          *string        `json:"dir,omitempty"`
	StartCommand *string        `json:"start_command,omitempty"`
	SuUser       *string        `json:"su_user,omitempty"`
}

func (s *serverInput) Validate() error {
	if s.Name == "" {
		return ErrNameIsRequired
	}

	if len(s.Name) > maxNameLength {
		return ErrNameTooLong
	}

	if s.GameID == "" {
		return ErrGameIDIsRequired
	}

	if s.DSID.Int() <= 0 {
		return ErrDSIDIsRequired
	}

	if s.GameModID.Int() <= 0 {
		return ErrGameModIDRequired
	}

	if s.ServerIP == "" {
		return ErrServerIPRequired
	}

	if !validation.IsValidIPOrHostname(s.ServerIP) {
		return ErrInvalidServerIP
	}

	if s.ServerPort.Int() < minPort || s.ServerPort.Int() > maxPort {
		return ErrInvalidServerPort
	}

	if s.QueryPort != nil && (s.QueryPort.Int() < minPort || s.QueryPort.Int() > maxPort) {
		return ErrInvalidQueryPort
	}

	if s.RconPort != nil && (s.RconPort.Int() < minPort || s.RconPort.Int() > maxPort) {
		return ErrInvalidRconPort
	}

	return nil
}

func (s *serverInput) ToDomain() *domain.Server {
	u, err := uuid.NewV7()
	if err != nil {
		slog.Error(
			"Unable to generate server UUID",
			slog.String("error", err.Error()),
		)

		u = uuid.New()
	}

	var queryPort *int
	if s.QueryPort != nil {
		qp := s.QueryPort.Int()
		queryPort = &qp
	}

	var rconPort *int
	if s.RconPort != nil {
		rp := s.RconPort.Int()
		rconPort = &rp
	}

	server := &domain.Server{
		UUID:         u,
		UUIDShort:    u.String()[0:8],
		Enabled:      true,
		Installed:    domain.ServerInstalledStatusNotInstalled,
		Blocked:      false,
		Name:         s.Name,
		GameID:       s.GameID,
		DSID:         uint(s.DSID.Int()),      //nolint:gosec // We check it in Validate
		GameModID:    uint(s.GameModID.Int()), //nolint:gosec // We check it in Validate
		ServerIP:     s.ServerIP,
		ServerPort:   s.ServerPort.Int(),
		QueryPort:    queryPort,
		RconPort:     rconPort,
		Rcon:         s.Rcon,
		Dir:          getDir(s.Dir),
		StartCommand: s.StartCommand,
		SuUser:       s.SuUser,
	}

	return server
}

func getDir(dir *string) string {
	if dir == nil || *dir == "" {
		return ""
	}

	return *dir
}
