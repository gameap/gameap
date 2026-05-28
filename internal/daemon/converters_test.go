package daemon

import (
	"math"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtoCommandResultToResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *proto.CommandResult
		want *CommandResult
	}{
		{
			name: "maps_output_bytes_to_string_and_exit_code",
			in:   &proto.CommandResult{Output: []byte("hello"), ExitCode: 2},
			want: &CommandResult{Output: "hello", ExitCode: 2},
		},
		{
			name: "empty_output_and_zero_exit_code",
			in:   &proto.CommandResult{},
			want: &CommandResult{Output: "", ExitCode: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := protoCommandResultToResult(tt.in)

			require.NotNil(t, got)
			assert.Equal(t, tt.want.Output, got.Output, "output must map from bytes to string")
			assert.Equal(t, tt.want.ExitCode, got.ExitCode, "exit code must map from int32 to int")
		})
	}
}

func TestProtoStatusResponseToNodeStatus(t *testing.T) {
	t.Parallel()

	// ARRANGE
	in := &proto.StatusResponse{
		Version:       "1.0",
		BuildDate:     "2026-04-01",
		UptimeSeconds: 90,
		WorkingTasks:  2,
		WaitingTasks:  3,
		OnlineServers: 5,
	}

	// ACT
	got := protoStatusResponseToNodeStatus(in)

	// ASSERT
	require.NotNil(t, got)
	assert.Equal(t, 90*time.Second, got.Uptime, "uptime seconds must convert to duration")
	assert.Equal(t, "1.0", got.Version)
	assert.Equal(t, "2026-04-01", got.BuildDate)
	assert.Equal(t, 2, got.WorkingTasks)
	assert.Equal(t, 3, got.WaitingTasks)
	assert.Equal(t, 5, got.OnlineServers)
}

func TestProtoStatusResponseToVersion(t *testing.T) {
	t.Parallel()

	// ACT
	got := protoStatusResponseToVersion(&proto.StatusResponse{Version: "2.5.1", BuildDate: "2026-01-15"})

	// ASSERT
	require.NotNil(t, got)
	assert.Equal(t, "2.5.1", got.Version)
	assert.Equal(t, "2026-01-15", got.BuildDate)
}

func TestProtoFileTypeToFileType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   proto.FileType
		want FileType
	}{
		{name: "directory", in: proto.FileType_FILE_TYPE_DIRECTORY, want: FileTypeDir},
		{name: "regular", in: proto.FileType_FILE_TYPE_REGULAR, want: FileTypeFile},
		{name: "symlink", in: proto.FileType_FILE_TYPE_SYMLINK, want: FileTypeSymlink},
		{name: "socket", in: proto.FileType_FILE_TYPE_SOCKET, want: FileTypeSocket},
		{name: "fifo", in: proto.FileType_FILE_TYPE_FIFO, want: FileTypeNamedPipe},
		{name: "block_device", in: proto.FileType_FILE_TYPE_BLOCK_DEVICE, want: FileTypeBlockDevice},
		{name: "char_device", in: proto.FileType_FILE_TYPE_CHAR_DEVICE, want: FileTypeDevice},
		{name: "unspecified_maps_to_unknown", in: proto.FileType_FILE_TYPE_UNSPECIFIED, want: FileTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, protoFileTypeToFileType(tt.in), "proto file type must map to domain file type")
		})
	}
}

func TestProtoFileStatToFileInfo(t *testing.T) {
	t.Parallel()

	t.Run("returns_nil_for_nil_input", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, protoFileStatToFileInfo(nil), "nil FileStat must map to nil FileInfo")
	})

	t.Run("maps_all_fields", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		in := &proto.FileStat{
			Name:          "deploy.sh",
			Path:          "/srv/games/deploy.sh",
			Size:          1024,
			Mode:          0o755,
			Type:          proto.FileType_FILE_TYPE_REGULAR,
			ModifiedAt:    timestamppb.New(time.Unix(1700000000, 0)),
			SymlinkTarget: "../target",
		}

		// ACT
		got := protoFileStatToFileInfo(in)

		// ASSERT
		require.NotNil(t, got)
		assert.Equal(t, "deploy.sh", got.Name)
		assert.Equal(t, "/srv/games/deploy.sh", got.Path)
		assert.Equal(t, uint64(1024), got.Size)
		assert.Equal(t, uint32(0o755), got.Perm)
		assert.Equal(t, FileTypeFile, got.Type)
		assert.Equal(t, uint64(1700000000), got.TimeModified)
		assert.Equal(t, "../target", got.SymlinkTarget)
	})

	t.Run("nil_modified_at_yields_zero_time", func(t *testing.T) {
		t.Parallel()

		got := protoFileStatToFileInfo(&proto.FileStat{Name: "x", ModifiedAt: nil})

		require.NotNil(t, got)
		assert.Equal(t, uint64(0), got.TimeModified, "missing ModifiedAt must yield zero TimeModified")
	})
}

func TestProtoFileStatToDetails(t *testing.T) {
	t.Parallel()

	t.Run("returns_empty_details_for_nil_input", func(t *testing.T) {
		t.Parallel()

		got := protoFileStatToDetails(nil)

		require.NotNil(t, got, "nil FileStat must map to a non-nil empty FileDetails")
		assert.Equal(t, &FileDetails{}, got)
	})

	t.Run("maps_all_fields_including_access_time", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		in := &proto.FileStat{
			Name:          "config.yaml",
			Size:          256,
			Mode:          0o644,
			Type:          proto.FileType_FILE_TYPE_REGULAR,
			ModifiedAt:    timestamppb.New(time.Unix(1700000000, 0)),
			AccessedAt:    timestamppb.New(time.Unix(1700000500, 0)),
			SymlinkTarget: "",
		}

		// ACT
		got := protoFileStatToDetails(in)

		// ASSERT
		require.NotNil(t, got)
		assert.Equal(t, "config.yaml", got.Name)
		assert.Equal(t, uint64(256), got.Size)
		assert.Equal(t, uint32(0o644), got.Perm)
		assert.Equal(t, FileTypeFile, got.Type)
		assert.Equal(t, uint64(1700000000), got.ModificationTime)
		assert.Equal(t, uint64(1700000500), got.AccessTime)
	})
}

func TestSafeUint64ToInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   uint64
		want int64
	}{
		{name: "small_value_passes_through", in: 12345, want: 12345},
		{name: "zero", in: 0, want: 0},
		{name: "max_int64_boundary_passes_through", in: uint64(math.MaxInt64), want: math.MaxInt64},
		{name: "one_past_max_int64_clamps", in: uint64(math.MaxInt64) + 1, want: math.MaxInt64},
		{name: "max_uint64_clamps", in: math.MaxUint64, want: math.MaxInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, safeUint64ToInt64(tt.in), "overflow must clamp to math.MaxInt64")
		})
	}
}

func TestStripWorkPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		workPath string
		fullPath string
		want     string
	}{
		{
			name:     "strips_work_path_prefix",
			workPath: "/srv/games",
			fullPath: "/srv/games/configs/server.cfg",
			want:     "configs/server.cfg",
		},
		{
			name:     "identical_path_yields_dot",
			workPath: "/srv/games",
			fullPath: "/srv/games",
			want:     ".",
		},
		{
			name:     "trailing_slash_only_yields_dot",
			workPath: "/srv",
			fullPath: "/srv/",
			want:     ".",
		},
		{
			name:     "windows_backslashes_normalized_to_forward",
			workPath: `C:\gameap`,
			fullPath: `C:\gameap\saves\world.sav`,
			want:     "saves/world.sav",
		},
		{
			name:     "mixed_separators_normalized",
			workPath: "/srv",
			fullPath: `/srv\sub/file.txt`,
			want:     "sub/file.txt",
		},
		{
			name:     "surviving_drive_letter_is_stripped",
			workPath: "",
			fullPath: "C:/gameap/x.txt",
			want:     "gameap/x.txt",
		},
		{
			name:     "non_matching_prefix_still_trims_leading_slash",
			workPath: "/srv",
			fullPath: "/other/file.txt",
			want:     "other/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, stripWorkPath(tt.workPath, tt.fullPath), "stripped path mismatch")
		})
	}
}

func TestOwnerOptions_IsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		owner OwnerOptions
		want  bool
	}{
		{name: "fully_zero_is_zero", owner: OwnerOptions{}, want: true},
		{name: "user_set_is_not_zero", owner: OwnerOptions{User: "gameap"}, want: false},
		{name: "uid_set_is_not_zero", owner: OwnerOptions{UID: 1000}, want: false},
		{name: "gid_set_is_not_zero", owner: OwnerOptions{GID: 1000}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.owner.IsZero(), "IsZero result mismatch")
		})
	}
}

func TestOwnerFromServer(t *testing.T) {
	t.Parallel()

	t.Run("nil_server_yields_zero_owner", func(t *testing.T) {
		t.Parallel()

		assert.True(t, OwnerFromServer(nil).IsZero(), "nil server must yield zero owner")
	})

	t.Run("nil_su_user_yields_zero_owner", func(t *testing.T) {
		t.Parallel()

		assert.True(t, OwnerFromServer(&domain.Server{SuUser: nil}).IsZero(), "nil SuUser must yield zero owner")
	})

	t.Run("su_user_populates_user", func(t *testing.T) {
		t.Parallel()

		got := OwnerFromServer(&domain.Server{SuUser: new("gameap")})

		assert.Equal(t, "gameap", got.User, "SuUser must map to OwnerOptions.User")
		assert.Equal(t, int32(0), got.UID, "UID must remain zero when only SuUser is set")
		assert.Equal(t, int32(0), got.GID, "GID must remain zero when only SuUser is set")
	})
}

func TestApplyCommandOptions(t *testing.T) {
	t.Parallel()

	t.Run("defaults_work_dir_to_root_when_no_options", func(t *testing.T) {
		t.Parallel()

		got := applyCommandOptions(nil)

		assert.Equal(t, "/", got.WorkDir, "WorkDir must default to '/'")
	})

	t.Run("with_work_dir_option_overrides_default", func(t *testing.T) {
		t.Parallel()

		got := applyCommandOptions([]CommandServiceOption{CommandServiceOptionWithWorkDir("/srv/games")})

		assert.Equal(t, "/srv/games", got.WorkDir, "WorkDir option must override the default")
	})
}
