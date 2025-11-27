package domain

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ServerInstalledStatus int

const (
	ServerInstalledStatusNotInstalled       ServerInstalledStatus = 0
	ServerInstalledStatusInstalled          ServerInstalledStatus = 1
	ServerInstalledStatusInstallationInProg ServerInstalledStatus = 2
)

func (s ServerInstalledStatus) Valid() bool {
	switch s {
	case ServerInstalledStatusNotInstalled,
		ServerInstalledStatusInstalled,
		ServerInstalledStatusInstallationInProg:
		return true
	default:
		return false
	}
}

type Server struct {
	ID               uint                  `db:"id"`
	UUID             uuid.UUID             `db:"uuid"`
	UUIDShort        string                `db:"uuid_short"`
	Enabled          bool                  `db:"enabled"`
	Installed        ServerInstalledStatus `db:"installed"`
	Blocked          bool                  `db:"blocked"`
	Name             string                `db:"name"`
	GameID           string                `db:"game_id"`
	DSID             uint                  `db:"ds_id"`
	GameModID        uint                  `db:"game_mod_id"`
	Expires          *time.Time            `db:"expires"`
	ServerIP         string                `db:"server_ip"`
	ServerPort       int                   `db:"server_port"`
	QueryPort        *int                  `db:"query_port"`
	RconPort         *int                  `db:"rcon_port"`
	Rcon             *string               `db:"rcon"`
	Dir              string                `db:"dir"`
	SuUser           *string               `db:"su_user"`
	CPULimit         *int                  `db:"cpu_limit"`
	RAMLimit         *int                  `db:"ram_limit"`
	NetLimit         *int                  `db:"net_limit"`
	StartCommand     *string               `db:"start_command"`
	StopCommand      *string               `db:"stop_command"`
	ForceStopCommand *string               `db:"force_stop_command"`
	RestartCommand   *string               `db:"restart_command"`
	ProcessActive    bool                  `db:"process_active"`
	LastProcessCheck *time.Time            `db:"last_process_check"`
	Vars             *string               `db:"vars"`
	CreatedAt        *time.Time            `db:"created_at"`
	UpdatedAt        *time.Time            `db:"updated_at"`
	DeletedAt        *time.Time            `db:"deleted_at"`
}

const timeExpireProcessCheck = 2 * time.Minute

func (s *Server) IsOnline() bool {
	if s.LastProcessCheck == nil || s.LastProcessCheck.IsZero() {
		return false
	}

	return s.ProcessActive && s.LastProcessCheck.UTC().After(time.Now().UTC().Add(-timeExpireProcessCheck))
}

// ReplaceServerShortcodes replaces shortcode placeholders in a command string with server-specific values.
// It first replaces any extra data provided, then replaces standard server shortcodes.
// Shortcodes are replaced in the format {key} with their corresponding values.
func (s *Server) ReplaceServerShortcodes(node *Node, command string, extra map[string]string) string {
	// Replace extra data first
	for key, value := range extra {
		command = strings.ReplaceAll(command, "{"+key+"}", value)
	}

	// Build the replacement map for server shortcodes
	replaceMap := map[string]string{
		"node_work_path":  node.WorkPath,
		"node_tools_path": node.WorkPath + "/tools",
		"host":            s.ServerIP,
		"port":            strconv.Itoa(s.ServerPort),
		"query_port":      "", // default empty, may be set below
		"rcon_port":       "", // default empty, may be set below
		"user":            "", // default empty, may be set below
		"id":              strconv.FormatUint(uint64(s.ID), 10),
		"uuid":            s.UUID.String(),
		"uuid_short":      s.UUIDShort,
		"game":            s.GameID,
		"dir":             s.Dir,
	}

	// Add optional fields if they are set
	if s.QueryPort != nil {
		replaceMap["query_port"] = strconv.Itoa(*s.QueryPort)
	}

	if s.RconPort != nil {
		replaceMap["rcon_port"] = strconv.Itoa(*s.RconPort)
	}

	if s.SuUser != nil {
		replaceMap["user"] = *s.SuUser
	}

	// Replace all server shortcodes
	for key, value := range replaceMap {
		command = strings.ReplaceAll(command, "{"+key+"}", value)
	}

	return command
}
