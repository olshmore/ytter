package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rakyll/statik/fs"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/olshmore/ytter/api"
	"github.com/olshmore/ytter/config"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"

	_ "github.com/olshmore/ytter/doc/statik"

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
		log.Fatal().Msgf("failed to load config err: %s", err)
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
	runGatewayServer(ctx, waitGroup, config, store)

	err = waitGroup.Wait()
	if err != nil {
		log.Fatal().Msgf("error from wait group: %s", err)
	}
}

func runDBMigrations(migrationURL string, DBSource string) {
	migration, err := migrate.New(migrationURL, DBSource)
	if err != nil {
		log.Fatal().Msgf("failed to create new migrate instance err: %s", err)
	}

	if err = migration.Up(); err != nil {
		if err == migrate.ErrNoChange {
			log.Info().Msg("db migration success! no change")
			return
		} else {
			log.Fatal().Msgf("failed to run migrate up: %s", err)
		}
	}

	log.Info().Msg("db migration success! db migrated")
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

func runGatewayServer(
	ctx context.Context,
	waitGroup *errgroup.Group,
	config config.Config,
	store db.Store,
) {
	server, err := api.NewServer(config, store)
	if err != nil {
		log.Fatal().Msgf("cannot create server: %s", err)
	}

	jsonOption := runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions: protojson.MarshalOptions{
			UseProtoNames: true,
		},
		UnmarshalOptions: protojson.UnmarshalOptions{
			DiscardUnknown: true,
		},
	})

	grpcMux := runtime.NewServeMux(jsonOption)

	err = pb.RegisterYtterHandlerServer(ctx, grpcMux, server)
	if err != nil {
		log.Fatal().Msgf("cannot register handler server: %s", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", grpcMux)

	// Swagger API Docs
	statikFS, err := fs.New()
	if err != nil {
		log.Fatal().Msgf("cannot create statik file system: %s", err)
	}

	swaggerHandler := http.StripPrefix("/swagger/", http.FileServer(statikFS))
	mux.Handle("/swagger/", swaggerHandler)

	httpServer := &http.Server{
		Handler: mux,
		Addr:    config.HTTPServerAddress,
	}

	waitGroup.Go(func() error {
		log.Info().Msgf("starting HTTP Gateway server at %s", httpServer.Addr)

		err = httpServer.ListenAndServe()
		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}

			log.Error().Msgf("HTTP Gateway server falied to serve: %s", err)

			return err
		}
		return nil
	})

	waitGroup.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("graceful shutdown HTTP gateway server")

		httpServer.Shutdown(context.Background())
		if err != nil {
			log.Error().Msg("falied to shutdown HTTP gateway server")

			return err
		}

		log.Info().Msg("HTTP gateway server is stopped")

		return nil
	})
}
