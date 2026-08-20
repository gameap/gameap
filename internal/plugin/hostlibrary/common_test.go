package hostlibrary

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUintsFromUint64s(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []uint64
		expected []uint
	}{
		{
			name:     "empty_slice",
			input:    []uint64{},
			expected: []uint{},
		},
		{
			name:     "single_element",
			input:    []uint64{42},
			expected: []uint{42},
		},
		{
			name:     "multiple_elements",
			input:    []uint64{1, 2, 3, 100, 999},
			expected: []uint{1, 2, 3, 100, 999},
		},
		{
			name:     "large_numbers",
			input:    []uint64{1000000, 2000000, 3000000},
			expected: []uint{1000000, 2000000, 3000000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := uintsFromUint64s(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUintPtrsFromUint64s(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []uint64
		expected []uint
	}{
		{
			name:     "empty_slice",
			input:    []uint64{},
			expected: []uint{},
		},
		{
			name:     "single_element",
			input:    []uint64{42},
			expected: []uint{42},
		},
		{
			name:     "multiple_elements",
			input:    []uint64{1, 2, 3},
			expected: []uint{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := uintPtrsFromUint64s(tt.input)
			require.Len(t, result, len(tt.expected))
			for i, ptr := range result {
				require.NotNil(t, ptr)
				assert.Equal(t, tt.expected[i], *ptr)
			}
		})
	}
}

func TestConvertSorting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []*common.Sorting
		expected []filters.Sorting
	}{
		{
			name:     "nil_input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty_slice",
			input:    []*common.Sorting{},
			expected: []filters.Sorting{},
		},
		{
			name: "ascending_sort",
			input: []*common.Sorting{
				{Field: "name", Descending: false},
			},
			expected: []filters.Sorting{
				{Field: "name", Direction: filters.SortDirectionAsc},
			},
		},
		{
			name: "descending_sort",
			input: []*common.Sorting{
				{Field: "id", Descending: true},
			},
			expected: []filters.Sorting{
				{Field: "id", Direction: filters.SortDirectionDesc},
			},
		},
		{
			name: "multiple_sorts",
			input: []*common.Sorting{
				{Field: "created_at", Descending: true},
				{Field: "name", Descending: false},
			},
			expected: []filters.Sorting{
				{Field: "created_at", Direction: filters.SortDirectionDesc},
				{Field: "name", Direction: filters.SortDirectionAsc},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertSorting(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUintPtrFromUint64Ptr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    *uint64
		expected *uint
	}{
		{
			name:     "nil_input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "zero_value",
			input:    new(uint64(0)),
			expected: new(uint(0)),
		},
		{
			name:     "positive_value",
			input:    new(uint64(42)),
			expected: new(uint(42)),
		},
		{
			name:     "large_value",
			input:    new(uint64(1000000)),
			expected: new(uint(1000000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := uintPtrFromUint64Ptr(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestUint64PtrFromUintPtr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    *uint
		expected *uint64
	}{
		{
			name:     "nil_input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "zero_value",
			input:    new(uint(0)),
			expected: new(uint64(0)),
		},
		{
			name:     "positive_value",
			input:    new(uint(42)),
			expected: new(uint64(42)),
		},
		{
			name:     "large_value",
			input:    new(uint(1000000)),
			expected: new(uint64(1000000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := uint64PtrFromUintPtr(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestEntityTypeFromProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    *proto.EntityType
		expected *string
	}{
		{
			name:     "nil_input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "unspecified_returns_nil",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_UNSPECIFIED),
			expected: nil,
		},
		{
			name:     "user_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_USER),
			expected: lo.ToPtr(string(domain.EntityTypeUser)),
		},
		{
			name:     "node_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_NODE),
			expected: lo.ToPtr(string(domain.EntityTypeNode)),
		},
		{
			name:     "game_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_GAME),
			expected: lo.ToPtr(string(domain.EntityTypeGame)),
		},
		{
			name:     "game_mod_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_GAME_MOD),
			expected: lo.ToPtr(string(domain.EntityTypeGameMod)),
		},
		{
			name:     "server_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_SERVER),
			expected: lo.ToPtr(string(domain.EntityTypeServer)),
		},
		{
			name:     "client_certificate_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_CLIENT_CERTIFICATE),
			expected: lo.ToPtr(string(domain.EntityTypeClientCertificate)),
		},
		{
			name:     "role_type",
			input:    lo.ToPtr(proto.EntityType_ENTITY_TYPE_ROLE),
			expected: lo.ToPtr(string(domain.EntityTypeRole)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := entityTypeFromProto(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestOptionalEntityTypeToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    *domain.EntityType
		expected *proto.EntityType
	}{
		{
			name:     "nil_input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty_type_returns_nil",
			input:    lo.ToPtr(domain.EntityTypeEmpty),
			expected: nil,
		},
		{
			name:     "unknown_type_returns_nil",
			input:    lo.ToPtr(domain.EntityType("unknown_type")),
			expected: nil,
		},
		{
			name:     "user_type",
			input:    lo.ToPtr(domain.EntityTypeUser),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_USER),
		},
		{
			name:     "server_type",
			input:    lo.ToPtr(domain.EntityTypeServer),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_SERVER),
		},
		{
			name:     "role_type",
			input:    lo.ToPtr(domain.EntityTypeRole),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_ROLE),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := optionalEntityTypeToProto(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestEntityTypeToProtoPtr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    *string
		expected *proto.EntityType
	}{
		{
			name:     "nil_input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty_string_returns_nil",
			input:    lo.ToPtr(string(domain.EntityTypeEmpty)),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_UNSPECIFIED),
		},
		{
			name:     "user_type",
			input:    lo.ToPtr(string(domain.EntityTypeUser)),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_USER),
		},
		{
			name:     "node_type",
			input:    lo.ToPtr(string(domain.EntityTypeNode)),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_NODE),
		},
		{
			name:     "game_type",
			input:    lo.ToPtr(string(domain.EntityTypeGame)),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_GAME),
		},
		{
			name:     "game_mod_type",
			input:    lo.ToPtr(string(domain.EntityTypeGameMod)),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_GAME_MOD),
		},
		{
			name:     "server_type",
			input:    lo.ToPtr(string(domain.EntityTypeServer)),
			expected: lo.ToPtr(proto.EntityType_ENTITY_TYPE_SERVER),
		},
		{
			name:     "unknown_type_returns_nil",
			input:    new("unknown_type"),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := entityTypeToProtoPtr(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}
