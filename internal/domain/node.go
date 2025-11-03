package domain

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"
)

type Node struct {
	ID                  uint                    `db:"id"`
	Enabled             bool                    `db:"enabled"`
	Name                string                  `db:"name"`
	OS                  NodeOS                  `db:"os"`
	Location            string                  `db:"location"`
	Provider            *string                 `db:"provider"`
	IPs                 IPList                  `db:"ip"`
	RAM                 *string                 `db:"ram"`
	CPU                 *string                 `db:"cpu"`
	WorkPath            string                  `db:"work_path"`
	SteamcmdPath        *string                 `db:"steamcmd_path"`
	GdaemonHost         string                  `db:"gdaemon_host"`
	GdaemonPort         int                     `db:"gdaemon_port"`
	GdaemonAPIKey       string                  `db:"gdaemon_api_key"`
	GdaemonAPIToken     *string                 `db:"gdaemon_api_token"`
	GdaemonLogin        *string                 `db:"gdaemon_login"`
	GdaemonPassword     *string                 `db:"gdaemon_password"`
	GdaemonServerCert   string                  `db:"gdaemon_server_cert"`
	ClientCertificateID uint                    `db:"client_certificate_id"`
	PreferInstallMethod NodePreferInstallMethod `db:"prefer_install_method"`
	ScriptInstall       *string                 `db:"script_install"`
	ScriptReinstall     *string                 `db:"script_reinstall"`
	ScriptUpdate        *string                 `db:"script_update"`
	ScriptStart         *string                 `db:"script_start"`
	ScriptPause         *string                 `db:"script_pause"`
	ScriptUnpause       *string                 `db:"script_unpause"`
	ScriptStop          *string                 `db:"script_stop"`
	ScriptKill          *string                 `db:"script_kill"`
	ScriptRestart       *string                 `db:"script_restart"`
	ScriptStatus        *string                 `db:"script_status"`
	ScriptStats         *string                 `db:"script_stats"`
	ScriptGetConsole    *string                 `db:"script_get_console"`
	ScriptSendCommand   *string                 `db:"script_send_command"`
	ScriptDelete        *string                 `db:"script_delete"`
	CreatedAt           *time.Time              `db:"created_at"`
	UpdatedAt           *time.Time              `db:"updated_at"`
	DeletedAt           *time.Time              `db:"deleted_at"`
}

type NodeOS string

const (
	NodeOSLinux   NodeOS = "linux"
	NodeOSWindows NodeOS = "windows"
	NodeOSMacOS   NodeOS = "macos"
	NodeOSOther   NodeOS = "other"
)

func ParseNodeOS(os string) NodeOS {
	os = strings.ToLower(strings.TrimSpace(os))
	if len(os) == 0 {
		return NodeOSOther
	}

	switch os[0] {
	case 'l':
		return NodeOSLinux
	case 'w':
		return NodeOSWindows
	}

	var os3 string
	if len(os) > 3 {
		os3 = os[:3]
	} else {
		os3 = os
	}

	switch os3 {
	case "ubu", "deb", "cen", "fed", "alm", "roc", "arc", "sus":
		return NodeOSLinux
	case "macos", "mac", "osx", "dar":
		return NodeOSMacOS
	default:
		return NodeOSOther
	}
}

func (os NodeOS) Value() (driver.Value, error) {
	switch os {
	case NodeOSLinux,
		NodeOSWindows,
		NodeOSMacOS,
		NodeOSOther:
		return string(os), nil
	default:
		return string(NodeOSOther), nil
	}
}

func (os *NodeOS) Scan(value any) error {
	if value == nil {
		*os = NodeOSOther

		return nil
	}

	if bytes, ok := value.([]byte); ok {
		*os = ParseNodeOS(string(bytes))

		return nil
	}

	if str, ok := value.(string); ok {
		*os = ParseNodeOS(str)

		return nil
	}

	*os = NodeOSOther

	return nil
}

type NodePreferInstallMethod string

const (
	NodePreferInstallMethodAuto     NodePreferInstallMethod = "auto"
	NodePreferInstallMethodCopy     NodePreferInstallMethod = "copy"
	NodePreferInstallMethodDownload NodePreferInstallMethod = "download"
	NodePreferInstallMethodScript   NodePreferInstallMethod = "script"
	NodePreferInstallMethodSteam    NodePreferInstallMethod = "steam"
	NodePreferInstallMethodNode     NodePreferInstallMethod = "none"
)

func (m NodePreferInstallMethod) Value() (driver.Value, error) {
	switch m {
	case NodePreferInstallMethodAuto,
		NodePreferInstallMethodCopy,
		NodePreferInstallMethodDownload,
		NodePreferInstallMethodScript,
		NodePreferInstallMethodSteam,
		NodePreferInstallMethodNode:
		return string(m), nil
	default:
		return string(NodePreferInstallMethodAuto), nil
	}
}

// IPList is a custom type that handles JSON array of IPs in the database.
type IPList []string

// Scan implements sql.Scanner interface.
func (list *IPList) Scan(value any) error {
	if value == nil {
		*list = []string{}

		return nil
	}

	// Handle []byte from database (TEXT/JSON column)
	if b, ok := value.([]byte); ok {
		if len(b) == 0 {
			*list = []string{}

			return nil
		}
		// Try to unmarshal as JSON array
		var result []string
		if err := json.Unmarshal(b, &result); err != nil {
			// If JSON unmarshal fails, treat as empty
			*list = []string{}

			return nil //nolint:nilerr // Treat unmarshal error as empty
		}
		*list = result

		return nil
	}

	// Handle string directly
	if str, ok := value.(string); ok {
		if str == "" {
			*list = []string{}

			return nil
		}
		var result []string
		if err := json.Unmarshal([]byte(str), &result); err != nil {
			*list = []string{}

			return nil //nolint:nilerr // Treat unmarshal error as empty
		}
		*list = result

		return nil
	}

	*list = []string{}

	return nil
}

// Value implements driver.Valuer interface.
func (list IPList) Value() (driver.Value, error) {
	if len(list) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]", err
	}

	return string(b), nil
}
