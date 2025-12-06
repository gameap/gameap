# Files Package

File storage abstraction layer with pluggable implementations.

## Implementations

| Type | Description |
|------|-------------|
| `LocalFileManager` | Local filesystem storage using sandboxed `os.Root` |
| `S3FileManager` | S3/MinIO compatible object storage |
| `InMemoryFileManager` | In-memory storage for testing |
| `MockFileManager` | Manual mock for unit tests |

## Interface

```go
type FileManager interface {
    Read(ctx context.Context, path string) ([]byte, error)
    Write(ctx context.Context, path string, data []byte) error
    Delete(ctx context.Context, path string) error
    Exists(ctx context.Context, path string) bool
    List(ctx context.Context, dir string) ([]string, error)
}
```

## Testing

Run tests:
```bash
go test ./internal/files/...
```

### S3 Integration Tests

S3 tests require `TEST_S3_DSN` environment variable. Tests are skipped if not set.

**DSN format:**
```
s3://ACCESS_KEY:SECRET_KEY@ENDPOINT/BUCKET?ssl=true|false
```

**Examples:**
```bash
# AWS S3
TEST_S3_DSN="s3://AKIAIOSFODNN7EXAMPLE:wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY@s3.us-east-1.amazonaws.com/my-bucket?ssl=true"

# MinIO (local)
TEST_S3_DSN="s3://minioadmin:minioadmin@localhost:9000/test-bucket?ssl=false"

# Run with S3 tests
TEST_S3_DSN="s3://user:pass@localhost:9000/bucket?ssl=false" go test ./internal/files/...
```
