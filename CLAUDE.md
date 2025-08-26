# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based web application for managing Google Business Profile reviews, featuring merchant dashboards and admin panels. The application uses Gin web framework with server-side rendered HTML templates and is heavily integrated with Supabase for database, authentication, and storage.

## Key Technologies

- **Backend**: Go with Gin web framework
- **Database**: PostgreSQL via Supabase
- **Authentication**: Supabase Auth with JWT tokens
- **Frontend**: Server-side rendered HTML templates with HTMX for interactivity
- **Container**: Docker with Air for hot reloading in development

## Essential Commands

### Development
```bash
# Run with hot reloading (requires Air)
air -c .air.toml

# Run directly
go run .

# Build the application
go build -o main .

# Run with Docker Compose (development)
docker-compose up

# Build Docker image for production
docker build -t auto-gbp-review --target production .
```

### Database Management
```bash
# Install Supabase CLI (if not installed)
npm i supabase --save-dev

# Initialize Supabase
npx supabase init

# Create new migration
npx supabase db diff --schema public

# Apply migrations to local database
npx supabase db push

# Reset local database
npx supabase db reset
```

### Dependencies
```bash
# Download dependencies
go mod download

# Update dependencies
go mod tidy

# Verify dependencies
go mod verify
```

## Architecture

### Core Components

1. **main.go**: Entry point, initializes router and database connection
2. **handlers.go**: Contains all HTTP handler functions for routes
3. **database.go**: Database connection and migration logic
4. **supabase_auth.go**: Supabase Auth integration for user authentication
5. **auth.go**: Legacy JWT token generation and validation
6. **unified_auth.go**: Dual-mode authentication middleware supporting both systems
7. **auth_config.go**: Authentication mode configuration (jwt/supabase/dual)
8. **storage.go**: Supabase Storage integration for file uploads
9. **supabase_client.go**: Supabase client initialization

### Route Structure

- `/` - Public home page
- `/merchant` - Public merchant page (with query param ?bn=businessname)
- `/login`, `/register` - Authentication pages
- `/admin/*` - Admin dashboard (protected, requires admin role)
- `/dashboard/*` - Merchant dashboard (protected, requires merchant role)
- `/api/*` - HTMX API endpoints

### Middleware

- `SupabaseAuthMiddleware`: Validates Supabase Auth tokens and enforces role-based access
- `SupabaseRedirectIfAuthenticated`: Redirects authenticated users from auth pages

### Database Schema

Located in `supabase/migrations/`. The application uses Supabase's migration system for schema management.

### Environment Variables

Required environment variables (see `.env.example`):
- `DATABASE_URL` - PostgreSQL connection string
- `SUPABASE_URL` - Supabase project URL
- `SUPABASE_ANON_KEY` - Supabase anonymous key
- `SUPABASE_SERVICE_ROLE_KEY` - Supabase service role key
- `JWT_SECRET` - Secret for JWT token signing
- `PORT` - Server port (default: 8082)

## Development Workflow

1. Environment setup: Copy `.env.example` to `.env` and configure
2. Database setup: Run Supabase locally or connect to cloud instance
3. Run application: Use `air` for hot reloading or `go run .`
4. Access at `http://localhost:8082`

## Testing

Currently no automated tests are configured. When implementing tests:
1. Create test files following Go convention (*_test.go)
2. Run tests with `go test ./...`
3. Generate coverage with `go test -cover ./...`

## Important Notes

- **Authentication**: Uses Supabase Auth exclusively for user management
- **Password Reset**: Built-in password reset functionality via email
- **HTMX Integration**: Frontend uses HTMX for dynamic updates without full page reloads
- Templates are in `templates/` with layouts, partials, and page-specific views
- Static assets are served from `static/` directory
- The project includes Docker support for both development and production deployments
- Database migrations should be managed through Supabase CLI, not the migrate() function in database.go
- User roles are stored in Supabase Auth user metadata