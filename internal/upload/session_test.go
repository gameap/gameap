package upload

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSession_ChunkSizeFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		totalSize uint64
		chunkSize uint64
		total     uint
		index     uint
		want      uint64
	}{
		{
			name:      "non_terminal_chunk_returns_full_chunk_size",
			totalSize: 10, chunkSize: 4, total: 3, index: 0, want: 4,
		},
		{
			name:      "middle_chunk_returns_full_chunk_size",
			totalSize: 10, chunkSize: 4, total: 3, index: 1, want: 4,
		},
		{
			name:      "last_chunk_returns_remainder",
			totalSize: 10, chunkSize: 4, total: 3, index: 2, want: 2,
		},
		{
			name:      "exactly_divisible_last_chunk_returns_full_chunk_size",
			totalSize: 12, chunkSize: 4, total: 3, index: 2, want: 4,
		},
		{
			name:      "single_chunk_session_returns_total_size",
			totalSize: 100, chunkSize: 8 << 20, total: 1, index: 0, want: 100,
		},
		{
			name:      "single_chunk_session_with_size_equal_to_chunk_size",
			totalSize: 4, chunkSize: 4, total: 1, index: 0, want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			sess := &Session{TotalSize: tt.totalSize, ChunkSize: tt.chunkSize, TotalChunks: tt.total}

			// ACT
			got := sess.ChunkSizeFor(tt.index)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPathHelpers(t *testing.T) {
	t.Parallel()
	const id = "abc123"

	t.Run("transferRoot_uses_transfers_prefix_and_trailing_slash", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "transfers/abc123/", transferRoot(id))
	})

	t.Run("metadataPath_lives_inside_transfer_root", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "transfers/abc123/upload.json", metadataPath(id))
	})

	t.Run("dataPath_lives_inside_transfer_root", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "transfers/abc123/data", dataPath(id))
	})

	t.Run("donePath_lives_inside_transfer_root", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "transfers/abc123/done", donePath(id))
	})

	t.Run("chunksPrefix_is_directory_with_trailing_slash", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "transfers/abc123/chunks/", chunksPrefix(id))
	})

	t.Run("chunkPath_pads_index_to_six_digits", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "transfers/abc123/chunks/000000", chunkPath(id, 0))
		assert.Equal(t, "transfers/abc123/chunks/000007", chunkPath(id, 7))
		assert.Equal(t, "transfers/abc123/chunks/123456", chunkPath(id, 123456))
		assert.Equal(t, "transfers/abc123/chunks/9999999", chunkPath(id, 9999999),
			"%06d does not truncate larger numbers; verifies width is a minimum, not maximum")
	})
}

func TestIndexFromChunkPath(t *testing.T) {
	t.Parallel()
	const id = "abc123"

	tests := []struct {
		name    string
		path    string
		want    uint
		wantOK  bool
		comment string
	}{
		{
			name:   "valid_padded_index",
			path:   "transfers/abc123/chunks/000007",
			want:   7,
			wantOK: true,
		},
		{
			name:    "valid_unpadded_index",
			path:    "transfers/abc123/chunks/12",
			want:    12,
			wantOK:  true,
			comment: "Sscanf with %06d still parses unpadded ints — width is a hint, not a constraint",
		},
		{
			name:   "valid_zero_index",
			path:   "transfers/abc123/chunks/000000",
			want:   0,
			wantOK: true,
		},
		{
			name:   "valid_local_bare_filename",
			path:   "000123",
			want:   123,
			wantOK: true,
		},
		{
			name:   "valid_s3_relative_key_with_trailing_slash",
			path:   "000321/",
			want:   321,
			wantOK: true,
		},
		{
			name:   "wrong_upload_id",
			path:   "transfers/zzz/chunks/000000",
			wantOK: false,
		},
		{
			name:   "missing_transfers_prefix",
			path:   "other/abc123/chunks/000000",
			wantOK: false,
		},
		{
			name:   "missing_chunks_segment",
			path:   "transfers/abc123/upload.json",
			wantOK: false,
		},
		{
			name:   "nested_path_under_chunks",
			path:   "transfers/abc123/chunks/foo/000000",
			wantOK: false,
		},
		{
			name:   "non_numeric_tail",
			path:   "transfers/abc123/chunks/notanumber",
			wantOK: false,
		},
		{
			name:   "empty_tail",
			path:   "transfers/abc123/chunks/",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got, ok := indexFromChunkPath(id, tt.path)

			// ASSERT
			assert.Equal(t, tt.wantOK, ok, tt.comment)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestUploadIDFromPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		path   string
		wantID string
		wantOK bool
	}{
		{
			name:   "metadata_path_yields_upload_id",
			path:   "transfers/abc/upload.json",
			wantID: "abc",
			wantOK: true,
		},
		{
			name:   "chunk_path_yields_upload_id",
			path:   "transfers/xyz/chunks/000000",
			wantID: "xyz",
			wantOK: true,
		},
		{
			name:   "local_bare_directory_name",
			path:   "abc",
			wantID: "abc",
			wantOK: true,
		},
		{
			name:   "s3_relative_key_with_trailing_slash",
			path:   "xyz/",
			wantID: "xyz",
			wantOK: true,
		},
		{
			name:   "missing_transfers_prefix",
			path:   "other/abc/upload.json",
			wantOK: false,
		},
		{
			name:   "no_slash_after_id",
			path:   "transfers/abc",
			wantOK: false,
		},
		{
			name:   "empty_id_segment",
			path:   "transfers//upload.json",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got, ok := uploadIDFromPath(tt.path)

			// ASSERT
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantID, got)
			}
		})
	}
}
