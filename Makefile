include config/apptesting.env
export

dev:
	@which air > /dev/null || (echo "Air is not installed. Install it with: go install github.com/air-verse/air@latest" && exit 1)
	@echo "Starting database services..."
	@docker compose up -d postgres redis
	@echo "Starting development server with hot reload..."
	air

up:
	docker compose up --build

down:
	docker compose down

db_docs:
	dbdocs build docs/db.dbml

db_schema:
	dbml2sql --postgres -o docs/schema.sql docs/db.dbml

new_migration:
	migrate create -ext sql -dir db/migration -seq $(name)
	
migrateup:
	@echo "DB_URL: $(DB_URL)"
	migrate -path db/migration -database "$(DB_URL)" -verbose up

migrateup1:
	migrate -path db/migration -database "$(DB_URL)" -verbose up 1

migratedown:
	migrate -path db/migration -database "$(DB_URL)" -verbose down

migratedown1:
	migrate -path db/migration -database "$(DB_URL)" -verbose down 1

server:
	go run main.go

sqlc:
	sqlc generate

proto:
	rm -f pb/*.go
	rm -f docs/swagger/*.swagger.json
	protoc --proto_path=proto --go_out=pb --go_opt=paths=source_relative \
	--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
	--grpc-gateway_out=pb --grpc-gateway_opt=paths=source_relative \
	--openapiv2_out=docs/swagger --openapiv2_opt=allow_merge=true,merge_file_name=ytter \
	proto/*.proto
	statik -src=./docs/swagger -dest=./docs

test:
	@echo "ENVIRONMENT: $(ENVIRONMENT)"
	@echo "DB_URL_LOCAL: $(DB_URL_LOCAL)"
	@echo "DB_URL: $(DB_URL)"
	go clean -testcache
	go test -v -count=1 -coverprofile=coverage.out -short ./...
	@go tool cover -func=coverage.out | awk '!/\/pb\// && !/\/db\// || /^total:/'

test_all:
	go test -v -count=1 -cover ./...

mock:
	mockgen -package mockdb -destination db/mock/store.go github.com/olshmore/ytter/db/sqlc Store
	mockgen -package mockwk -destination internal/worker/mock/distributor.go github.com/olshmore/ytter/internal/worker TaskDistributor

.PHONY: dev up down db_docs db_schema new_migration migrateup migratedown migrateup1 migratedown1 server sqlc proto test test_all mock