package putserver

import (
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/flexible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrFI(v int) *flexible.Int {
	fi := flexible.Int(v)

	return &fi
}

func validInput() updateServerInput {
	return updateServerInput{
		Name:       "Valid Server",
		GameID:     "cstrike",
		DSID:       flexible.Int(1),
		GameModID:  flexible.Int(1),
		ServerIP:   "192.168.1.100",
		ServerPort: flexible.Int(27015),
	}
}

func TestUpdateServerInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     updateServerInput
		wantError string
	}{
		{
			name:  "valid_minimal_required_fields_only",
			input: validInput(),
		},
		{
			name: "valid_full_with_optional_dir",
			input: updateServerInput{
				Name:       "Full Server",
				GameID:     "cstrike",
				DSID:       flexible.Int(1),
				GameModID:  flexible.Int(1),
				ServerIP:   "192.168.1.100",
				ServerPort: flexible.Int(27015),
				QueryPort:  ptrFI(27016),
				RconPort:   ptrFI(27017),
				Dir:        new("servers/cs"),
				Installed:  ptrFI(1),
				CPULimit:   ptrFI(2000),
				RAMLimit:   ptrFI(4294967296),
			},
		},
		{
			name: "name_required_empty",
			input: func() updateServerInput {
				in := validInput()
				in.Name = ""

				return in
			}(),
			wantError: ErrNameIsRequired.Error(),
		},
		{
			name: "name_too_long_129_chars",
			input: func() updateServerInput {
				in := validInput()
				in.Name = strings.Repeat("a", 129)

				return in
			}(),
			wantError: ErrNameTooLong.Error(),
		},
		{
			name: "name_at_max_length_128_chars_is_valid",
			input: func() updateServerInput {
				in := validInput()
				in.Name = strings.Repeat("a", 128)

				return in
			}(),
		},
		{
			name: "game_id_required_empty",
			input: func() updateServerInput {
				in := validInput()
				in.GameID = ""

				return in
			}(),
			wantError: ErrGameIDIsRequired.Error(),
		},
		{
			name: "ds_id_required_zero",
			input: func() updateServerInput {
				in := validInput()
				in.DSID = flexible.Int(0)

				return in
			}(),
			wantError: ErrDSIDIsRequired.Error(),
		},
		{
			name: "ds_id_required_negative",
			input: func() updateServerInput {
				in := validInput()
				in.DSID = flexible.Int(-1)

				return in
			}(),
			wantError: ErrDSIDIsRequired.Error(),
		},
		{
			name: "game_mod_id_required_zero",
			input: func() updateServerInput {
				in := validInput()
				in.GameModID = flexible.Int(0)

				return in
			}(),
			wantError: ErrGameModIDRequired.Error(),
		},
		{
			name: "game_mod_id_required_negative",
			input: func() updateServerInput {
				in := validInput()
				in.GameModID = flexible.Int(-1)

				return in
			}(),
			wantError: ErrGameModIDRequired.Error(),
		},
		{
			name: "server_ip_required_empty",
			input: func() updateServerInput {
				in := validInput()
				in.ServerIP = ""

				return in
			}(),
			wantError: ErrServerIPRequired.Error(),
		},
		{
			name: "server_ip_invalid_format",
			input: func() updateServerInput {
				in := validInput()
				in.ServerIP = "not_valid!!!"

				return in
			}(),
			wantError: ErrInvalidServerIP.Error(),
		},
		{
			name: "server_ip_octet_out_of_range",
			input: func() updateServerInput {
				in := validInput()
				in.ServerIP = "192.168.1.999"

				return in
			}(),
			wantError: ErrInvalidServerIP.Error(),
		},
		{
			name: "installed_zero_is_valid",
			input: func() updateServerInput {
				in := validInput()
				in.Installed = ptrFI(0)

				return in
			}(),
		},
		{
			name: "installed_status_invalid",
			input: func() updateServerInput {
				in := validInput()
				in.Installed = ptrFI(99)

				return in
			}(),
			wantError: ErrInvalidInstalled.Error(),
		},
		{
			name: "installed_status_negative_invalid",
			input: func() updateServerInput {
				in := validInput()
				in.Installed = ptrFI(-1)

				return in
			}(),
			wantError: ErrInvalidInstalled.Error(),
		},
		{
			name: "server_port_below_min",
			input: func() updateServerInput {
				in := validInput()
				in.ServerPort = flexible.Int(0)

				return in
			}(),
			wantError: ErrInvalidServerPort.Error(),
		},
		{
			name: "server_port_negative",
			input: func() updateServerInput {
				in := validInput()
				in.ServerPort = flexible.Int(-1)

				return in
			}(),
			wantError: ErrInvalidServerPort.Error(),
		},
		{
			name: "server_port_above_max",
			input: func() updateServerInput {
				in := validInput()
				in.ServerPort = flexible.Int(65536)

				return in
			}(),
			wantError: ErrInvalidServerPort.Error(),
		},
		{
			name: "query_port_below_min",
			input: func() updateServerInput {
				in := validInput()
				in.QueryPort = ptrFI(0)

				return in
			}(),
			wantError: ErrInvalidQueryPort.Error(),
		},
		{
			name: "query_port_above_max",
			input: func() updateServerInput {
				in := validInput()
				in.QueryPort = ptrFI(65536)

				return in
			}(),
			wantError: ErrInvalidQueryPort.Error(),
		},
		{
			name: "rcon_port_below_min",
			input: func() updateServerInput {
				in := validInput()
				in.RconPort = ptrFI(0)

				return in
			}(),
			wantError: ErrInvalidRconPort.Error(),
		},
		{
			name: "rcon_port_above_max",
			input: func() updateServerInput {
				in := validInput()
				in.RconPort = ptrFI(65536)

				return in
			}(),
			wantError: ErrInvalidRconPort.Error(),
		},
		{
			name: "cpu_limit_negative",
			input: func() updateServerInput {
				in := validInput()
				in.CPULimit = ptrFI(-1)

				return in
			}(),
			wantError: ErrInvalidCPULimit.Error(),
		},
		{
			name: "cpu_limit_zero_is_valid",
			input: func() updateServerInput {
				in := validInput()
				in.CPULimit = ptrFI(0)

				return in
			}(),
		},
		{
			name: "ram_limit_negative",
			input: func() updateServerInput {
				in := validInput()
				in.RAMLimit = ptrFI(-1)

				return in
			}(),
			wantError: ErrInvalidRAMLimit.Error(),
		},
		{
			name: "ram_limit_zero_is_valid",
			input: func() updateServerInput {
				in := validInput()
				in.RAMLimit = ptrFI(0)

				return in
			}(),
		},
		{
			name: "dir_nil_is_allowed",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = nil

				return in
			}(),
		},
		{
			name: "dir_empty_string_is_allowed",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new("")

				return in
			}(),
		},
		{
			name: "dir_valid_relative_path",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new("servers/cs")

				return in
			}(),
		},
		{
			name: "dir_leading_slash_rejected",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new("/srv/gameap/servers/cs")

				return in
			}(),
			wantError: ErrInvalidDir.Error(),
		},
		{
			name: "dir_leading_backslash_rejected",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new(`\gameap\servers\cs`)

				return in
			}(),
			wantError: ErrInvalidDir.Error(),
		},
		{
			name: "dir_windows_drive_letter_rejected",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new(`C:\gameap\servers\cs`)

				return in
			}(),
			wantError: ErrInvalidDir.Error(),
		},
		{
			name: "dir_unc_share_path_rejected",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new(`\\server\share\dir`)

				return in
			}(),
			wantError: ErrInvalidDir.Error(),
		},
		{
			name: "dir_dot_dot_segment_rejected",
			input: func() updateServerInput {
				in := validInput()
				in.Dir = new("../etc/passwd")

				return in
			}(),
			wantError: ErrInvalidDir.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError == "" {
				require.NoError(t, err, "expected validation to succeed")
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			}
		})
	}
}
