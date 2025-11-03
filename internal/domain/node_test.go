package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseNodeOS(t *testing.T) {
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
			result := ParseNodeOS(test.input)
			assert.Equal(t, test.expectedOS, result)
		})
	}
}
