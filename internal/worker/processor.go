package worker

import (
	"context"

	"github.com/hibiken/asynq"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/email"
	"github.com/olshmore/ytter/pkg/config"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

type TaskProcessor interface {
	Start() error
	Shutdown()
	ProcessTaskSendVerificationEmail(
		ctx context.Context,
		task *asynq.Task,
	) error
	ProcessTaskSendBookingEmail(
		ctx context.Context,
		task *asynq.Task,
	) error
	ProcessTaskSendBookingReminderEmail(
		ctx context.Context,
		task *asynq.Task,
	) error
}

type RedisTaskProcessor struct {
	server *asynq.Server
	store  db.Store
	mailer email.EmailSender
	config config.Config
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, store db.Store, mailer email.EmailSender, config config.Config) TaskProcessor {
	logger := NewLogger()
	redis.SetLogger(logger)

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Queues: map[string]int{
				QueueCritical: 10,
				QueueDefault:  5,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Error().Err(err).Str("type", task.Type()).Bytes("payload", task.Payload()).Msg("failed to process task")
			}),
			Logger: logger,
		},
	)

	return &RedisTaskProcessor{
		server: server,
		store:  store,
		mailer: mailer,
		config: config,
	}
}

func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TaskSendVerificationEmail, processor.ProcessTaskSendVerificationEmail)
	mux.HandleFunc(TaskSendBookingEmail, processor.ProcessTaskSendBookingEmail)
	mux.HandleFunc(TaskSendBookingReminderEmail, processor.ProcessTaskSendBookingReminderEmail)

	return processor.server.Start(mux)
}

func (processor *RedisTaskProcessor) Shutdown() {
	processor.server.Shutdown()
}
