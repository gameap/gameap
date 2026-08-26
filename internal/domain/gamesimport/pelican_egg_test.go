package gamesimport

import (
	"encoding/json"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPelicanEggConfig_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantConfig PelicanEggConfig
		wantError  string
	}{
		{
			name: "config_fields_as_strings",
			input: `{
				"files": "{}",
				"startup": "{\"done\": \"Bot logged in as \"}",
				"logs": "{}",
				"stop": "^C"
			}`,
			wantConfig: PelicanEggConfig{
				Stop: "^C",
				Startup: PelicanEggConfigStartup{
					Done: "Bot logged in as ",
				},
				Files: map[string]PelicanEggConfigFile{},
			},
		},
		{
			name: "config_fields_as_objects",
			input: `{
				"files": {},
				"startup": {"done": "Server started"},
				"logs": {},
				"stop": "stop"
			}`,
			wantConfig: PelicanEggConfig{
				Stop: "stop",
				Startup: PelicanEggConfigStartup{
					Done: "Server started",
				},
				Files: map[string]PelicanEggConfigFile{},
			},
		},
		{
			name: "config_with_complex_files_as_string",
			input: `{
				"files": "{\"config.json\": {\"parser\": \"json\", \"find\": {\"server.port\": \"{{server.build.default.port}}\"}}}"
			}`,
			wantConfig: PelicanEggConfig{
				Files: map[string]PelicanEggConfigFile{
					"config.json": {
						Parser: "json",
						Find: map[string]any{
							"server.port": "{{server.build.default.port}}",
						},
					},
				},
			},
		},
		{
			name: "config_with_windows_newlines",
			input: `{
				"files": "{}",
				"startup": "{\r\n    \"done\": \"Bot logged in as \"\r\n}",
				"logs": "{}",
				"stop": "^C"
			}`,
			wantConfig: PelicanEggConfig{
				Stop: "^C",
				Startup: PelicanEggConfigStartup{
					Done: "Bot logged in as ",
				},
				Files: map[string]PelicanEggConfigFile{},
			},
		},
		{
			name: "config_with_complex_files_as_object",
			input: `{
				"files": {
					"server.properties": {
						"parser": "properties",
						"find": {
							"server-port": "{{server.build.default.port}}"
						}
					}
				},
				"startup": {"done": "Done"},
				"stop": "stop"
			}`,
			wantConfig: PelicanEggConfig{
				Stop: "stop",
				Startup: PelicanEggConfigStartup{
					Done: "Done",
				},
				Files: map[string]PelicanEggConfigFile{
					"server.properties": {
						Parser: "properties",
						Find: map[string]any{
							"server-port": "{{server.build.default.port}}",
						},
					},
				},
			},
		},
		{
			name: "config_with_empty_strings",
			input: `{
				"files": "",
				"startup": "",
				"logs": "",
				"stop": "quit"
			}`,
			wantConfig: PelicanEggConfig{
				Stop:  "quit",
				Files: map[string]PelicanEggConfigFile{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var config PelicanEggConfig
			err := json.Unmarshal([]byte(tt.input), &config)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantConfig.Stop, config.Stop)
			assert.Equal(t, tt.wantConfig.Startup.Done, config.Startup.Done)
			assert.Equal(t, len(tt.wantConfig.Files), len(config.Files))

			for key, wantFile := range tt.wantConfig.Files {
				gotFile, ok := config.Files[key]
				require.True(t, ok, "expected file %s to exist", key)
				assert.Equal(t, wantFile.Parser, gotFile.Parser)
			}
		})
	}
}

func TestParsePelicanEgg_WithStringConfigFields(t *testing.T) {
	t.Parallel()

	input := `{
		"meta": {
			"version": "1.0",
			"update_url": "https://example.com",
			"exported_at": "2024-01-01"
		},
		"uuid": "test-uuid",
		"author": "test@example.com",
		"name": "Test Game",
		"description": "Test description",
		"features": [],
		"docker_images": {
			"ghcr.io/test/image:latest": "ghcr.io/test/image:latest"
		},
		"file_denylist": [],
		"startup": "./start.sh",
		"config": {
			"files": "{}",
			"startup": "{\r\n    \"done\": \"Server started\"\r\n}",
			"logs": "{}",
			"stop": "^C"
		},
		"scripts": {
			"installation": {
				"script": "#!/bin/bash",
				"container": "alpine",
				"entrypoint": "bash"
			}
		},
		"variables": []
	}`

	egg, err := ParsePelicanEgg([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "test-uuid", egg.UUID)
	assert.Equal(t, "Test Game", egg.Name)
	assert.Equal(t, "^C", egg.Config.Stop)
	assert.Equal(t, "Server started", egg.Config.Startup.Done)

	require.NotNil(t, egg.Raw)
	assert.Equal(t, "test-uuid", egg.Raw["uuid"])
	assert.Equal(t, "Test Game", egg.Raw["name"])
}

func TestParsePelicanEgg_RawPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	input := `{
		"_comment": "DO NOT EDIT: FILE GENERATED AUTOMATICALLY BY PTERODACTYL PANEL",
		"meta": {
			"version": "PTDL_v2",
			"update_url": null
		},
		"exported_at": "2024-04-02T14:13:31+02:00",
		"uuid": "preserve-uuid",
		"name": "Test Preserve",
		"author": "test@example.com",
		"description": "Test description",
		"features": ["feature1"],
		"docker_images": {
			"Nodejs 18": "ghcr.io/parkervcp/yolks:nodejs_18"
		},
		"file_denylist": ["secret.txt"],
		"startup": "./start.sh",
		"config": {},
		"scripts": {
			"installation": {
				"script": "#!/bin/bash",
				"container": "alpine",
				"entrypoint": "bash"
			}
		},
		"variables": []
	}`

	egg, err := ParsePelicanEgg([]byte(input))
	require.NoError(t, err)

	require.NotNil(t, egg.Raw)
	assert.Equal(t, "DO NOT EDIT: FILE GENERATED AUTOMATICALLY BY PTERODACTYL PANEL", egg.Raw["_comment"])
	assert.Equal(t, "2024-04-02T14:13:31+02:00", egg.Raw["exported_at"])
	assert.Equal(t, "preserve-uuid", egg.Raw["uuid"])

	meta, ok := egg.Raw["meta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "PTDL_v2", meta["version"])

	dockerImages, ok := egg.Raw["docker_images"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ghcr.io/parkervcp/yolks:nodejs_18", dockerImages["Nodejs 18"])

	features, ok := egg.Raw["features"].([]any)
	require.True(t, ok)
	require.Len(t, features, 1)
	assert.Equal(t, "feature1", features[0])

	fileDenylist, ok := egg.Raw["file_denylist"].([]any)
	require.True(t, ok)
	require.Len(t, fileDenylist, 1)
	assert.Equal(t, "secret.txt", fileDenylist[0])
}

func TestPelicanEgg_GetStartupCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		egg      *PelicanEgg
		expected string
	}{
		{
			name: "startup_commands_Default_takes_priority",
			egg: &PelicanEgg{
				Startup: "./legacy_server",
				StartupCommands: map[string]string{
					"Default": "./new_server",
				},
			},
			expected: "./new_server",
		},
		{
			name: "falls_back_to_startup_when_no_startup_commands",
			egg: &PelicanEgg{
				Startup: "./legacy_server",
			},
			expected: "./legacy_server",
		},
		{
			name: "falls_back_to_startup_when_Default_is_empty",
			egg: &PelicanEgg{
				Startup: "./legacy_server",
				StartupCommands: map[string]string{
					"Default": "",
				},
			},
			expected: "./legacy_server",
		},
		{
			name: "falls_back_to_startup_when_Default_key_missing",
			egg: &PelicanEgg{
				Startup: "./legacy_server",
				StartupCommands: map[string]string{
					"Other": "./other_server",
				},
			},
			expected: "./legacy_server",
		},
		{
			name: "empty_startup_commands_nil",
			egg: &PelicanEgg{
				Startup:         "./server",
				StartupCommands: nil,
			},
			expected: "./server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.egg.GetStartupCommand()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlexibleRules_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  FlexibleRules
		wantError string
	}{
		{
			name:     "string_format",
			input:    `"required|string|max:64"`,
			expected: FlexibleRules{"required|string|max:64"},
		},
		{
			name:     "array_format",
			input:    `["required", "string", "max:64"]`,
			expected: FlexibleRules{"required", "string", "max:64"},
		},
		{
			name:     "empty_string",
			input:    `""`,
			expected: FlexibleRules{},
		},
		{
			name:     "empty_array",
			input:    `[]`,
			expected: FlexibleRules{},
		},
		{
			name:      "invalid_format",
			input:     `123`,
			wantError: "rules must be string or array of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var rules FlexibleRules
			err := json.Unmarshal([]byte(tt.input), &rules)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, rules)
		})
	}
}

func TestParsePelicanEgg_PLCN_v3_Format(t *testing.T) {
	t.Parallel()

	input := `{
		"meta": {
			"version": "PLCN_v3",
			"update_url": "https://example.com",
			"exported_at": "2024-01-01"
		},
		"uuid": "plcn-v3-uuid",
		"author": "test@example.com",
		"name": "Test PLCN v3 Game",
		"description": "Test description",
		"features": [],
		"docker_images": {
			"ghcr.io/test/image:latest": "ghcr.io/test/image:latest"
		},
		"file_denylist": [],
		"startup_commands": {
			"Default": "./start.sh -port {{server.build.default.port}}"
		},
		"config": {
			"files": "{}",
			"startup": "{\"done\": \"Server started\"}",
			"logs": "{}",
			"stop": "^C"
		},
		"scripts": {
			"installation": {
				"script": "#!/bin/bash",
				"container": "alpine",
				"entrypoint": "bash"
			}
		},
		"variables": [
			{
				"name": "Server Name",
				"description": "Name of the server",
				"env_variable": "SERVER_NAME",
				"default_value": "My Server",
				"user_viewable": true,
				"user_editable": true,
				"rules": ["required", "string", "max:64"],
				"field_type": "text"
			}
		]
	}`

	egg, err := ParsePelicanEgg([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "plcn-v3-uuid", egg.UUID)
	assert.Equal(t, "Test PLCN v3 Game", egg.Name)
	assert.Equal(t, "PLCN_v3", egg.Meta.Version)
	assert.Equal(t, "", egg.Startup)
	assert.Equal(t, "./start.sh -port {{server.build.default.port}}", egg.StartupCommands["Default"])
	assert.Equal(t, "./start.sh -port {{server.build.default.port}}", egg.GetStartupCommand())

	require.Len(t, egg.Variables, 1)
	assert.Equal(t, FlexibleRules{"required", "string", "max:64"}, egg.Variables[0].Rules)
}

func TestParsePelicanEgg_WithObjectConfigFields(t *testing.T) {
	t.Parallel()

	input := `{
		"meta": {
			"version": "1.0",
			"update_url": "https://example.com",
			"exported_at": "2024-01-01"
		},
		"uuid": "test-uuid-2",
		"author": "test@example.com",
		"name": "Test Game 2",
		"description": "Test description",
		"features": [],
		"docker_images": {},
		"file_denylist": [],
		"startup": "./start.sh",
		"config": {
			"files": {
				"config.yml": {
					"parser": "yaml",
					"find": {
						"port": "{{server.build.default.port}}"
					}
				}
			},
			"startup": {
				"done": "Ready!"
			},
			"logs": {},
			"stop": "stop"
		},
		"scripts": {
			"installation": {
				"script": "#!/bin/bash",
				"container": "alpine",
				"entrypoint": "bash"
			}
		},
		"variables": []
	}`

	egg, err := ParsePelicanEgg([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "test-uuid-2", egg.UUID)
	assert.Equal(t, "stop", egg.Config.Stop)
	assert.Equal(t, "Ready!", egg.Config.Startup.Done)
	require.Len(t, egg.Config.Files, 1)
	assert.Equal(t, "yaml", egg.Config.Files["config.yml"].Parser)
}

func TestDetectFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "JSON_format",
			input:    `{"meta": {"version": "1.0"}}`,
			expected: "json",
		},
		{
			name:     "JSON_format_with_whitespace",
			input:    `  { "meta": {"version": "1.0"}}`,
			expected: "json",
		},
		{
			name:     "JSON_format_with_newline",
			input:    "\n{\n  \"meta\": {\"version\": \"1.0\"}}",
			expected: "json",
		},
		{
			name:     "YAML_format",
			input:    "meta:\n  version: '1.0'",
			expected: "yaml",
		},
		{
			name:     "YAML_format_with_comment",
			input:    "# Comment\nmeta:\n  version: '1.0'",
			expected: "yaml",
		},
		{
			name:     "empty_input_defaults_to_yaml",
			input:    "",
			expected: "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := detectFormat([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  FlexibleStringSlice
		wantError string
	}{
		{
			name:     "array_with_values",
			input:    `["file1.txt", "file2.txt"]`,
			expected: FlexibleStringSlice{"file1.txt", "file2.txt"},
		},
		{
			name:     "empty_array",
			input:    `[]`,
			expected: FlexibleStringSlice{},
		},
		{
			name:      "invalid_format_object",
			input:     `{}`,
			wantError: "file_denylist must be an array of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var result FlexibleStringSlice
			err := json.Unmarshal([]byte(tt.input), &result)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlexibleStringSlice_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  FlexibleStringSlice
		wantError string
	}{
		{
			name:     "array_with_values",
			input:    "- file1.txt\n- file2.txt",
			expected: FlexibleStringSlice{"file1.txt", "file2.txt"},
		},
		{
			name:     "empty_array",
			input:    "[]",
			expected: FlexibleStringSlice{},
		},
		{
			name:     "empty_object_treated_as_empty_array",
			input:    "{}",
			expected: FlexibleStringSlice{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var result FlexibleStringSlice
			err := yaml.Unmarshal([]byte(tt.input), &result)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlexibleRules_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  FlexibleRules
		wantError string
	}{
		{
			name:     "array_format",
			input:    "- required\n- string\n- max:64",
			expected: FlexibleRules{"required", "string", "max:64"},
		},
		{
			name:     "string_format",
			input:    "required|string|max:64",
			expected: FlexibleRules{"required|string|max:64"},
		},
		{
			name:     "empty_array",
			input:    "[]",
			expected: FlexibleRules{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var result FlexibleRules
			err := yaml.Unmarshal([]byte(tt.input), &result)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPelicanEggConfig_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantConfig PelicanEggConfig
		wantError  string
	}{
		{
			name: "direct_object_format",
			input: `files:
  server.properties:
    parser: properties
    find:
      server-port: "{{server.build.default.port}}"
startup:
  done: "Server started"
stop: stop`,
			wantConfig: PelicanEggConfig{
				Stop: "stop",
				Startup: PelicanEggConfigStartup{
					Done: "Server started",
				},
				Files: map[string]PelicanEggConfigFile{
					"server.properties": {
						Parser: "properties",
						Find: map[string]any{
							"server-port": "{{server.build.default.port}}",
						},
					},
				},
			},
		},
		{
			name: "empty_files_and_startup",
			input: `files: {}
startup: {}
stop: ^C`,
			wantConfig: PelicanEggConfig{
				Stop:    "^C",
				Startup: PelicanEggConfigStartup{},
				Files:   map[string]PelicanEggConfigFile{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var config PelicanEggConfig
			err := yaml.Unmarshal([]byte(tt.input), &config)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantConfig.Stop, config.Stop)
			assert.Equal(t, tt.wantConfig.Startup.Done, config.Startup.Done)
			assert.Equal(t, len(tt.wantConfig.Files), len(config.Files))

			for key, wantFile := range tt.wantConfig.Files {
				gotFile, ok := config.Files[key]
				require.True(t, ok, "expected file %s to exist", key)
				assert.Equal(t, wantFile.Parser, gotFile.Parser)
			}
		})
	}
}

func TestParsePelicanEgg_YAML_Format(t *testing.T) {
	t.Parallel()

	input := `meta:
  version: PLCN_v3
  update_url: https://example.com
  exported_at: "2024-01-01"
uuid: yaml-test-uuid
author: test@example.com
name: Vanilla Minecraft
description: Minecraft server using the official Mojang binaries
features:
  - eula
  - java_version
docker_images:
  Java 21: ghcr.io/pelican-eggs/yolks:java_21
file_denylist: {}
startup_commands:
  Default: java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}}
config:
  files:
    server.properties:
      parser: properties
      find:
        server-port: "{{server.build.default.port}}"
  startup:
    done: "Done"
    userInteraction: []
  stop: stop
  logs: {}
scripts:
  installation:
    script: |
      #!/bin/bash
      echo "Installing"
    container: ghcr.io/pelican-eggs/installers:debian
    entrypoint: bash
variables:
  - name: Server Jar File
    description: The name of the server jar file
    env_variable: SERVER_JARFILE
    default_value: server.jar
    user_viewable: true
    user_editable: true
    rules:
      - required
      - regex:/^[\w\d._-]+\.jar$/
    field_type: text`

	egg, err := ParsePelicanEgg([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "yaml-test-uuid", egg.UUID)
	assert.Equal(t, "Vanilla Minecraft", egg.Name)
	assert.Equal(t, "PLCN_v3", egg.Meta.Version)
	assert.Equal(t, "test@example.com", egg.Author)
	assert.Equal(t, "Minecraft server using the official Mojang binaries", egg.Description)

	require.Len(t, egg.Features, 2)
	assert.Contains(t, egg.Features, "eula")
	assert.Contains(t, egg.Features, "java_version")

	require.Len(t, egg.DockerImages, 1)
	assert.Equal(t, "ghcr.io/pelican-eggs/yolks:java_21", egg.DockerImages["Java 21"])

	assert.Equal(t, FlexibleStringSlice{}, egg.FileDenylist)

	assert.Equal(t, "java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}}", egg.StartupCommands["Default"])
	assert.Equal(t, "java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JARFILE}}", egg.GetStartupCommand())

	assert.Equal(t, "stop", egg.Config.Stop)
	assert.Equal(t, "Done", egg.Config.Startup.Done)
	require.Len(t, egg.Config.Files, 1)
	assert.Equal(t, "properties", egg.Config.Files["server.properties"].Parser)

	assert.Equal(t, "ghcr.io/pelican-eggs/installers:debian", egg.Scripts.Installation.Container)
	assert.Equal(t, "bash", egg.Scripts.Installation.Entrypoint)
	assert.Contains(t, egg.Scripts.Installation.Script, "Installing")

	require.Len(t, egg.Variables, 1)
	assert.Equal(t, "Server Jar File", egg.Variables[0].Name)
	assert.Equal(t, "SERVER_JARFILE", egg.Variables[0].EnvVariable)
	assert.Equal(t, "server.jar", egg.Variables[0].DefaultValue)
	assert.True(t, egg.Variables[0].UserViewable)
	assert.True(t, egg.Variables[0].UserEditable)
	assert.Equal(t, FlexibleRules{"required", "regex:/^[\\w\\d._-]+\\.jar$/"}, egg.Variables[0].Rules)

	require.NotNil(t, egg.Raw)
	assert.Equal(t, "yaml-test-uuid", egg.Raw["uuid"])
}

func TestParsePelicanEgg_YAML_Format_WithFileDenylistArray(t *testing.T) {
	t.Parallel()

	input := `meta:
  version: PLCN_v3
uuid: yaml-denylist-test
author: test@example.com
name: Test Game
description: Test
features: []
docker_images: {}
file_denylist:
  - secret.txt
  - .env
startup_commands:
  Default: ./start.sh
config:
  files: {}
  startup: {}
  stop: ^C
scripts:
  installation:
    script: "#!/bin/bash"
    container: alpine
    entrypoint: bash
variables: []`

	egg, err := ParsePelicanEgg([]byte(input))
	require.NoError(t, err)

	assert.Equal(t, "yaml-denylist-test", egg.UUID)
	require.Len(t, egg.FileDenylist, 2)
	assert.Contains(t, []string(egg.FileDenylist), "secret.txt")
	assert.Contains(t, []string(egg.FileDenylist), ".env")
}

func TestParsePelicanEgg_EmptyInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{
			name:      "empty_string",
			input:     "",
			wantError: "empty input data",
		},
		{
			name:      "whitespace_only",
			input:     "   \n\t  ",
			wantError: "empty input data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParsePelicanEgg([]byte(tt.input))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}
