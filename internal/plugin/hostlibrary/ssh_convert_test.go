package hostlibrary

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderWriteFileCommand covers OWASP API8:2023 Security Misconfiguration:
// the remote write must never leave the target readable wider than the
// requested mode, and must keep hostile path names one shell argument.
func TestRenderWriteFileCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		temp   string
		mode   uint32
		want   string
	}{
		{
			name:   "mode_zero_moves_the_temp_file_into_place",
			target: "/etc/app/config.yaml",
			temp:   "/etc/app/config.yaml.gameap-tmp.d2ff2m0c1s2t3u4v5w6x",
			mode:   0,
			want: "{ cat > '/etc/app/config.yaml.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" && mv -f -- '/etc/app/config.yaml.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" '/etc/app/config.yaml'; }" +
				" || { rc=$?; rm -f -- '/etc/app/config.yaml.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'; exit $rc; }",
		},
		{
			name:   "mode_sets_permissions_on_the_temp_file_before_the_move",
			target: "/root/.ssh/authorized_keys",
			temp:   "/root/.ssh/authorized_keys.gameap-tmp.d2ff2m0c1s2t3u4v5w6x",
			mode:   0o600,
			want: "umask 077; { cat > '/root/.ssh/authorized_keys.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" && chmod 600 -- '/root/.ssh/authorized_keys.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" && mv -f -- '/root/.ssh/authorized_keys.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" '/root/.ssh/authorized_keys'; }" +
				" || { rc=$?; rm -f -- '/root/.ssh/authorized_keys.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'; exit $rc; }",
		},
		{
			name:   "path_with_a_single_quote_stays_one_argument",
			target: "/tmp/it's.sh",
			temp:   "/tmp/it's.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x",
			mode:   0o700,
			want: `umask 077; { cat > '/tmp/it'\''s.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'` +
				` && chmod 700 -- '/tmp/it'\''s.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'` +
				` && mv -f -- '/tmp/it'\''s.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x' '/tmp/it'\''s.sh'; }` +
				` || { rc=$?; rm -f -- '/tmp/it'\''s.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'; exit $rc; }`,
		},
		{
			name:   "mode_0o777_renders_as_octal",
			target: "/opt/run.sh",
			temp:   "/opt/run.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x",
			mode:   0o777,
			want: "umask 077; { cat > '/opt/run.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" && chmod 777 -- '/opt/run.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'" +
				" && mv -f -- '/opt/run.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x' '/opt/run.sh'; }" +
				" || { rc=$?; rm -f -- '/opt/run.sh.gameap-tmp.d2ff2m0c1s2t3u4v5w6x'; exit $rc; }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, renderWriteFileCommand(tt.target, tt.temp, tt.mode))
		})
	}
}

// TestWriteFileCommand covers OWASP API8:2023 Security Misconfiguration: the
// remote write must reject modes that are not permission bits (the
// decimal-vs-octal literal trap), and must give every transfer its own
// temporary sibling so concurrent writes to one target neither interleave
// their content nor delete each other's temp file.
func TestWriteFileCommand(t *testing.T) {
	t.Parallel()

	t.Run("every_write_gets_its_own_temp_sibling", func(t *testing.T) {
		t.Parallel()

		const target = "/etc/app/config.yaml"

		first, err := writeFileCommand(target, 0o600)
		require.NoError(t, err)

		second, err := writeFileCommand(target, 0o600)
		require.NoError(t, err)

		firstTemp := tempPathFromCommand(t, first)
		secondTemp := tempPathFromCommand(t, second)

		assert.NotEqual(t, firstTemp, secondTemp)
		assert.True(t, strings.HasPrefix(firstTemp, target+writeFileTempSuffix),
			"temp file must be a sibling of the target, got %q", firstTemp)

		assert.Equal(t, renderWriteFileCommand(target, firstTemp, 0o600), first,
			"the generated temp path must be used by cat, chmod, mv and the cleanup")
		assert.Equal(t, renderWriteFileCommand(target, secondTemp, 0o600), second,
			"the generated temp path must be used by cat, chmod, mv and the cleanup")
	})

	t.Run("path_is_trimmed_before_the_temp_sibling_is_derived", func(t *testing.T) {
		t.Parallel()

		command, err := writeFileCommand("  /opt/run.sh  ", 0)

		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(tempPathFromCommand(t, command),
			"/opt/run.sh"+writeFileTempSuffix), "unexpected command %q", command)
	})

	tests := []struct {
		name      string
		path      string
		mode      uint32
		wantError string
	}{
		{
			name: "mode_0o777_is_accepted",
			path: "/opt/run.sh",
			mode: 0o777,
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
				assert.Empty(t, command)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, command)
		})
	}
}

// tempPathFromCommand reads back the temporary sibling a write command streams
// into.
func tempPathFromCommand(t *testing.T, command string) string {
	t.Helper()

	match := regexp.MustCompile(`cat > '(.+?)'`).FindStringSubmatch(command)
	require.Len(t, match, 2, "no temp path in %q", command)

	return match[1]
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
