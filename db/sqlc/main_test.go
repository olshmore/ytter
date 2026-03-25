package db

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olshmore/ytter/pkg/config"
)

var testStore Store

func TestMain(m *testing.M) {
	config, err := config.LoadConfig("../../config")
	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	connPool, err := pgxpool.New(context.Background(), config.DBSource)
	if err != nil {
		log.Printf("db/sqlc: skipping tests (failed to create pool): %v", err)
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pingErr := connPool.Ping(ctx)
	cancel()
	if pingErr != nil {
		connPool.Close()
		log.Printf("db/sqlc: skipping tests (database unavailable): %v", pingErr)
		os.Exit(m.Run())
	}

	testStore = NewStore(connPool)
	code := m.Run()
	connPool.Close()
	os.Exit(code)
}
