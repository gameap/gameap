# MySQL repositories

This directory contains the MySQL implementations of the repositories for the GameAP API.

## Testing

To run the tests for the MySQL repositories, use the following command.

Run a MariaDB container for testing:

```bash
docker run -it \
    --rm \
    -e MARIADB_ROOT_PASSWORD=testpassword \
    -e MARIADB_DATABASE=gameap \
    -p 3306:3306 \
    mariadb:10.11.14
```

Then you need to set `TEST_MYSQL_DSN` environment variable:
```
TEST_MYSQL_DSN=root:testpassword@tcp(localhost:3306)/gameap?parseTime=true go test ./... -v
```

Or you can run only tests in this directory:
```
TEST_MYSQL_DSN=root:testpassword@tcp(localhost:3306)/gameap?parseTime=true go test ./internal/repositories/mysql/ -v
```