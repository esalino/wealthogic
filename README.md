# Wealthogic

A personal wealth and portfolio tracker for monitoring investments and financial positions.

## Getting Started

Install dependencies from the repo root:

```bash
pnpm install
```

Start all apps:

```bash
pnpm dev
```

This starts both apps concurrently:
- Web: http://localhost:5173
- API: http://localhost:8080

## Database

Wealthogic uses **PostgreSQL** as its database. You'll need a local Postgres instance running before starting the app.

Start Postgres locally (e.g. via Homebrew):

```bash
brew services start postgresql
```

Or with Docker:

```bash
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres
```

Configure your connection string in `apps/api/.env` before running the app.
