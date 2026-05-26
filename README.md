# Ytter

Go backend for **Appointa**: authentication, locations, services, slots, bookings, email, background jobs, and the AI gateway. Exposes **gRPC** (port 50051) and a **grpc-gateway** HTTP API (port 8080) consumed by [appointa-frontend](../book/appointa-frontend).

Related repositories:

| Repo | Role |
|------|------|
| **ytter** (this repo) | API, PostgreSQL schema, workers, protobuf contracts |
| **appointa-frontend** | Nuxt customer and host UI |
| **appointa-specs** | Product, API, and testing specs |

## Prerequisites

- Go 1.25+ (see `go.mod`)
- Docker (Postgres 18 and Redis 7 via `docker compose`)
- [golang-migrate](https://github.com/golang-migrate/migrate) for schema migrations
- [sqlc](https://sqlc.dev) for query code generation

Optional for day-to-day development: [Air](https://github.com/air-verse/air) (hot reload, used by `make dev`).

## Quick start (local)

1. Copy and edit `config/app.env` with your secrets (OAuth, email, OpenAI). **Do not commit real credentials.**

2. For running the API on your machine (not inside Docker), point `DB_URL` and `REDIS_ADDRESS` at localhost — the values in `config/apptesting.env` are a useful reference:

   ```
   DB_URL=postgresql://root:secret@localhost:5432/ytter?sslmode=disable
   REDIS_ADDRESS=localhost:6379
   ```

   When using `make up` (API in Docker), use the `postgres` and `redis` service hostnames instead.

3. Start dependencies and the API with hot reload:

   ```bash
   make dev
   ```

   This runs `docker compose up -d postgres redis`, applies migrations on startup, and starts Air.

   HTTP API: [http://localhost:8080](http://localhost:8080)  
   Swagger UI is served from the embedded statik bundle (generated with `make proto`).

4. Run [appointa-frontend](../book/appointa-frontend) with `NUXT_PUBLIC_API_BASE=http://localhost:8080`.

### Full stack in Docker

```bash
make up      # postgres, redis, api container
make down
```

## Configuration

Settings load from `config/app.env` (see `pkg/config/config.go`). Important variables:

| Variable | Purpose |
|----------|---------|
| `ENVIRONMENT` | `development` or `production` |
| `DB_URL` | PostgreSQL connection string |
| `MIGRATION_URL` | Usually `file://db/migration` |
| `HTTP_SERVER_ADDRESS` | REST/gateway listen address (default `0.0.0.0:8080`) |
| `GRPC_SERVER_ADDRESS` | gRPC listen address (default `0.0.0.0:50051`) |
| `REDIS_ADDRESS` | Asynq task queue |
| `TOKEN_SYMMETRIC_KEY` | 32-byte PASETO signing key |
| `FRONTEND_BASE_URL` | Used in email links and OAuth redirects |
| `ALLOWED_ORIGINS` | CORS origins for the Nuxt app |
| `GOOGLE_*` | Google OAuth client credentials |
| `EMAIL_SENDER_*` | SMTP sender for transactional mail |
| `OPENAI_*` / `AI_*` | AI assistant and slot-preview gateway |

`make test` exports variables from `config/apptesting.env` for the test database URL.

## Make targets

| Target | Description |
|--------|-------------|
| `make dev` | Postgres + Redis in Docker, API with Air hot reload |
| `make up` / `make down` | Full Docker Compose stack |
| `make server` | Run API without Air (`go run main.go`) |
| `make migrateup` / `make migratedown` | Apply or roll back migrations |
| `make migrateup1` / `make migratedown1` | Single-step migration |
| `make new_migration name=<name>` | Create a new migration pair |
| `make sqlc` | Regenerate `db/sqlc` from `db/query/*.sql` |
| `make proto` | Regenerate `pb/` and Swagger from `proto/*.proto` |
| `make mock` | Regenerate test mocks |
| `make test` | Unit tests with coverage (short mode) |
| `make test_all` | All tests without `-short` |
| `make db_schema` | Export `docs/schema.sql` from `docs/db.dbml` |
| `make db_docs` | Publish DBML docs via dbdocs |

## Project layout

```
api/              # gRPC/HTTP handlers, middleware, route RBAC
proto/            # Protobuf service and RPC definitions
pb/               # Generated Go and grpc-gateway code
db/
  migration/      # SQL migrations (golang-migrate)
  query/          # sqlc query files
  sqlc/           # Generated store and models
internal/
  worker/         # Asynq email and background tasks
  email/          # SMTP sender
  ai/             # OpenAI gateway, schema validation, fallbacks
  booking/        # Policy, access, cancel tokens
pkg/              # config, token, validator, utils
docs/
  db.dbml         # Canonical schema (source for dbml2sql / dbdocs)
deployments/eks/  # Kubernetes manifests and deploy scripts
```

New API work typically flows: `proto/*.proto` → `make proto` → handler in `api/` → sqlc queries if persistence changes → `make test`.

Specs and acceptance criteria live in **appointa-specs** (`backend/`, `product/`, `testing/`).

## Code generation

After changing protos or SQL:

```bash
make proto    # protoc + grpc-gateway + openapiv2 + statik
make sqlc     # after editing db/query/*.sql
make mock     # after interface changes in db/sqlc or internal/worker
```

New migration:

```bash
make new_migration name=add_some_table
# edit db/migration/*_up.sql and *_down.sql
make migrateup
```

## Development tools

Install once when setting up a new machine:

| Tool | Install |
|------|---------|
| golang-migrate | `brew install golang-migrate` |
| sqlc | `brew install sqlc` |
| protobuf / plugins | `brew install protobuf`; see `make proto` in Makefile for `protoc-gen-go`, grpc-gateway, etc. |
| mockgen | `go install github.com/golang/mock/mockgen@v1.6.0` |
| Air | `go install github.com/air-verse/air@latest` |
| statik | `go install github.com/rakyll/statik` (used by `make proto`) |
| dbdocs | `npm install -g dbdocs` → `make db_docs` |
| dbml2sql | `npm install -g @dbml/cli` → `make db_schema` |

Kubernetes deployment helpers (`kubectl`, `k9s`) and EKS manifests are under `deployments/eks/`.

## Stack

- Go, gRPC, grpc-gateway (JSON/HTTP)
- PostgreSQL (pgx), golang-migrate, sqlc
- Redis + Asynq for async email
- PASETO tokens, Google OAuth, SMTP email
- OpenAI (optional) for booking and slot assistants
