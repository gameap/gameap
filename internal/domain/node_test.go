package domain

import (
	"database/sql/driver"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseNodeOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		expectedOS NodeOS
	}{
		{
			name:       "valid_linux_os",
			input:      "linux",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "valid_linux_os_symbols",
			input:      "  LiNuX  ",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "valid_windows_os",
			input:      "windows",
			expectedOS: NodeOSWindows,
		},
		{
			name:       "windows_short_three_chars",
			input:      "win",
			expectedOS: NodeOSWindows,
		},
		{
			name:       "macos_short_three_chars",
			input:      "osx",
			expectedOS: NodeOSMacOS,
		},
		{
			name:       "valid_macos_os",
			input:      "macos",
			expectedOS: NodeOSMacOS,
		},
		{
			name:       "invalid_os",
			input:      "invalid",
			expectedOS: NodeOSOther,
		},
		{
			name:       "ubuntu_distribution",
			input:      "ubuntu",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "debian_distribution",
			input:      "debian",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "centos_distribution",
			input:      "centos",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "fedora_distribution",
			input:      "fedora",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "almalinux_distribution",
			input:      "almalinux",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "rockylinux_distribution",
			input:      "rockylinux",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "arch_distribution",
			input:      "archlinux",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "suse_distribution",
			input:      "suse",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "darwin_os",
			input:      "darwin",
			expectedOS: NodeOSMacOS,
		},
		{
			name:       "mac_short_three_chars",
			input:      "mac",
			expectedOS: NodeOSMacOS,
		},
		{
			name:       "empty_string",
			input:      "",
			expectedOS: NodeOSOther,
		},
		{
			name:       "whitespace_only",
			input:      "   ",
			expectedOS: NodeOSOther,
		},
		{
			name:       "short_string_two_chars",
			input:      "li",
			expectedOS: NodeOSLinux,
		},
		{
			name:       "windows_mixed_case",
			input:      "WiNdOwS",
			expectedOS: NodeOSWindows,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := ParseNodeOS(test.input)
			assert.Equal(t, test.expectedOS, result)
		})
	}
}

func TestNodeOS_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    NodeOS
		expected driver.Value
	}{
		{
			name:     "linux_os",
			input:    NodeOSLinux,
			expected: "linux",
		},
		{
			name:     "windows_os",
			input:    NodeOSWindows,
			expected: "windows",
		},
		{
			name:     "macos_os",
			input:    NodeOSMacOS,
			expected: "macos",
		},
		{
			name:     "other_os",
			input:    NodeOSOther,
			expected: "other",
		},
		{
			name:     "invalid_os_returns_other",
			input:    NodeOS("invalid"),
			expected: "other",
		},
		{
			name:     "empty_os_returns_other",
			input:    NodeOS(""),
			expected: "other",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.input.Value()
			require.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestNodeOS_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected NodeOS
	}{
		{
			name:     "nil_value_overrides_to_other",
			input:    nil,
			expected: NodeOSOther,
		},
		{
			name:     "linux_bytes",
			input:    []byte("linux"),
			expected: NodeOSLinux,
		},
		{
			name:     "windows_bytes",
			input:    []byte("windows"),
			expected: NodeOSWindows,
		},
		{
			name:     "macos_bytes",
			input:    []byte("macos"),
			expected: NodeOSMacOS,
		},
		{
			name:     "other_bytes",
			input:    []byte("other"),
			expected: NodeOSOther,
		},
		{
			name:     "linux_string",
			input:    "linux",
			expected: NodeOSLinux,
		},
		{
			name:     "windows_string",
			input:    "windows",
			expected: NodeOSWindows,
		},
		{
			name:     "macos_string",
			input:    "macos",
			expected: NodeOSMacOS,
		},
		{
			name:     "ubuntu_string",
			input:    "ubuntu",
			expected: NodeOSLinux,
		},
		{
			name:     "debian_bytes",
			input:    []byte("debian"),
			expected: NodeOSLinux,
		},
		{
			name:     "darwin_string",
			input:    "darwin",
			expected: NodeOSMacOS,
		},
		{
			name:     "invalid_string",
			input:    "invalid",
			expected: NodeOSOther,
		},
		{
			name:     "empty_string",
			input:    "",
			expected: NodeOSOther,
		},
		{
			name:     "empty_bytes",
			input:    []byte(""),
			expected: NodeOSOther,
		},
		{
			name:     "unsupported_type_overrides_to_other",
			input:    123,
			expected: NodeOSOther,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE — start from a non-zero receiver distinct from any expected value
			// so the test can detect missing assignments in any branch.
			result := NodeOS("__sentinel__")

			// ACT
			err := result.Scan(test.input)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, test.expected, result, "scan should overwrite the receiver to expected value")
		})
	}
}

func TestNodePreferInstallMethod_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    NodePreferInstallMethod
		expected driver.Value
	}{
		{
			name:     "auto_method",
			input:    NodePreferInstallMethodAuto,
			expected: "auto",
		},
		{
			name:     "copy_method",
			input:    NodePreferInstallMethodCopy,
			expected: "copy",
		},
		{
			name:     "download_method",
			input:    NodePreferInstallMethodDownload,
			expected: "download",
		},
		{
			name:     "script_method",
			input:    NodePreferInstallMethodScript,
			expected: "script",
		},
		{
			name:     "steam_method",
			input:    NodePreferInstallMethodSteam,
			expected: "steam",
		},
		{
			name:     "none_method",
			input:    NodePreferInstallMethodNode,
			expected: "none",
		},
		{
			name:     "invalid_method_returns_auto",
			input:    NodePreferInstallMethod("invalid"),
			expected: "auto",
		},
		{
			name:     "empty_method_returns_auto",
			input:    NodePreferInstallMethod(""),
			expected: "auto",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.input.Value()
			require.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestIPList_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected IPList
	}{
		{
			name:     "nil_value",
			input:    nil,
			expected: IPList{},
		},
		{
			name:     "empty_bytes",
			input:    []byte(""),
			expected: IPList{},
		},
		{
			name:     "empty_array_bytes",
			input:    []byte("[]"),
			expected: IPList{},
		},
		{
			name:     "single_ip_bytes",
			input:    []byte(`["192.168.1.1"]`),
			expected: IPList{"192.168.1.1"},
		},
		{
			name:     "multiple_ips_bytes",
			input:    []byte(`["192.168.1.1","10.0.0.1","172.16.0.1"]`),
			expected: IPList{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
		},
		{
			name:     "empty_string",
			input:    "",
			expected: IPList{},
		},
		{
			name:     "empty_array_string",
			input:    "[]",
			expected: IPList{},
		},
		{
			name:     "single_ip_string",
			input:    `["192.168.1.1"]`,
			expected: IPList{"192.168.1.1"},
		},
		{
			name:     "multiple_ips_string",
			input:    `["192.168.1.1","10.0.0.1"]`,
			expected: IPList{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:     "invalid_json_bytes_become_empty",
			input:    []byte(`{invalid json`),
			expected: IPList{},
		},
		{
			name:     "invalid_json_string_becomes_empty",
			input:    `{invalid json`,
			expected: IPList{},
		},
		{
			name:     "non_array_json_bytes",
			input:    []byte(`{"key":"value"}`),
			expected: IPList{},
		},
		{
			name:     "unsupported_type_becomes_empty",
			input:    123,
			expected: IPList{},
		},
		{
			name:     "array_with_empty_strings",
			input:    []byte(`["","192.168.1.1",""]`),
			expected: IPList{"", "192.168.1.1", ""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE — start from a prefilled receiver to detect missing assignments.
			result := IPList{"preexisting"}

			// ACT
			err := result.Scan(test.input)

			// ASSERT
			require.NoError(t, err)
			require.NotNil(t, result, "scan should never leave receiver nil")
			require.Len(t, result, len(test.expected), "length mismatch")
			assert.Equal(t, test.expected, result, "value mismatch")
		})
	}
}

func TestIPList_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    IPList
		expected string
	}{
		{
			name:     "nil_list",
			input:    nil,
			expected: "[]",
		},
		{
			name:     "empty_list",
			input:    []string{},
			expected: "[]",
		},
		{
			name:     "single_ip",
			input:    []string{"192.168.1.1"},
			expected: `["192.168.1.1"]`,
		},
		{
			name:     "multiple_ips",
			input:    []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
			expected: `["192.168.1.1","10.0.0.1","172.16.0.1"]`,
		},
		{
			name:     "ips_with_ports",
			input:    []string{"192.168.1.1:8080", "10.0.0.1:3000"},
			expected: `["192.168.1.1:8080","10.0.0.1:3000"]`,
		},
		{
			name:     "ipv6_addresses",
			input:    []string{"2001:db8::1", "fe80::1"},
			expected: `["2001:db8::1","fe80::1"]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := test.input.Value()
			require.NoError(t, err)
			assert.JSONEq(t, test.expected, result.(string))
		})
	}
}

func TestNodeOS_ScanAndValue_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []NodeOS{
		NodeOSLinux,
		NodeOSWindows,
		NodeOSMacOS,
		NodeOSOther,
	}

	for _, original := range tests {
		t.Run(string(original), func(t *testing.T) {
			t.Parallel()

			value, err := original.Value()
			require.NoError(t, err)

			var result NodeOS
			err = result.Scan(value)
			require.NoError(t, err)

			assert.Equal(t, original, result)
		})
	}
}

func TestIPList_ScanAndValue_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input IPList
	}{
		{
			name:  "nil_list",
			input: nil,
		},
		{
			name:  "empty_list",
			input: []string{},
		},
		{
			name:  "single_ip",
			input: []string{"192.168.1.1"},
		},
		{
			name:  "multiple_ips",
			input: []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"},
		},
		{
			name:  "ipv4_and_ipv6_mixed",
			input: []string{"192.168.1.1", "2001:db8::1", "10.0.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			value, err := test.input.Value()
			require.NoError(t, err)

			var result IPList
			err = result.Scan(value)
			require.NoError(t, err)

			if test.input == nil {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, test.input, result)
			}
		})
	}
}
