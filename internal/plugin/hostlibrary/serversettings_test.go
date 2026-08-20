package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/serversettings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSettingsService_FindServerSettings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.ServerSettingRepository)
		request   *serversettings.FindServerSettingsRequest
		wantCount int
		wantNames []string
	}{
		{
			name: "returns_settings_for_server",
			setupRepo: func(r *inmemory.ServerSettingRepository) {
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "maxplayers",
					Value:    domain.NewServerSettingValue("32"),
				})
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "hostname",
					Value:    domain.NewServerSettingValue("My Server"),
				})
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 2,
					Name:     "maxplayers",
					Value:    domain.NewServerSettingValue("16"),
				})
			},
			request: &serversettings.FindServerSettingsRequest{
				ServerId: 1,
			},
			wantCount: 2,
			wantNames: []string{"maxplayers", "hostname"},
		},
		{
			name: "filter_by_names",
			setupRepo: func(r *inmemory.ServerSettingRepository) {
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "maxplayers",
					Value:    domain.NewServerSettingValue("32"),
				})
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "hostname",
					Value:    domain.NewServerSettingValue("My Server"),
				})
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "password",
					Value:    domain.NewServerSettingValue("secret"),
				})
			},
			request: &serversettings.FindServerSettingsRequest{
				ServerId: 1,
				Names:    []string{"maxplayers", "password"},
			},
			wantCount: 2,
			wantNames: []string{"maxplayers", "password"},
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.ServerSettingRepository) {},
			request: &serversettings.FindServerSettingsRequest{
				ServerId: 1,
			},
			wantCount: 0,
			wantNames: []string{},
		},
		{
			name: "nonexistent_server_returns_empty",
			setupRepo: func(r *inmemory.ServerSettingRepository) {
				_ = r.Save(context.Background(), &domain.ServerSetting{
					ServerID: 1,
					Name:     "maxplayers",
					Value:    domain.NewServerSettingValue("32"),
				})
			},
			request: &serversettings.FindServerSettingsRequest{
				ServerId: 999,
			},
			wantCount: 0,
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerSettingRepository()
			tt.setupRepo(repo)

			svc := NewServerSettingsService(repo)
			resp, err := svc.FindServerSettings(context.Background(), tt.request)

			require.NoError(t, err)
			require.Len(t, resp.Settings, tt.wantCount)

			if len(tt.wantNames) > 0 {
				actualNames := make([]string, len(resp.Settings))
				for i, setting := range resp.Settings {
					actualNames[i] = setting.Name
				}
				for _, wantName := range tt.wantNames {
					assert.Contains(t, actualNames, wantName)
				}
			}
		})
	}
}

func TestServerSettingsService_SaveServerSetting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		request   *serversettings.SaveServerSettingRequest
		wantError string
	}{
		{
			name: "save_new_setting_success",
			request: &serversettings.SaveServerSettingRequest{
				ServerId: 1,
				Name:     "maxplayers",
				Value:    "32",
			},
		},
		{
			name: "save_setting_with_empty_value",
			request: &serversettings.SaveServerSettingRequest{
				ServerId: 1,
				Name:     "emptyvalue",
				Value:    "",
			},
		},
		{
			name: "save_setting_with_special_characters",
			request: &serversettings.SaveServerSettingRequest{
				ServerId: 1,
				Name:     "password",
				Value:    "p@$$w0rd!#%",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewServerSettingRepository()
			svc := NewServerSettingsService(repo)

			resp, err := svc.SaveServerSetting(context.Background(), tt.request)

			require.NoError(t, err)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.True(t, resp.Success)
			assert.Nil(t, resp.Error)

			findResp, err := svc.FindServerSettings(context.Background(), &serversettings.FindServerSettingsRequest{
				ServerId: tt.request.ServerId,
				Names:    []string{tt.request.Name},
			})
			require.NoError(t, err)
			require.Len(t, findResp.Settings, 1)
			assert.Equal(t, tt.request.Name, findResp.Settings[0].Name)
			assert.Equal(t, tt.request.Value, findResp.Settings[0].Value)
		})
	}
}

func TestServerSettingsService_SaveServerSetting_CreatesDuplicate(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewServerSettingRepository()
	_ = repo.Save(context.Background(), &domain.ServerSetting{
		ServerID: 1,
		Name:     "maxplayers",
		Value:    domain.NewServerSettingValue("16"),
	})

	svc := NewServerSettingsService(repo)

	resp, err := svc.SaveServerSetting(context.Background(), &serversettings.SaveServerSettingRequest{
		ServerId: 1,
		Name:     "maxplayers",
		Value:    "32",
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)

	findResp, err := svc.FindServerSettings(context.Background(), &serversettings.FindServerSettingsRequest{
		ServerId: 1,
		Names:    []string{"maxplayers"},
	})
	require.NoError(t, err)
	require.Len(t, findResp.Settings, 2)
}

func TestConvertServerSettingsToProto(t *testing.T) {
	t.Parallel()
	settings := []domain.ServerSetting{
		{
			ID:       1,
			ServerID: 10,
			Name:     "maxplayers",
			Value:    domain.NewServerSettingValue("32"),
		},
		{
			ID:       2,
			ServerID: 10,
			Name:     "hostname",
			Value:    domain.NewServerSettingValue("Test Server"),
		},
	}

	result := convertServerSettingsToProto(settings)

	require.Len(t, result, 2)
	assert.Equal(t, uint64(1), result[0].Id)
	assert.Equal(t, uint64(10), result[0].ServerId)
	assert.Equal(t, "maxplayers", result[0].Name)
	assert.Equal(t, "32", result[0].Value)

	assert.Equal(t, uint64(2), result[1].Id)
	assert.Equal(t, uint64(10), result[1].ServerId)
	assert.Equal(t, "hostname", result[1].Name)
	assert.Equal(t, "Test Server", result[1].Value)
}

func TestNewServerSettingsHostLibrary(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewServerSettingRepository()
	lib := NewServerSettingsHostLibrary(repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
