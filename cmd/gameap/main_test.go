package main

import (
	"bytes"
	"testing"

	"github.com/gameap/gameap/internal/application/defaults"
	"github.com/stretchr/testify/assert"
)

func TestIsVersionCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no_args", args: nil, want: false},
		{name: "version_command", args: []string{"version"}, want: true},
		{name: "version_command_with_extra_args", args: []string{"version", "--env", ".env"}, want: true},
		{name: "version_flag", args: []string{"--version"}, want: false},
		{name: "version_not_first", args: []string{"--env", ".env", "version"}, want: false},
		{name: "other_args", args: []string{"--env", ".env"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isVersionCommand(tt.args))
		})
	}
}

func TestPrintVersion(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}

	printVersion(buf)

	assert.Equal(t, "GameAP "+defaults.Version+"\nBuild date: "+defaults.BuildDate+"\n", buf.String())
}
