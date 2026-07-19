# GameAP Docker Deployment Guide

This guide explains how to deploy GameAP using Docker and Docker Compose.

## Quick Start

### Using Docker Compose (Recommended)

The simplest way to get started with GameAP is using Docker Compose:

```bash
# Clone the repository
git clone https://github.com/gameap/gameap.git
cd gameap

# Start GameAP with PostgreSQL and Redis
docker-compose up -d

# Access GameAP at http://localhost:8025
```

## Docker Compose Configurations

We provide multiple Docker Compose configurations for different use cases. Copy the appropriate configuration below and save it as `docker-compose.yml` in your project directory.

### 1. Default (PostgreSQL + Redis)

Full production setup with PostgreSQL database and Redis cache:

```bash
docker-compose up -d
```

**Services:**
- GameAP API (port 8025)
- PostgreSQL 17
- Redis 7

**Environment Variables:**
- `ENCRYPTION_KEY` - Encryption key for sensitive data (change in production!)
- `AUTH_SECRET` - Secret for JWT/PASETO tokens (change in production!)
- `POSTGRES_PASSWORD` - PostgreSQL password (default: gameap)
- `LOGGER_LEVEL` - Log level: debug, info, error (default: info)

### 2. MySQL Variant

For users who prefer MySQL/MariaDB:

```bash
docker-compose -f docker-compose.mysql.yml up -d
```

**Services:**
- GameAP API (port 8025)
- MySQL 8.4
- Redis 7

**Additional Environment Variables:**
- `MYSQL_PASSWORD` - MySQL user password (default: gameap)
- `MYSQL_ROOT_PASSWORD` - MySQL root password (default: rootpassword)

**docker-compose.mysql.yml:**

```yaml
services:
  gameap:
    image: gameap/gameap:latest
    build:
      context: .
      dockerfile: Dockerfile
      args:
        VERSION: dev
        BUILD_DATE: ${BUILD_DATE:-2025-01-01T00:00:00Z}
    container_name: gameap
    ports:
      - "8025:8025"
    environment:
      HTTP_HOST: 0.0.0.0
      HTTP_PORT: 8025
      DATABASE_DRIVER: mysql
      DATABASE_URL: gameap:gameap@tcp(mysql:3306)/gameap?parseTime=true
      ENCRYPTION_KEY: ${ENCRYPTION_KEY:-change-me-in-production}
      AUTH_SECRET: ${AUTH_SECRET:-change-me-in-production}
      CACHE_DRIVER: redis
      CACHE_REDIS_ADDR: redis:6379
      FILES_DRIVER: local
      FILES_LOCAL_BASE_PATH: /var/lib/gameap/files
      LOGGER_LEVEL: ${LOGGER_LEVEL:-info}
    volumes:
      - gameap-storage:/var/lib/gameap
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
    networks:
      - gameap-network
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8025/api/health"]
      interval: 30s
      timeout: 3s
      start_period: 10s
      retries: 3

  mysql:
    image: mysql:8.4
    container_name: gameap-mysql
    environment:
      MYSQL_DATABASE: gameap
      MYSQL_USER: gameap
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-gameap}
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:-rootpassword}
    volumes:
      - mysql-data:/var/lib/mysql
    networks:
      - gameap-network
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost", "-u", "root", "-p${MYSQL_ROOT_PASSWORD:-rootpassword}"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: gameap-redis
    command: redis-server --appendonly yes
    volumes:
      - redis-data:/data
    networks:
      - gameap-network
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  gameap-storage:
    driver: local
  mysql-data:
    driver: local
  redis-data:
    driver: local

networks:
  gameap-network:
    driver: bridge
```

### 3. Development (PostgreSQL + MySQL)

For development only with both PostgreSQL and MySQL databases available:

**WARNING: This configuration is for development only. Do not use in production!**

**Services:**
- PostgreSQL 17 (port 25432)
- MySQL 8.1 (port 23306)

**docker-compose.dev.yml:**

```yaml
# This is for development only. Do not use in production.

services:
  postgres:
    image: postgres:17.2
    ports:
        - "25432:5432"
    environment:
        POSTGRES_USER: gameap
        POSTGRES_PASSWORD: gameap
        POSTGRES_DB: gameap

  mysql:
    image: mysql:8.1
    ports:
        - "23306:3306"
    environment:
        MYSQL_ROOT_PASSWORD: mysql
        MYSQL_DATABASE: gameap
        MYSQL_USER: gameap
        MYSQL_PASSWORD: gameap
```

### 4. Simple (SQLite)

Minimal setup for development/testing with SQLite and in-memory cache:

**Services:**
- GameAP API only (port 8025)
- No external dependencies

**docker-compose.simple.yml:**

```yaml
services:
  gameap:
    image: gameap/gameap:latest
    build:
      context: .
      dockerfile: Dockerfile
      args:
        VERSION: dev
        BUILD_DATE: ${BUILD_DATE:-2025-01-01T00:00:00Z}
    container_name: gameap
    ports:
      - "8025:8025"
    environment:
      HTTP_HOST: 0.0.0.0
      HTTP_PORT: 8025
      DATABASE_DRIVER: sqlite
      DATABASE_URL: file:/var/lib/gameap/db.sqlite?_busy_timeout=5000&_journal_mode=WAL&cache=shared
      ENCRYPTION_KEY: ${ENCRYPTION_KEY:-change-me-in-production}
      AUTH_SECRET: ${AUTH_SECRET:-change-me-in-production}
      CACHE_DRIVER: memory
      FILES_DRIVER: local
      FILES_LOCAL_BASE_PATH: /var/lib/gameap/files
      LOGGER_LEVEL: ${LOGGER_LEVEL:-debug}
    volumes:
      - gameap-storage:/var/lib/gameap
    networks:
      - gameap-network
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8025/api/health"]
      interval: 30s
      timeout: 3s
      start_period: 10s
      retries: 3

volumes:
  gameap-storage:
    driver: local

networks:
  gameap-network:
    driver: bridge
```

## Using Pre-built Docker Images

GameAP images are available on Docker Hub:

```bash
# Pull the latest image
docker pull gameap/gameap:latest

# Or pull a specific version
docker pull gameap/gameap:v1.0.0

# Run with minimal configuration
docker run -d \
  -p 8025:8025 \
  -e DATABASE_DRIVER=sqlite \
  -e DATABASE_URL=file:/var/lib/gameap/db.sqlite?_busy_timeout=5000&_journal_mode=WAL&cache=shared \
  -e ENCRYPTION_KEY=your-secret-key \
  -e AUTH_SECRET=your-auth-secret \
  -v gameap-data:/var/lib/gameap \
  gameap/gameap:latest
```

## Building Your Own Image

### Build Locally

```bash
# Build the image
docker build -t gameap/gameap:custom .

# With version information
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_DATE=$(date --iso-8601=seconds) \
  -t gameap/gameap:1.0.0 \
  .
```

### Multi-platform Build

```bash
# Set up buildx (one-time setup)
docker buildx create --name gameap-builder --use

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_DATE=$(date --iso-8601=seconds) \
  -t gameap/gameap:1.0.0 \
  --push \
  .
```

## Configuration

### Environment Variables

All configuration is done via environment variables:

#### Server Configuration
- `HTTP_HOST` - Host to bind to (default: `0.0.0.0`)
- `HTTP_PORT` - Port to listen on (default: `8025`)

#### Database Configuration
- `DATABASE_DRIVER` - Database type: `mysql`, `postgres`, `sqlite`, `inmemory`
- `DATABASE_URL` - Connection string:
  - PostgreSQL: `postgres://user:pass@host:5432/dbname?sslmode=disable`
  - MySQL: `user:pass@tcp(host:3306)/dbname?parseTime=true`
  - SQLite: `file:/path/to/db.sqlite?_busy_timeout=5000&_journal_mode=WAL&cache=shared` (parameters recommended for concurrent access)

#### Security
- `ENCRYPTION_KEY` - **Required** - Key for encrypting sensitive data
- `AUTH_SECRET` - **Required** - Secret for JWT/PASETO tokens
- `AUTH_SERVICE` - Authentication service (default: `paseto`)

#### Cache Configuration
- `CACHE_DRIVER` - Cache driver: `memory`, `redis`
- `CACHE_REDIS_ADDR` - Redis address (default: `localhost:6379`)

#### File Storage
- `FILES_DRIVER` - Storage driver: `local`, `s3`
- `FILES_LOCAL_BASE_PATH` - Base path for local files
- For S3:
  - `FILES_S3_ENDPOINT`
  - `FILES_S3_ACCESS_KEY_ID`
  - `FILES_S3_SECRET_ACCESS_KEY`
  - `FILES_S3_BUCKET`
  - `FILES_S3_REGION`

#### Other
- `LOGGER_LEVEL` - Log level: `debug`, `info`, `error` (default: `info`)
- `RBAC_CACHE_TTL` - RBAC cache TTL (default: `30s`)

### Example .env File

Create a `.env` file in the same directory as your `docker-compose.yml`:

```bash
# Security (CHANGE THESE IN PRODUCTION!)
ENCRYPTION_KEY=your-random-32-char-encryption-key-here
AUTH_SECRET=your-random-32-char-auth-secret-here

# Database
POSTGRES_PASSWORD=secure-database-password

# Logging
LOGGER_LEVEL=info

# Optional: External services
# FILES_DRIVER=s3
# FILES_S3_ENDPOINT=https://s3.amazonaws.com
# FILES_S3_BUCKET=gameap-files
```

## Production Deployment

### Security Checklist

- [ ] Change `ENCRYPTION_KEY` and `AUTH_SECRET` to strong random values
- [ ] Use strong database passwords
- [ ] Use HTTPS/TLS with a reverse proxy (nginx, Traefik, Caddy)
- [ ] Keep Docker images updated
- [ ] Regularly backup volumes
- [ ] Use secrets management for sensitive values
- [ ] Limit exposed ports (use reverse proxy instead of direct access)

### Using with Reverse Proxy

Example nginx configuration:

```nginx
server {
    listen 80;
    server_name gameap.example.com;

    # File manager uploads send chunks of FILES_UPLOAD_CHUNK_SIZE (8MB by default).
    # The nginx default client_max_body_size of 1m rejects them with 413.
    client_max_body_size 64m;

    location / {
        proxy_pass http://localhost:8025;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Backup and Restore

#### Backup

```bash
# Backup PostgreSQL database
docker exec gameap-postgres pg_dump -U gameap gameap > backup.sql

# Backup volumes
docker run --rm \
  -v gameap_gameap-storage:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/gameap-storage.tar.gz /data

# Backup MySQL database
docker exec gameap-mysql mysqldump -u gameap -p gameap > backup.sql
```

#### Restore

```bash
# Restore PostgreSQL database
docker exec -i gameap-postgres psql -U gameap gameap < backup.sql

# Restore volumes
docker run --rm \
  -v gameap_gameap-storage:/data \
  -v $(pwd):/backup \
  alpine sh -c "cd / && tar xzf /backup/gameap-storage.tar.gz"

# Restore MySQL database
docker exec -i gameap-mysql mysql -u gameap -p gameap < backup.sql
```

## Health Checks

The GameAP container includes a health check that queries `/api/health`:

```bash
# Check container health
docker inspect --format='{{.State.Health.Status}}' gameap

# View health check logs
docker inspect --format='{{json .State.Health}}' gameap | jq
```

## Logs

```bash
# View GameAP logs
docker logs gameap

# Follow logs in real-time
docker logs -f gameap

# View all services logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f gameap
docker-compose logs -f postgres
```

## Troubleshooting

### Container won't start

Check logs:
```bash
docker logs gameap
```

Common issues:
- Database connection errors: Verify `DATABASE_URL` and ensure database container is healthy
- Port conflicts: Ensure port 8025 is not already in use
- Missing secrets: Set `ENCRYPTION_KEY` and `AUTH_SECRET`

### Database connection errors

Verify database service is healthy:
```bash
docker-compose ps
```

Check database logs:
```bash
docker-compose logs postgres  # or mysql
```

### Permission errors

The container runs as user `gameap` (UID 1000). Ensure mounted volumes have correct permissions:
```bash
sudo chown -R 1000:1000 /path/to/mounted/volume
```

### Reset everything

```bash
# Stop and remove containers, volumes, and networks
docker-compose down -v

# Start fresh
docker-compose up -d
```

## CI/CD

GitHub Actions automatically builds and pushes Docker images:

- **On push to main**: Tagged as `latest`
- **On version tag** (e.g., `v1.0.0`): Tagged as version number
- **Multi-platform**: Built for `linux/amd64` and `linux/arm64`

To trigger a build, push to main or create a release tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Advanced Configuration

### Using External Database

Update `DATABASE_URL` to point to your external database:

```yaml
services:
  gameap:
    environment:
      DATABASE_DRIVER: postgres
      DATABASE_URL: postgres://user:pass@external-host:5432/gameap?sslmode=require
```

### Custom Network

```yaml
networks:
  gameap-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.25.0.0/16
```

### Resource Limits

```yaml
services:
  gameap:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

## Support

- Documentation: https://docs.gameap.com
- GitHub Issues: https://github.com/gameap/gameap/issues
- Website: https://gameap.com
