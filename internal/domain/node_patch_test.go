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
			name:  "string_value_at_limit",
			patch: NodePatch{Metadata: Metadata{"key": strings.Repeat("a", NodeMetadataValueMaxLength)}},
		},
		{
			name:      "string_value_over_limit",
			patch:     NodePatch{Metadata: Metadata{"key": strings.Repeat("a", NodeMetadataValueMaxLength+1)}},
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
