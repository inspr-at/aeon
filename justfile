# PAIMOS AEON development recipes

db_url := "postgres://aeon:aeon@127.0.0.1:55432/aeon?sslmode=disable"

# Start a local Postgres 18 + pgvector (Docker via Colima)
db-up:
    docker run -d --name aeon-dev-db -p 55432:5432 -e POSTGRES_USER=aeon -e POSTGRES_PASSWORD=aeon -e POSTGRES_DB=aeon pgvector/pgvector:pg18 >/dev/null 2>&1 || docker start aeon-dev-db >/dev/null
    until docker exec aeon-dev-db pg_isready -U aeon >/dev/null 2>&1; do sleep 1; done
    @echo "postgres ready at {{db_url}}"

# Stop the local Postgres
db-down:
    docker stop aeon-dev-db

# Go tests. The URL is a maintenance database; each test creates its own.
test:
    AEON_TEST_DATABASE_URL="postgres://aeon:aeon@127.0.0.1:55432/aeon?sslmode=disable" go test ./...

# Web typecheck and build
web-check:
    cd web && npm run typecheck && npm run build

# Playwright smoke against a running server (BASE_URL defaults to http://127.0.0.1:8080)
e2e:
    cd web && npx playwright install chromium && npm run e2e

# Build the web app and the binary
build: web-check
    go build -o bin/aeon ./cmd/aeon

# Run the server locally
dev:
    AEON_DATABASE_URL="{{db_url}}" go run ./cmd/aeon serve

# Release checks (bundle pin and version source)
release-check:
    node scripts/verify-release.mjs

# Release history manifest (inspr.release-history.v1) embedded in the server; reads the local tags.
release-history:
    go run ./internal/releasehistory/generate -repo . -repository inspr-at/aeon -offline
