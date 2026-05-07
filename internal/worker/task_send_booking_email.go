package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
)

const (
	TaskSendBookingEmail         = "task:send_booking_email"
	TaskSendBookingReminderEmail = "task:send_booking_reminder_email"
)

type PayloadSendBookingEmail struct {
	BookingID string `json:"booking_id"`
	EventType string `json:"event_type"`
}

type PayloadSendBookingReminderEmail struct {
	BookingID string `json:"booking_id"`
}

func (distibutor *RedisTaskDistributor) DistributeTaskSendBookingEmail(
	ctx context.Context,
	payload *PayloadSendBookingEmail,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TaskSendBookingEmail, jsonPayload, opts...)
	info, err := distibutor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

func (distibutor *RedisTaskDistributor) DistributeTaskSendBookingReminderEmail(
	ctx context.Context,
	payload *PayloadSendBookingReminderEmail,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TaskSendBookingReminderEmail, jsonPayload, opts...)
	info, err := distibutor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskSendBookingEmail(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadSendBookingEmail
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal task payload: %w", asynq.SkipRetry)
	}

	bookingID, err := uuid.Parse(payload.BookingID)
	if err != nil {
		return fmt.Errorf("invalid booking_id in payload: %w", asynq.SkipRetry)
	}

	booking, err := processor.store.GetBookingByID(ctx, bookingID)
	if err != nil {
		return fmt.Errorf("failed to load booking: %w", err)
	}

	subject := fmt.Sprintf("Booking %s", payload.EventType)
	content := fmt.Sprintf(
		"Hello %s,<br/>Your booking (%s) was %s.<br/>Current status: %s",
		booking.GuestName,
		booking.ID.String(),
		payload.EventType,
		booking.Status,
	)
	to := []string{booking.GuestEmail}

	if processor.config.EmailSenderAddress == "" {
		log.Debug().Str("booking_id", booking.ID.String()).Str("event_type", payload.EventType).Msg("debug: booking email would have been sent")
	} else {
		if err := processor.mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
			return fmt.Errorf("failed to send booking email: %w", err)
		}
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("email", booking.GuestEmail).Msg("processed task")
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskSendBookingReminderEmail(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadSendBookingReminderEmail
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal task payload: %w", asynq.SkipRetry)
	}

	bookingID, err := uuid.Parse(payload.BookingID)
	if err != nil {
		return fmt.Errorf("invalid booking_id in payload: %w", asynq.SkipRetry)
	}

	row, err := processor.store.GetBookingForCancelByIDForUpdate(ctx, bookingID)
	if err != nil {
		return fmt.Errorf("failed to load booking reminder context: %w", err)
	}

	if row.Status != "pending" && row.Status != "confirmed" {
		log.Info().Str("booking_id", bookingID.String()).Str("status", row.Status).Msg("skipping reminder for ineligible status")
		return nil
	}
	if !row.StartAt.After(time.Now()) {
		log.Info().Str("booking_id", bookingID.String()).Msg("skipping reminder for past slot")
		return nil
	}

	subject := "Booking reminder"
	content := fmt.Sprintf(
		"Hello %s,<br/>This is a reminder for your booking on %s.",
		row.GuestName,
		row.StartAt.Format(time.RFC3339),
	)
	to := []string{row.GuestEmail}

	if processor.config.EmailSenderAddress == "" {
		log.Debug().Str("booking_id", bookingID.String()).Msg("debug: booking reminder email would have been sent")
	} else {
		if err := processor.mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
			return fmt.Errorf("failed to send booking reminder email: %w", err)
		}
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("email", row.GuestEmail).Msg("processed task")
	return nil
}
