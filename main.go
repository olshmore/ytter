package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/olshmore/ytter/api"
	"github.com/olshmore/ytter/config"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var interruptSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGINT,
}

func main() {
	// Environment
	config, err := config.LoadConfig(".")
	if err != nil {
		log.Fatal().Msg("failed to load config")
	}

	// Log
	if config.Environment == "development" {
		// Pretty log
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), interruptSignals...)
	defer stop()

	// Connect to PostgreSQL
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		log.Fatal().Msg("failed to connect to db")
	}

	// Run db migrations
	runDBMigrations(config.MigrationURL, config.DBSource)

	// Init store
	store := db.NewStore(connPool)

	waitGroup, ctx := errgroup.WithContext(ctx)

	runGrpcServer(ctx, waitGroup, config, store)

	err = waitGroup.Wait()
	if err != nil {
		log.Fatal().Msg("error from wait group")
	}
}

func runDBMigrations(migrationURL string, DBSource string) {
	migration, err := migrate.New(migrationURL, DBSource)
	if err != nil {
		log.Fatal().Msg("failed to create new migrate instance")
	}

	if err = migration.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal().Msg("failed to run migrate up")
	}

	log.Info().Msg("db migrated successfully")
}

func runGrpcServer(
	ctx context.Context,
	waitGroup *errgroup.Group,
	config config.Config,
	store db.Store,
) {
	server, err := api.NewServer(config, store)
	if err != nil {
		log.Fatal().Msg("failed to create server")
	}

	grpcServer := grpc.NewServer()

	pb.RegisterYtterServer(grpcServer, server)

	reflection.Register(grpcServer)

	listener, err := net.Listen("tcp", config.GRPCServerAddress)
	if err != nil {
		log.Fatal().Msg("failed to create tcp gRPC listener")
	}

	waitGroup.Go(func() error {
		log.Info().Msgf("starting gRPC server at %s", listener.Addr().String())

		err = grpcServer.Serve(listener)
		if err != nil {
			if errors.Is(err, grpc.ErrServerStopped) {
				return nil
			}

			log.Error().Msg("failed to start gRPC server")

			return err
		}

		return nil
	})

	waitGroup.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("graceful shutdown gRPC server")

		grpcServer.GracefulStop()
		log.Info().Msg("gRPC server is stopped")

		return nil
	})
}
