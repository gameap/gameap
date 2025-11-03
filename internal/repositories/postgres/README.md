# PostgreSQL repositories

This directory contains the PostgreSQL implementations of the repositories for the GameAP API.

## Testing

To run the tests for the PostgreSQL repositories, use the following command.
Run a PostgreSQL container for testing:

```bash
docker run -it \
    --rm \
    -e POSTGRES_PASSWORD=testpassword \
    -e POSTGRES_DB=gameap \
    -p 5432:5432 \
    postgres:16
```

Then you need to set `TEST_POSTGRES_DSN` environment variable:
```
TEST_POSTGRES_DSN=postgres://postgres:testpassword@localhost:5432/gameap?sslmode=disable go test ./... -v
```

Or you can run only tests in this directory:
```
TEST_POSTGRES_DSN=postgres://postgres:testpassword@localhost:5432/gameap?sslmode=disable go test ./internal/repositories/postgres/ -v
```
