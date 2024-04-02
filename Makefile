DB_URL=postgresql://root:secret@localhost:5432/ytter?sslmode=disable

up:
	docker compose up

down:
	docker compose down

db_docs:
	dbdocs build doc/db.dbml

db_schema:
	dbml2sql --postgres -o doc/schema.sql doc/db.dbml

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
	protoc --proto_path=proto --go_out=pb --go_opt=paths=source_relative \
	--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
	--grpc-gateway_out=pb --grpc-gateway_opt=paths=source_relative \
	proto/*.proto

test:
	go test -v -count=1 -cover -short ./...

.PHONY: up down db_docs db_schema new_migration migrateup migratedown migrateup1 migratedown1 server sqlc proto test