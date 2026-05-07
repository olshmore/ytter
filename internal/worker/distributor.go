package worker

import (
	"context"

	"github.com/hibiken/asynq"
)

type TaskDistributor interface {
	DistributeTaskSendVerificationEmail(
		ctx context.Context,
		payload *PayloadSendVerificationEmail,
		opt ...asynq.Option,
	) error
	DistributeTaskSendBookingEmail(
		ctx context.Context,
		payload *PayloadSendBookingEmail,
		opt ...asynq.Option,
	) error
	DistributeTaskSendBookingReminderEmail(
		ctx context.Context,
		payload *PayloadSendBookingReminderEmail,
		opt ...asynq.Option,
	) error
}

type RedisTaskDistributor struct {
	client *asynq.Client
}

func NewRedisTaskDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	client := asynq.NewClient(redisOpt)
	return &RedisTaskDistributor{
		client: client,
	}
}
