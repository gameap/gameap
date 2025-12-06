package files

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errInvalidScheme  = errors.New("invalid scheme: expected s3")
	errBucketRequired = errors.New("bucket is required in DSN path")
)

type s3Config struct {
	endpoint        string
	accessKeyID     string
	secretAccessKey string
	bucket          string
	useSSL          bool
}

func parseS3DSN(dsn string) (*s3Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}

	if u.Scheme != "s3" {
		return nil, errInvalidScheme
	}

	password, _ := u.User.Password()

	bucket := strings.TrimPrefix(u.Path, "/")
	if bucket == "" {
		return nil, errBucketRequired
	}

	useSSL := u.Query().Get("ssl") != "false"

	return &s3Config{
		endpoint:        u.Host,
		accessKeyID:     u.User.Username(),
		secretAccessKey: password,
		bucket:          bucket,
		useSSL:          useSSL,
	}, nil
}

func setupS3Test(t *testing.T) (*S3FileManager, string, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_S3_DSN")
	if dsn == "" {
		t.Skip("TEST_S3_DSN environment variable not set, skipping S3 tests")
	}

	cfg, err := parseS3DSN(dsn)
	require.NoError(t, err, "invalid TEST_S3_DSN")

	fm, err := NewS3FileManager(cfg.endpoint, cfg.accessKeyID, cfg.secretAccessKey, cfg.bucket, cfg.useSSL)
	require.NoError(t, err, "cannot create S3FileManager")

	testPrefix := fmt.Sprintf("test-%s/", uuid.New().String())

	cleanup := func() {
		ctx := context.Background()
		client, _ := minio.New(cfg.endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.accessKeyID, cfg.secretAccessKey, ""),
			Secure: cfg.useSSL,
		})

		objectCh := client.ListObjects(ctx, cfg.bucket, minio.ListObjectsOptions{
			Prefix:    testPrefix,
			Recursive: true,
		})

		for object := range objectCh {
			if object.Err == nil {
				_ = client.RemoveObject(ctx, cfg.bucket, object.Key, minio.RemoveObjectOptions{})
			}
		}
	}

	return fm, testPrefix, cleanup
}

func TestS3FileManager_Read(t *testing.T) {
	fm, prefix, cleanup := setupS3Test(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("read_existing_file", func(t *testing.T) {
		path := prefix + "read_test.txt"
		content := []byte("hello s3 world")
		err := fm.Write(ctx, path, content)
		require.NoError(t, err)

		data, err := fm.Read(ctx, path)

		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("read_non_existent_file", func(t *testing.T) {
		path := prefix + "nonexistent.txt"

		_, err := fm.Read(ctx, path)

		require.Error(t, err)
	})
}

func TestS3FileManager_Write(t *testing.T) {
	fm, prefix, cleanup := setupS3Test(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("write_new_file", func(t *testing.T) {
		path := prefix + "write_new.txt"
		content := []byte("new s3 content")

		err := fm.Write(ctx, path, content)

		require.NoError(t, err)
		data, err := fm.Read(ctx, path)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("overwrite_existing_file", func(t *testing.T) {
		path := prefix + "write_overwrite.txt"
		oldContent := []byte("old content")
		newContent := []byte("new content")

		err := fm.Write(ctx, path, oldContent)
		require.NoError(t, err)

		err = fm.Write(ctx, path, newContent)

		require.NoError(t, err)
		data, err := fm.Read(ctx, path)
		require.NoError(t, err)
		assert.Equal(t, newContent, data)
	})
}

func TestS3FileManager_Delete(t *testing.T) {
	fm, prefix, cleanup := setupS3Test(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("delete_existing_file", func(t *testing.T) {
		path := prefix + "delete_existing.txt"
		err := fm.Write(ctx, path, []byte("to delete"))
		require.NoError(t, err)

		err = fm.Delete(ctx, path)

		require.NoError(t, err)
		assert.False(t, fm.Exists(ctx, path))
	})

	t.Run("delete_non_existent_file", func(t *testing.T) {
		path := prefix + "delete_nonexistent.txt"

		err := fm.Delete(ctx, path)

		require.NoError(t, err)
	})
}

func TestS3FileManager_Exists(t *testing.T) {
	fm, prefix, cleanup := setupS3Test(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("file_exists", func(t *testing.T) {
		path := prefix + "exists.txt"
		err := fm.Write(ctx, path, []byte("content"))
		require.NoError(t, err)

		exists := fm.Exists(ctx, path)

		assert.True(t, exists)
	})

	t.Run("file_does_not_exist", func(t *testing.T) {
		path := prefix + "not_exists.txt"

		exists := fm.Exists(ctx, path)

		assert.False(t, exists)
	})
}

func TestS3FileManager_List(t *testing.T) {
	fm, prefix, cleanup := setupS3Test(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("list_files_in_directory", func(t *testing.T) {
		dir := prefix + "list_dir/"
		err := fm.Write(ctx, dir+"file1.txt", []byte("content1"))
		require.NoError(t, err)
		err = fm.Write(ctx, dir+"file2.txt", []byte("content2"))
		require.NoError(t, err)

		files, err := fm.List(ctx, dir)

		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Contains(t, files, "file1.txt")
		assert.Contains(t, files, "file2.txt")
	})

	t.Run("list_empty_directory", func(t *testing.T) {
		dir := prefix + "empty_dir/"

		files, err := fm.List(ctx, dir)

		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("list_with_prefix", func(t *testing.T) {
		dir := prefix + "prefix_dir/"
		err := fm.Write(ctx, dir+"a_file.txt", []byte("a"))
		require.NoError(t, err)
		err = fm.Write(ctx, dir+"b_file.txt", []byte("b"))
		require.NoError(t, err)
		err = fm.Write(ctx, prefix+"other_dir/c_file.txt", []byte("c"))
		require.NoError(t, err)

		files, err := fm.List(ctx, dir)

		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Contains(t, files, "a_file.txt")
		assert.Contains(t, files, "b_file.txt")
	})
}

func TestParseS3DSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		want    *s3Config
		wantErr bool
	}{
		{
			name: "full_dsn_with_ssl",
			dsn:  "s3://access-key:secret-key@s3.amazonaws.com/my-bucket?ssl=true",
			want: &s3Config{
				endpoint:        "s3.amazonaws.com",
				accessKeyID:     "access-key",
				secretAccessKey: "secret-key",
				bucket:          "my-bucket",
				useSSL:          true,
			},
			wantErr: false,
		},
		{
			name: "dsn_without_ssl_defaults_to_true",
			dsn:  "s3://user:pass@localhost:9000/bucket",
			want: &s3Config{
				endpoint:        "localhost:9000",
				accessKeyID:     "user",
				secretAccessKey: "pass",
				bucket:          "bucket",
				useSSL:          true,
			},
			wantErr: false,
		},
		{
			name: "dsn_with_ssl_false",
			dsn:  "s3://user:pass@localhost:9000/bucket?ssl=false",
			want: &s3Config{
				endpoint:        "localhost:9000",
				accessKeyID:     "user",
				secretAccessKey: "pass",
				bucket:          "bucket",
				useSSL:          false,
			},
			wantErr: false,
		},
		{
			name:    "invalid_scheme",
			dsn:     "http://user:pass@localhost:9000/bucket",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing_bucket",
			dsn:     "s3://user:pass@localhost:9000",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseS3DSN(tt.dsn)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want.endpoint, cfg.endpoint)
				assert.Equal(t, tt.want.accessKeyID, cfg.accessKeyID)
				assert.Equal(t, tt.want.secretAccessKey, cfg.secretAccessKey)
				assert.Equal(t, tt.want.bucket, cfg.bucket)
				assert.Equal(t, tt.want.useSSL, cfg.useSSL)
			}
		})
	}
}
