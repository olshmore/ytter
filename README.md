# ytter
ytter

## Tools
### migrate
```
brew install golang-migrate
```

### dbdocs
###### **make db_docs** creates db documentetion
```
npm install -g dbdocs
dbdocs login
```

### dbml2sql
###### **make db_schema** creates doc/schema.sql
```
npm install -g @dbml/cli
```

### sqlc
```
brew install sqlc
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

## Documentation
### db documentetion
```
make db_docs
```

## Code generation
### New migration
```
make new_migration name=<migration_name>
```

### SQL: generate doc/schema.sql form doc/db.dbml
```
make db_schema
```

### Generate pb
```
make proto
```