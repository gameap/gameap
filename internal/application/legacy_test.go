package application

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertLegacyEnvToCurrentEnv(t *testing.T) {
	tests := []struct {
		name       string
		legacyVars map[string]string
		expected   map[string]string
		setup      func()
		cleanup    func()
	}{
		{
			name: "database_connection_conversion",
			legacyVars: map[string]string{
				"DB_CONNECTION": "mysql",
			},
			expected: map[string]string{
				"DATABASE_DRIVER": "mysql",
			},
		},
		{
			name: "database_url_construction",
			legacyVars: map[string]string{
				"DB_CONNECTION": "mysql",
				"DB_HOST":       "127.0.0.1",
				"DB_PORT":       "3306",
				"DB_DATABASE":   "homestead",
				"DB_USERNAME":   "homestead",
				"DB_PASSWORD":   "secret",
			},
			expected: map[string]string{
				"DATABASE_DRIVER": "mysql",
				"DATABASE_URL":    "homestead:secret@tcp(127.0.0.1:3306)/homestead?parseTime=true",
			},
		},
		{
			name: "database_url_with_default_port",
			legacyVars: map[string]string{
				"DB_HOST":     "127.0.0.1",
				"DB_DATABASE": "homestead",
				"DB_USERNAME": "homestead",
				"DB_PASSWORD": "secret",
			},
			expected: map[string]string{
				"DATABASE_URL": "homestead:secret@tcp(127.0.0.1:3306)/homestead?parseTime=true",
			},
		},
		{
			name: "encryption_key_conversion",
			legacyVars: map[string]string{
				"APP_KEY": "base64:somekey",
			},
			expected: map[string]string{
				"ENCRYPTION_KEY": "base64:somekey",
				"AUTH_SECRET":    "base64:somekey",
			},
		},
		{
			name: "logger_level_debug",
			legacyVars: map[string]string{
				"APP_DEBUG": "true",
			},
			expected: map[string]string{
				"LOGGER_LEVEL": "debug",
			},
		},
		{
			name: "logger_level_info",
			legacyVars: map[string]string{
				"APP_DEBUG": "false",
			},
			expected: map[string]string{
				"LOGGER_LEVEL": "info",
			},
		},
		{
			name: "cache_driver_file_to_memory",
			legacyVars: map[string]string{
				"CACHE_DRIVER": "file",
			},
			expected: map[string]string{
				"CACHE_DRIVER": "memory",
			},
		},
		{
			name: "cache_driver_redis",
			legacyVars: map[string]string{
				"CACHE_DRIVER": "redis",
			},
			expected: map[string]string{
				"CACHE_DRIVER": "redis",
			},
		},
		{
			name: "redis_addr_construction",
			legacyVars: map[string]string{
				"REDIS_HOST": "127.0.0.1",
				"REDIS_PORT": "6379",
			},
			expected: map[string]string{
				"CACHE_REDIS_ADDR": "127.0.0.1:6379",
			},
		},
		{
			name: "redis_addr_with_default_port",
			legacyVars: map[string]string{
				"REDIS_HOST": "localhost",
			},
			expected: map[string]string{
				"CACHE_REDIS_ADDR": "localhost:6379",
			},
		},
		{
			name: "redis_password_conversion",
			legacyVars: map[string]string{
				"REDIS_PASSWORD": "mypassword",
			},
			expected: map[string]string{
				"CACHE_REDIS_PASSWORD": "mypassword",
			},
		},
		{
			name: "redis_password_null_ignored",
			legacyVars: map[string]string{
				"REDIS_PASSWORD": "null",
			},
			expected: map[string]string{},
		},
		{
			name: "complete_legacy_config",
			legacyVars: map[string]string{
				"APP_KEY":        "base64:somekey",
				"APP_DEBUG":      "true",
				"DB_CONNECTION":  "mysql",
				"DB_HOST":        "127.0.0.1",
				"DB_PORT":        "3306",
				"DB_DATABASE":    "homestead",
				"DB_USERNAME":    "homestead",
				"DB_PASSWORD":    "secret",
				"CACHE_DRIVER":   "file",
				"REDIS_HOST":     "127.0.0.1",
				"REDIS_PORT":     "6379",
				"REDIS_PASSWORD": "null",
			},
			expected: map[string]string{
				"DATABASE_DRIVER":  "mysql",
				"DATABASE_URL":     "homestead:secret@tcp(127.0.0.1:3306)/homestead?parseTime=true",
				"ENCRYPTION_KEY":   "base64:somekey",
				"AUTH_SECRET":      "base64:somekey",
				"LOGGER_LEVEL":     "debug",
				"CACHE_DRIVER":     "memory",
				"CACHE_REDIS_ADDR": "127.0.0.1:6379",
			},
		},
		{
			name: "does_not_override_existing_values",
			legacyVars: map[string]string{
				"DB_CONNECTION": "mysql",
				"APP_DEBUG":     "true",
			},
			expected: map[string]string{
				"DATABASE_DRIVER": "postgres",
				"LOGGER_LEVEL":    "warn",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "does_not_override_existing_values" {
				t.Setenv("DATABASE_DRIVER", "postgres")
				t.Setenv("LOGGER_LEVEL", "warn")
			}

			for key := range tt.expected {
				if os.Getenv(key) == "" {
					t.Setenv(key, "")
				}
			}

			convertLegacyEnvToCurrentEnv(tt.legacyVars)

			for key, expectedValue := range tt.expected {
				actualValue := os.Getenv(key)
				assert.Equal(t, expectedValue, actualValue, "Environment variable %s should be %s, got %s", key, expectedValue, actualValue)
			}
		})
	}
}

func TestParseLegacyEnvFile(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expected    map[string]string
		expectError bool
	}{
		{
			name: "valid_env_file",
			fileContent: `APP_NAME=GameAP
APP_ENV=local
DB_HOST=127.0.0.1
DB_PORT=3306`,
			expected: map[string]string{
				"APP_NAME": "GameAP",
				"APP_ENV":  "local",
				"DB_HOST":  "127.0.0.1",
				"DB_PORT":  "3306",
			},
		},
		{
			name: "env_file_with_comments",
			fileContent: `# This is a comment
APP_NAME=GameAP
# Another comment
DB_HOST=127.0.0.1`,
			expected: map[string]string{
				"APP_NAME": "GameAP",
				"DB_HOST":  "127.0.0.1",
			},
		},
		{
			name: "env_file_with_empty_lines",
			fileContent: `APP_NAME=GameAP

DB_HOST=127.0.0.1

`,
			expected: map[string]string{
				"APP_NAME": "GameAP",
				"DB_HOST":  "127.0.0.1",
			},
		},
		{
			name: "env_file_with_equals_in_value",
			fileContent: `APP_KEY=base64:abc=def==
DB_HOST=127.0.0.1`,
			expected: map[string]string{
				"APP_KEY": "base64:abc=def==",
				"DB_HOST": "127.0.0.1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp(t.TempDir(), "test-env-*.env")
			assert.NoError(t, err)

			_, err = tmpFile.WriteString(tt.fileContent)
			assert.NoError(t, err)
			_ = tmpFile.Close()

			result, err := parseLegacyEnvFile(tmpFile.Name())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestLoadLegacyConfig_FileNotExists(t *testing.T) {
	t.Setenv("LEGACY_ENV_PATH", "/nonexistent/path/.env")

	err := loadLegacyEnv("")
	assert.NoError(t, err)
}

func TestLoadLegacyConfig_ValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-env-*.env")
	assert.NoError(t, err)

	envContent := `DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=testdb
DB_USERNAME=testuser
DB_PASSWORD=testpass
APP_DEBUG=true`

	_, err = tmpFile.WriteString(envContent)
	assert.NoError(t, err)
	_ = tmpFile.Close()

	t.Setenv("LEGACY_ENV_PATH", tmpFile.Name())
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("LOGGER_LEVEL", "")

	err = loadLegacyEnv("")
	assert.NoError(t, err)

	assert.Equal(t, "mysql", os.Getenv("DATABASE_DRIVER"))
	assert.Equal(t, "testuser:testpass@tcp(127.0.0.1:3306)/testdb?parseTime=true", os.Getenv("DATABASE_URL"))
	assert.Equal(t, "debug", os.Getenv("LOGGER_LEVEL"))
}
