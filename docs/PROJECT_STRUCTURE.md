# GameAP API - Project Structure and Best Practices

## Overview

GameAP is a game server management panel written in Go. It follows clean architecture principles with a layered design supporting multiple databases (PostgreSQL, MySQL, SQLite), cache backends (Redis, in-memory), and file storage options (local, S3).

---

## Project Structure

```
gameap-api/
├── cmd/gameap/              # Application entry point
│   └── main.go              # Main function, loads env, calls application.Run()
├── internal/                # Private application code
│   ├── domain/              # Domain models (business entities)
│   ├── api/                 # HTTP handlers organized by resource
│   ├── application/         # Dependency injection container, bootstrap
│   ├── repositories/        # Data access layer with multi-DB support
│   ├── services/            # Business logic layer
│   ├── cache/               # Multi-driver cache abstraction
│   ├── files/               # File storage abstraction
│   ├── rbac/                # Role-based access control
│   ├── plugin/              # WebAssembly plugin system
│   ├── config/              # Environment configuration parsing
│   ├── certificates/        # TLS certificate management
│   ├── daemon/              # GameAP Daemon communication
│   ├── filters/             # Query filtering/sorting/pagination
│   └── i18n/                # Internationalization
├── pkg/                     # Reusable public packages
│   ├── api/                 # HTTP utilities (readers, responders, errors)
│   ├── auth/                # Authentication (PASETO, JWT, password hashing)
│   ├── plugin/              # WebAssembly plugin SDK
│   ├── proto/               # Protocol buffers
│   ├── quercon/             # Game query protocol library
│   ├── validation/          # Input validation utilities
│   ├── flexible/            # Flexible type handling
│   ├── strings/             # String utilities
│   ├── carbon/              # Time/date utilities
│   └── testcontainer/       # Docker testing utilities
├── openapi/                 # OpenAPI 3.1 specification
│   ├── openapi.yaml         # Main entry point
│   ├── paths/               # Endpoint definitions
│   ├── schemas/             # Data models
│   ├── parameters/          # Reusable parameters
│   └── security/            # Auth schemes
├── migrations/              # Database migrations (Goose)
│   ├── mysql/
│   ├── postgres/
│   └── sqlite/
├── web/                     # Frontend
│   ├── frontend/            # Vue 3 + Vite SPA
│   └── static/              # Static assets
├── test/                    # Test utilities and fixtures
├── certs/                   # SSL/TLS certificates
├── Makefile                 # Build commands
├── Dockerfile               # Multi-stage Docker build
├── docker-compose.yml       # Development environment
├── .golangci.yaml           # Linter configuration
└── go.mod                   # Go module definition
```

---

## Package Descriptions

### `internal/domain/`

Core domain models representing business entities:

| Model | Description |
|-------|-------------|
| `User` | System users for authentication and access control |
| `Server` | Game server instances with configuration and lifecycle |
| `Node` | Physical/virtual machines hosting game servers |
| `Game` | Base game definitions with engine info |
| `GameMod` | Game modifications with RCON commands |
| `Auth` | Personal access tokens for API authentication |
| `RBAC` | Role-based access control (roles, permissions, abilities) |
| `ClientCertificate` | TLS certificates for daemon communication |
| `DaemonTask` | Low-level daemon tasks |
| `ServerTask` | Scheduled server tasks |
| `ServerSetting` | Key-value server configuration |
| `Plugin` | WebAssembly plugin metadata |
| `PluginStorage` | Plugin persistent key-value storage |

### `internal/api/`

HTTP handlers organized by resource, each endpoint in its own directory:

```
api/
├── auth/login/          # Authentication
├── servers/             # Server CRUD
│   ├── getconsole/
│   ├── getquery/
│   ├── postcommand/
│   └── rcon/
├── filemanager/         # File operations
├── nodes/               # Node management
├── games/               # Game CRUD
├── gamemods/            # Game mod management
├── users/               # User management
├── daemonapi/           # Internal daemon API
├── plugins/             # Plugin management
└── gethealth/           # Health check
```

Each handler directory contains:
- `handler.go` - HTTP handler logic
- `handler_test.go` - Unit tests
- `input.go` - Request validation
- `response.go` - Response formatting

### `internal/repositories/`

Data access layer with interface-based abstraction:

```
repositories/
├── contracts.go         # Repository interfaces
├── base/                # Shared types and base implementations
├── mysql/               # MySQL implementation
├── postgres/            # PostgreSQL implementation
├── sqlite/              # SQLite implementation
├── inmemory/            # In-memory implementation (testing)
├── cached/              # Caching decorator
└── testing/             # Test utilities
```

### `internal/cache/`

Multi-driver cache with performance benchmarks:

| Driver | Get Performance |
|--------|----------------|
| In-memory | ~57 ns/op |
| Redis | ~133 μs/op |
| PostgreSQL | ~147 μs/op |
| MySQL | ~297 μs/op |

### `internal/files/`

File storage abstraction:

| Implementation | Description |
|---------------|-------------|
| `LocalFileManager` | Sandboxed local filesystem |
| `S3FileManager` | S3/MinIO compatible storage |
| `InMemoryFileManager` | Testing implementation |
| `MockFileManager` | Unit test mocks |

Interface:
```go
type FileManager interface {
    Read(ctx context.Context, path string) ([]byte, error)
    Write(ctx context.Context, path string, data []byte) error
    Delete(ctx context.Context, path string) error
    Exists(ctx context.Context, path string) bool
    List(ctx context.Context, dir string) ([]string, error)
}
```

---

## Architectural Patterns

### Clean Architecture / Hexagonal Design

```
HTTP Layer (internal/api/*)
     ↓
Business Logic (internal/services/*)
     ↓
Domain Models (internal/domain/*)
     ↓
Data Access (internal/repositories/*)
     ↓
Databases (MySQL, PostgreSQL, SQLite)
```

### Dependency Injection Container

Centralized container manages all dependencies with lazy initialization:

```go
type Container struct {
    gameRepository repositories.GameRepository
    // ...
}

func (c *Container) GameRepository() repositories.GameRepository {
    if c.gameRepository == nil {
        c.gameRepository = c.createGameRepository()
    }
    return c.gameRepository
}
```

### Handler Constructor Injection

All handlers receive dependencies via constructor:

```go
type Handler struct {
    userRepo  repositories.UserRepository
    responder base.Responder
}

func NewHandler(userRepo repositories.UserRepository, responder base.Responder) *Handler {
    return &Handler{userRepo: userRepo, responder: responder}
}
```

### Repository Pattern with Multi-DB Support

Same interface implemented for all databases:

```go
type GameRepository interface {
    FindAll(ctx context.Context, order []filters.Sorting, pagination *filters.Pagination) ([]domain.Game, error)
    Find(ctx context.Context, filter *filters.FindGame, ...) ([]domain.Game, error)
    Save(ctx context.Context, game *domain.Game) error
    Delete(ctx context.Context, code string) error
}
```

### Decorator Pattern for Caching

Repositories wrapped with caching when Redis is configured:

```go
if c.config.Cache.Driver == cacheDriverRedis {
    return cached.NewGameRepository(baseRepo, c.Cache(), ttl)
}
```

---

## Coding Conventions

### Error Handling

- Use `errors.Wrap()` for external package errors
- Use `errors.WithMessage()` for internal errors
- Never use `fmt.Errorf` for wrapping

```go
// External error
result, err := externalPkg.DoSomething()
if err != nil {
    return errors.Wrap(err, "failed to do something")
}

// Internal error
result, err := c.userRepo.Find(ctx, filter)
if err != nil {
    return errors.WithMessage(err, "failed to find user")
}
```

### Input Reading

Use `pkg/api/reader.go` instead of direct request reading:

```go
// Good
reader := api.NewInputReader(r)
id, err := reader.ReadUint("id")

// Avoid
vars := mux.Vars(r)
id := vars["id"]
```

### Pointer Utilities

Use `samber/lo` for pointer conversions:

```go
ptr := lo.ToPtr(value)
val := lo.FromPtr(ptr)
```

### Comments

Avoid obvious comments:

```go
// Bad - obvious comment
// Open file
file, err := os.Open("file.txt")

// Good - no comment needed for obvious code
file, err := os.Open("file.txt")
```

### Filtering and Pagination

Consistent filter objects for each entity:

```go
type FindUser struct {
    IDs    []uint
    Logins []string
    Emails []string
}

type Sorting struct {
    Field     string
    Direction string // "asc" or "desc"
}

type Pagination struct {
    Limit  int
    Offset int
}
```

---

## Testing Best Practices

### Table-Driven Tests

```go
tests := []struct {
    name           string
    setupRepo      func(*inmemory.UserRepository)
    requestBody    string
    expectedStatus int
    wantError      string  // Use string, not bool
    checkResponse  func(*testing.T, map[string]any)
}{
    {
        name: "valid_user_creation",  // Use underscores, not spaces
        // ...
    },
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // ARRANGE
        repo := inmemory.NewUserRepository()
        if tt.setupRepo != nil {
            tt.setupRepo(repo)
        }
        // ACT
        // ASSERT
    })
}
```

### Naming Conventions

- Use underscores instead of spaces in test names: `valid_user_creation`
- Avoid "fail" in test names (reserved for actual failures)
- Use `wantError string` instead of `wantErr bool`

### Assertions

```go
// Use require.Len to prevent index out of range panics
require.Len(t, results, 3)

// Use assert.Contains for error messages
if tt.wantError != "" {
    assert.Contains(t, err.Error(), tt.wantError)
}
```

### Repository Testing

Use `setupRepo` pattern:

```go
{
    name: "find_by_id",
    setupRepo: func(repo *inmemory.UserRepository) {
        repo.Save(ctx, &domain.User{ID: 1, Login: "test"})
    },
    // ...
}
```

### Database Testing

```bash
# MySQL
TEST_MYSQL_DSN=root:password@tcp(localhost:3306)/gameap?parseTime=true go test ./...

# PostgreSQL
TEST_POSTGRES_DSN=postgres://user:pass@localhost:5432/gameap go test ./...

# S3
TEST_S3_DSN=s3://access:secret@localhost:9000/bucket?ssl=false go test ./...
```

---

## API Development Workflow

1. **Update OpenAPI spec first** (`openapi/`)
2. Implement handler in `internal/api/`
3. Write tests with table-driven approach
4. Run linter: `golangci-lint run ./...`

---

## Build and Development

### Makefile Commands

```bash
make lint          # Run linter
make lint-fix      # Fix linting issues
```

### Docker

```bash
docker-compose up -d    # Start development environment
docker build -t gameap . # Build production image
```

### Environment Variables

Key configuration:

| Variable | Description |
|----------|-------------|
| `DATABASE_DRIVER` | mysql, postgres, sqlite, inmemory |
| `DATABASE_URL` | Connection string |
| `DATABASE_CONNECT_TIMEOUT` | Startup connect retry window; default `30s` |
| `CACHE_DRIVER` | memory, redis |
| `FILES_DRIVER` | local, s3 |
| `AUTH_SERVICE` | paseto (default) |
| `AUTH_ALLOW_WEAK_PASSWORDS` | Disable common-password blocklist; default `false` (logs warning when enabled) |
| `ENCRYPTION_KEY` | Required for production |

---

## Key Dependencies

| Library | Purpose |
|---------|---------|
| `gorilla/mux` | HTTP routing |
| `pgx/v5` | PostgreSQL driver |
| `go-sql-driver/mysql` | MySQL driver |
| `modernc.org/sqlite` | SQLite driver |
| `squirrel` | SQL query builder |
| `go-paseto` | PASETO tokens |
| `wazero` | WebAssembly runtime |
| `samber/lo` | Functional utilities |
| `testify` | Testing assertions |
| `golangci-lint` | Code quality |

---

## CI/CD

### GitHub Actions Workflows

1. **Test** - Runs on all PRs and pushes
   - Linting with golangci-lint
   - Unit tests with coverage
   - Integration tests (MySQL, PostgreSQL, Redis, MinIO)

2. **Release** - Builds binaries for all platforms
   - Linux, Windows, macOS
   - amd64, arm64, 386

3. **Docker** - Builds multi-platform images
   - linux/amd64, linux/arm64
   - Pushed to Docker Hub
