package domain_test

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestNewPluginScheduledTaskErrorPolicyFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  domain.PluginScheduledTaskErrorPolicy
	}{
		{name: "retry", input: "retry", want: domain.PluginScheduledTaskErrorPolicyRetry},
		{name: "ignore", input: "ignore", want: domain.PluginScheduledTaskErrorPolicyIgnore},
		{
			name:  "empty_string_falls_back_to_ignore",
			input: "",
			want:  domain.PluginScheduledTaskErrorPolicyIgnore,
		},
		{
			name:  "unknown_value_falls_back_to_ignore",
			input: "abort",
			want:  domain.PluginScheduledTaskErrorPolicyIgnore,
		},
		{
			name:  "uppercase_is_not_recognised",
			input: "RETRY",
			want:  domain.PluginScheduledTaskErrorPolicyIgnore,
		},
		{
			name:  "padded_value_is_not_recognised",
			input: " retry ",
			want:  domain.PluginScheduledTaskErrorPolicyIgnore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			stored := tt.input

			// ACT
			got := domain.NewPluginScheduledTaskErrorPolicyFromString(stored)

			// ASSERT
			assert.Equal(t, tt.want, got, "error policy for %q", stored)
		})
	}
}
