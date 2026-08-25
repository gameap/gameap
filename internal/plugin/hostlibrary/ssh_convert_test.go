package hostlibrary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteFileCommand covers OWASP API8:2023 Security Misconfiguration: the
// remote write must never leave the target readable wider than the requested
// mode, must reject modes that are not permission bits (the decimal-vs-octal
// literal trap), and must keep hostile path names one shell argument.
func TestWriteFileCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		mode      uint32
		want      string
		wantError string
	}{
		{
			name: "mode_zero_moves_the_temp_file_into_place",
			path: "/etc/app/config.yaml",
			mode: 0,
			want: "{ cat > '/etc/app/config.yaml.gameap-tmp'" +
				" && mv -f -- '/etc/app/config.yaml.gameap-tmp' '/etc/app/config.yaml'; }" +
				" || { rc=$?; rm -f -- '/etc/app/config.yaml.gameap-tmp'; exit $rc; }",
		},
		{
			name: "mode_sets_permissions_on_the_temp_file_before_the_move",
			path: "/root/.ssh/authorized_keys",
			mode: 0o600,
			want: "umask 077; { cat > '/root/.ssh/authorized_keys.gameap-tmp'" +
				" && chmod 600 -- '/root/.ssh/authorized_keys.gameap-tmp'" +
				" && mv -f -- '/root/.ssh/authorized_keys.gameap-tmp' '/root/.ssh/authorized_keys'; }" +
				" || { rc=$?; rm -f -- '/root/.ssh/authorized_keys.gameap-tmp'; exit $rc; }",
		},
		{
			name: "path_with_a_single_quote_stays_one_argument",
			path: "/tmp/it's.sh",
			mode: 0o700,
			want: `umask 077; { cat > '/tmp/it'\''s.sh.gameap-tmp'` +
				` && chmod 700 -- '/tmp/it'\''s.sh.gameap-tmp'` +
				` && mv -f -- '/tmp/it'\''s.sh.gameap-tmp' '/tmp/it'\''s.sh'; }` +
				` || { rc=$?; rm -f -- '/tmp/it'\''s.sh.gameap-tmp'; exit $rc; }`,
		},
		{
			name: "mode_0o777_is_accepted",
			path: "/opt/run.sh",
			mode: 0o777,
			want: "umask 077; { cat > '/opt/run.sh.gameap-tmp'" +
				" && chmod 777 -- '/opt/run.sh.gameap-tmp'" +
				" && mv -f -- '/opt/run.sh.gameap-tmp' '/opt/run.sh'; }" +
				" || { rc=$?; rm -f -- '/opt/run.sh.gameap-tmp'; exit $rc; }",
		},
		{
			name:      "blank_path_is_refused",
			path:      "   ",
			mode:      0o644,
			wantError: "path is required",
		},
		{
			name:      "mode_above_the_permission_bits_is_refused",
			path:      "/opt/run.sh",
			mode:      0o7777,
			wantError: "octal permission bits",
		},
		{
			name:      "decimal_mode_is_refused_with_guidance",
			path:      "/opt/run.sh",
			mode:      755,
			wantError: "0o644",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			command, err := writeFileCommand(tt.path, tt.mode)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, command)
		})
	}
}

func TestReadFileCommand(t *testing.T) {
	t.Parallel()

	t.Run("uses_the_end_of_options_marker", func(t *testing.T) {
		t.Parallel()

		command, err := readFileCommand("/etc/passwd")

		require.NoError(t, err)
		assert.Equal(t, "cat -- '/etc/passwd'", command)
	})

	t.Run("blank_path_is_refused", func(t *testing.T) {
		t.Parallel()

		_, err := readFileCommand("  ")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "path is required")
	})
}
