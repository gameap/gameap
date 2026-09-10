package plugin

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainServerSettingToProto(t *testing.T) {
	t.Parallel()

	// A value read from the database is type-guessed by Scan ("007" becomes
	// the int 7); plugins must still see the stored text.
	var scanned domain.ServerSettingValue
	require.NoError(t, scanned.Scan([]byte("007")))

	tests := []struct {
		name    string
		setting domain.ServerSetting
		want    string
	}{
		{
			name:    "string_value",
			setting: domain.ServerSetting{ID: 1, ServerID: 10, Name: "map", Value: domain.NewServerSettingValue("de_dust2")},
			want:    "de_dust2",
		},
		{
			name:    "int_value_is_stringified",
			setting: domain.ServerSetting{ID: 2, ServerID: 10, Name: "tickrate", Value: domain.NewServerSettingValue(128)},
			want:    "128",
		},
		{
			name:    "bool_value_is_stringified",
			setting: domain.ServerSetting{ID: 3, ServerID: 10, Name: "private", Value: domain.NewServerSettingValue(true)},
			want:    "true",
		},
		{
			name:    "scanned_text_is_kept_verbatim",
			setting: domain.ServerSetting{ID: 4, ServerID: 10, Name: "code", Value: scanned},
			want:    "007",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := DomainServerSettingToProto(tt.setting)

			assert.Equal(t, uint64(tt.setting.ID), got.Id)
			assert.Equal(t, uint64(tt.setting.ServerID), got.ServerId)
			assert.Equal(t, tt.setting.Name, got.Name)
			assert.Equal(t, tt.want, got.Value)
		})
	}
}
