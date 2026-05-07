package api

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/olshmore/ytter/internal/worker"
)

const defaultBookingReminderLead = 24 * time.Hour

func (server *Server) emitBookingEmailEvent(ctx context.Context, bookingID string, eventType string) {
	if server.taskDistributor == nil {
		return
	}

	_ = server.taskDistributor.DistributeTaskSendBookingEmail(
		ctx,
		&worker.PayloadSendBookingEmail{
			BookingID: bookingID,
			EventType: eventType,
		},
		asynq.MaxRetry(10),
		asynq.Queue(worker.QueueCritical),
	)
}

func (server *Server) scheduleBookingReminderEmail(ctx context.Context, bookingID string, slotStartAt time.Time) {
	if server.taskDistributor == nil {
		return
	}

	sendAt := slotStartAt.Add(-defaultBookingReminderLead)
	if time.Now().After(sendAt) {
		return
	}

	_ = server.taskDistributor.DistributeTaskSendBookingReminderEmail(
		ctx,
		&worker.PayloadSendBookingReminderEmail{
			BookingID: bookingID,
		},
		asynq.MaxRetry(10),
		asynq.Queue(worker.QueueDefault),
		asynq.ProcessAt(sendAt),
	)
}
