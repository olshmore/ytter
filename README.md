# ytter
ytter

## Tools
### migrate
```
brew install golang-migrate
migrate -version
```

### dbdocs
###### **make db_docs** creates db documentetion
```
npm install -g dbdocs
dbdocs login
```

### dbml2sql
###### **make db_schema** creates docs/schema.sql
```
npm install -g @dbml/cli
```

### sqlc
```
brew install sqlc
sqlc version
```

### kubectl
```
brew install kubectl
kubectl version --client
```

### k9s
```
brew install derailed/k9s/k9s
```

### proto
```
brew install protobuf
protoc --version

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
protoc-gen-go --version

go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
protoc-gen-go-grpc --version
```

### grpc gateway
go install \
  github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
  github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2 \
  google.golang.org/protobuf/cmd/protoc-gen-go \
  google.golang.org/grpc/cmd/protoc-gen-go-grpc

### mockgen
go install github.com/golang/mock/mockgen@v1.6.0
which mockgen

## Documentation
### db documentetion
```
make db_docs
```

### statik for swagger binary
```
go install github.com/rakyll/statik
statik -help
```

## Code generation
### New migration
```
make new_migration name=<migration_name>
```

### SQL: generate docs/schema.sql form docs/db.dbml
```
make db_schema
```

### Generate pb
```
make proto
```