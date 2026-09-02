package domain

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodePatch_ValidateMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		patch     NodePatch
		wantError string
	}{
		{
			name:  "empty_patch",
			patch: NodePatch{},
		},
		{
			// The limit is on the serialized value, so the two quotes json.Marshal
			// adds count against it.
			name:  "string_value_at_limit",
			patch: NodePatch{Metadata: Metadata{"key": strings.Repeat("a", NodeMetadataValueMaxLength-2)}},
		},
		{
			name:      "string_value_over_limit",
			patch:     NodePatch{Metadata: Metadata{"key": strings.Repeat("a", NodeMetadataValueMaxLength+1)}},
			wantError: "value of key: metadata is too large",
		},
		{
			// Half the cap in quotes doubles once escaped, so a raw byte count
			// would wave this through at twice what the column stores.
			name:      "escaped_string_over_limit",
			patch:     NodePatch{Metadata: Metadata{"key": strings.Repeat(`"`, NodeMetadataValueMaxLength/2)}},
			wantError: "value of key: metadata is too large",
		},
		{
			// json.Marshal escapes "<" as \u003c: six stored bytes per character.
			name:      "html_escaped_string_over_limit",
			patch:     NodePatch{Metadata: Metadata{"key": strings.Repeat("<", NodeMetadataValueMaxLength/2)}},
			wantError: "value of key: metadata is too large",
		},
		{
			name:  "number_value",
			patch: NodePatch{Metadata: Metadata{"port": 8080}},
		},
		{
			name:  "bool_value",
			patch: NodePatch{Metadata: Metadata{"tls": true}},
		},
		{
			name:  "null_value",
			patch: NodePatch{Metadata: Metadata{"note": nil}},
		},
		{
			name:      "nested_document_over_limit",
			patch:     NodePatch{Metadata: Metadata{"tags": []any{strings.Repeat("b", NodeMetadataValueMaxLength)}}},
			wantError: "value of tags: metadata is too large",
		},
		{
			name:      "empty_key",
			patch:     NodePatch{Metadata: Metadata{"  ": "value"}},
			wantError: "metadata key must not be empty",
		},
		{
			name:      "key_over_limit",
			patch:     NodePatch{Metadata: Metadata{strings.Repeat("k", NodeMetadataKeyMaxLength+1): "value"}},
			wantError: "metadata is too large",
		},
		{
			name:      "too_many_keys",
			patch:     NodePatch{Metadata: metadataWithKeys(NodeMetadataMaxKeys + 1)},
			wantError: "metadata is too large",
		},
		{
			name:      "empty_remove_key",
			patch:     NodePatch{RemoveMetadataKeys: []string{""}},
			wantError: "metadata key must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.patch.Validate()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestNodePatch_ApplyToMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		stored       Metadata
		patch        NodePatch
		wantMetadata Metadata
		// wantKeyCount replaces wantMetadata when the expected bag is too large
		// to spell out.
		wantKeyCount int
		wantError    string
	}{
		{
			name:         "merge_keeps_unlisted_keys",
			stored:       Metadata{"region": "fsn1", "port": 8080},
			patch:        NodePatch{Metadata: Metadata{"region": "hel1"}},
			wantMetadata: Metadata{"region": "hel1", "port": 8080},
		},
		{
			name:         "removal_applied_after_merge",
			stored:       Metadata{"region": "fsn1", "stale": "yes"},
			patch:        NodePatch{Metadata: Metadata{"fresh": "1"}, RemoveMetadataKeys: []string{"stale"}},
			wantMetadata: Metadata{"region": "fsn1", "fresh": "1"},
		},
		{
			name:         "emptied_bag_becomes_nil",
			stored:       Metadata{"only": "one"},
			patch:        NodePatch{RemoveMetadataKeys: []string{"only"}},
			wantMetadata: nil,
		},
		{
			name:         "untouched_when_patch_has_no_metadata",
			stored:       Metadata{"region": "fsn1"},
			patch:        NodePatch{Name: new("renamed")},
			wantMetadata: Metadata{"region": "fsn1"},
		},
		{
			name:      "merged_bag_over_key_limit",
			stored:    metadataWithKeys(NodeMetadataMaxKeys),
			patch:     NodePatch{Metadata: Metadata{"one-key-too-many": "1"}},
			wantError: "metadata is too large",
		},
		{
			name:         "merged_bag_at_key_limit",
			stored:       metadataWithKeys(NodeMetadataMaxKeys - 1),
			patch:        NodePatch{Metadata: Metadata{"last": "1"}},
			wantKeyCount: NodeMetadataMaxKeys,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			node := &Node{Metadata: tt.stored}

			err := tt.patch.ApplyTo(node)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Equal(t, tt.stored, node.Metadata, "a rejected patch must not touch the stored bag")

				return
			}

			require.NoError(t, err)

			if tt.wantKeyCount > 0 {
				assert.Len(t, node.Metadata, tt.wantKeyCount)

				return
			}

			assert.Equal(t, tt.wantMetadata, node.Metadata)
		})
	}
}

func metadataWithKeys(count int) Metadata {
	metadata := make(Metadata, count)
	for i := range count {
		metadata["key"+strconv.Itoa(i)] = "value"
	}

	return metadata
}

func TestNodePatch_Validate_Strings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		patch     NodePatch
		wantError error
	}{
		{
			name:  "absent_fields_are_left_alone",
			patch: NodePatch{},
		},
		{
			name: "all_fields_within_limits",
			patch: NodePatch{
				Name:         new("node-1"),
				Location:     new("Frankfurt"),
				Provider:     new("Hetzner"),
				WorkPath:     new("/srv/gameap"),
				SteamcmdPath: new("/srv/gameap/steamcmd"),
			},
		},

		{
			name:      "empty_name",
			patch:     NodePatch{Name: new("")},
			wantError: ErrNodeNameRequired,
		},
		{
			name:      "whitespace_only_name_counts_as_empty",
			patch:     NodePatch{Name: new("   \t\n")},
			wantError: ErrNodeNameRequired,
		},
		{
			name:  "name_at_the_limit",
			patch: NodePatch{Name: new(strings.Repeat("n", NodeNameMaxLength))},
		},
		{
			name:      "name_over_the_limit",
			patch:     NodePatch{Name: new(strings.Repeat("n", NodeNameMaxLength+1))},
			wantError: ErrNodeNameTooLong,
		},
		{
			// The name is trimmed before it is measured, so padding that pushes
			// the raw string over the limit is not a rejection.
			name:  "padded_name_is_trimmed_before_measuring",
			patch: NodePatch{Name: new("  " + strings.Repeat("n", NodeNameMaxLength) + "  ")},
		},

		{
			name:      "empty_location",
			patch:     NodePatch{Location: new("")},
			wantError: ErrNodeLocationRequired,
		},
		{
			name:      "whitespace_only_location_counts_as_empty",
			patch:     NodePatch{Location: new(" ")},
			wantError: ErrNodeLocationRequired,
		},
		{
			name:  "location_at_the_limit",
			patch: NodePatch{Location: new(strings.Repeat("l", NodeLocationMaxLength))},
		},
		{
			name:      "location_over_the_limit",
			patch:     NodePatch{Location: new(strings.Repeat("l", NodeLocationMaxLength+1))},
			wantError: ErrNodeLocationTooLong,
		},

		{
			name:  "empty_provider_is_allowed",
			patch: NodePatch{Provider: new("")},
		},
		{
			name:  "provider_at_the_limit",
			patch: NodePatch{Provider: new(strings.Repeat("p", NodeProviderMaxLength))},
		},
		{
			name:      "provider_over_the_limit",
			patch:     NodePatch{Provider: new(strings.Repeat("p", NodeProviderMaxLength+1))},
			wantError: ErrNodeProviderTooLong,
		},
		{
			// Unlike the name and the location, the provider is measured raw:
			// ApplyTo stores it as given, so the column width applies as given.
			name:      "provider_of_whitespace_over_the_limit",
			patch:     NodePatch{Provider: new(strings.Repeat(" ", NodeProviderMaxLength+1))},
			wantError: ErrNodeProviderTooLong,
		},

		{
			name:      "empty_work_path",
			patch:     NodePatch{WorkPath: new("")},
			wantError: ErrNodeWorkPathRequired,
		},
		{
			name:      "whitespace_only_work_path_counts_as_empty",
			patch:     NodePatch{WorkPath: new("  ")},
			wantError: ErrNodeWorkPathRequired,
		},
		{
			name:  "work_path_at_the_limit",
			patch: NodePatch{WorkPath: new("/" + strings.Repeat("w", NodePathMaxLength-1))},
		},
		{
			name:      "work_path_over_the_limit",
			patch:     NodePatch{WorkPath: new("/" + strings.Repeat("w", NodePathMaxLength))},
			wantError: ErrNodePathTooLong,
		},

		{
			name:  "empty_steamcmd_path_is_allowed",
			patch: NodePatch{SteamcmdPath: new("")},
		},
		{
			name:  "steamcmd_path_at_the_limit",
			patch: NodePatch{SteamcmdPath: new(strings.Repeat("s", NodePathMaxLength))},
		},
		{
			name:      "steamcmd_path_over_the_limit",
			patch:     NodePatch{SteamcmdPath: new(strings.Repeat("s", NodePathMaxLength+1))},
			wantError: ErrNodePathTooLong,
		},
		{
			// The steamcmd path is measured raw as well, so trailing padding is
			// not trimmed away before the length check.
			name:      "padded_steamcmd_path_is_measured_raw",
			patch:     NodePatch{SteamcmdPath: new(strings.Repeat("s", NodePathMaxLength) + " ")},
			wantError: ErrNodePathTooLong,
		},

		{
			// Validate reports the first problem, and the fields are checked in
			// declaration order.
			name: "name_problem_is_reported_before_a_location_problem",
			patch: NodePatch{
				Name:     new(""),
				Location: new(""),
			},
			wantError: ErrNodeNameRequired,
		},
		{
			name: "string_problem_is_reported_before_an_invalid_ip",
			patch: NodePatch{
				Name: new(""),
				IPs:  []string{"not a host!"},
			},
			wantError: ErrNodeNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			patch := tt.patch

			// ACT
			err := patch.Validate()

			// ASSERT
			if tt.wantError == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.wantError)
		})
	}
}
