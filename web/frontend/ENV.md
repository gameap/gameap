# Environment Variables

## Configuration

The frontend application supports configuration via environment variables.

### Setup

1. Copy `.env.example` to `.env`:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` and set your values

### Available Variables

#### `VITE_API_BASE_URL`

Base URL for API requests.

- **Default**: Empty (uses relative URLs, same domain as frontend)
- **Example**: `VITE_API_BASE_URL=http://localhost:8080`
- **Use case**: When API runs on different domain/port than frontend

### Notes

- All environment variables must start with `VITE_` prefix to be exposed to the application
- Changes to `.env` require restart of development server (`npm run dev`)
- For production builds, set environment variables before running `npm run build`
- `.env` files are git-ignored and should not be committed to the repository
