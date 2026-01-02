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
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rakyll/statik/fs"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/olshmore/ytter/api"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/email"
	"github.com/olshmore/ytter/internal/worker"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/config"

	_ "github.com/olshmore/ytter/docs/statik"

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
	config, err := config.LoadConfig("./config")
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

	// Connect to Redis
	redisOpt := asynq.RedisClientOpt{
		Addr: config.RedisAddress,
	}
	taskDistributor := worker.NewRedisTaskDistributor(redisOpt)

	waitGroup, ctx := errgroup.WithContext(ctx)

	runTaskProcessor(ctx, waitGroup, config, redisOpt, store)
	runGrpcServer(ctx, waitGroup, config, store, taskDistributor)
	runGatewayServer(ctx, waitGroup, config, store, taskDistributor)

	err = waitGroup.Wait()
	if err != nil {
		log.Fatal().Err(err).Msg("error from wait group")
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

func runTaskProcessor(
	ctx context.Context,
	waitGroup *errgroup.Group,
	config config.Config,
	redisOpt asynq.RedisClientOpt,
	store db.Store,
) {
	mailer := email.NewGmailSender(config.EmailSenderName, config.EmailSenderAddress, config.EmailSenderPassword)

	taskProcessor := worker.NewRedisTaskProcessor(redisOpt, store, mailer)

	err := taskProcessor.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start task processor")
	}

	log.Info().Msg("started task processor")

	waitGroup.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("graceful shutdown task processor")

		taskProcessor.Shutdown()
		log.Info().Msg("task processor is stopped")

		return nil
	})
}

func runGrpcServer(
	ctx context.Context,
	waitGroup *errgroup.Group,
	config config.Config,
	store db.Store,
	taskDistributor worker.TaskDistributor,
) {
	server, err := api.NewServer(config, store, taskDistributor)
	if err != nil {
		log.Fatal().Msg("failed to create server")
	}

	roleConfig := api.ConfigureRoleBasedAccess()

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			api.GrpcLogger,
			server.RequireRoles(roleConfig),
		),
	)

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
	taskDistributor worker.TaskDistributor,
) {
	server, err := api.NewServer(config, store, taskDistributor)
	if err != nil {
		log.Fatal().Msgf("cannot create server: %s", err)
	}

	jsonOption := runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions: protojson.MarshalOptions{
			UseProtoNames:   true,
			EmitUnpopulated: true,
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

	// Role-based access control
	roleConfig := api.ConfigureRoleBasedAccess()
	handler := server.RequireRolesHTTP(roleConfig)(mux)

	// CORS
	c := cors.New(cors.Options{
		AllowedOrigins: config.AllowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders: []string{
			"Authorization",
			"Content-Type",
		},
		AllowCredentials: true,
	})
	handler = c.Handler(api.HttpLogger(handler))

	httpServer := &http.Server{
		Handler: handler,
		Addr:    config.HTTPServerAddress,
	}

	waitGroup.Go(func() error {
		log.Info().Msgf("starting HTTP Gateway server at %s", httpServer.Addr)

		err = httpServer.ListenAndServe()
		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}

			log.Error().Msgf("HTTP Gateway server failed to serve: %s", err)

			return err
		}
		return nil
	})

	waitGroup.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("graceful shutdown HTTP gateway server")

		err := httpServer.Shutdown(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("failed to shutdown HTTP gateway server")
			return err
		}

		log.Info().Msg("HTTP gateway server is stopped")

		return nil
	})
}
