package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/rs/zerolog/log"
)

const TaskSendVerificationEmail = "task:send_verification_email"

type PayloadSendVerificationEmail struct {
	Username string `json:"username"`
}

func (distibutor *RedisTaskDistributor) DistributeTaskSendVerificationEmail(
	ctx context.Context,
	payload *PayloadSendVerificationEmail,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TaskSendVerificationEmail, jsonPayload, opts...)
	info, err := distibutor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")

	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskSendVerificationEmail(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadSendVerificationEmail
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal task payload: %w", asynq.SkipRetry)
	}

	user, err := processor.store.GetUserByUsername(ctx, payload.Username)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			return fmt.Errorf("user not found: %w", asynq.SkipRetry)
		}
		return fmt.Errorf("failed to find user: %w", err)
	}

	// generate random token
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	randomToken := u.String()

	// create verification email record in db
	verificationEmail, err := processor.store.CreateVerificationEmail(ctx, db.CreateVerificationEmailParams{
		Email:             user.Email,
		VerificationToken: randomToken,
	})
	if err != nil {
		return fmt.Errorf("failed to create verification email: %w", err)
	}

	// send email
	subject := "Welcome to Ytter"
	// TODO: utilise email templates and replace debug with frontend url
	verificationURL := fmt.Sprintf("http://localhost:8080/v1/auth/verify_email?verification_token=%s", verificationEmail.VerificationToken)
	content := fmt.Sprintf(`Hello %s,<br/>
	Thank you for registering!<br/>
	<a href="%s">Click here to verify your email</a>
	`, user.FirstName, verificationURL)
	to := []string{user.Email}

	err = processor.mailer.SendEmail(subject, content, to, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("email", user.Email).Msg("processed task")

	return nil
}
