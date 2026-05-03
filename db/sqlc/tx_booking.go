package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/olshmore/ytter/internal/booking/access"
	"github.com/olshmore/ytter/internal/booking/policy"
	"github.com/olshmore/ytter/pkg/token"
)

var (
	ErrHostBookingAccessDenied   = errors.New("host booking access denied")
	ErrHostBookingApproveState   = errors.New("host booking approve invalid state")
	ErrHostBookingRejectState    = errors.New("host booking reject invalid state")
	ErrHostBookingCancelState    = errors.New("host booking cancel invalid state")
	ErrHostBookingNoShowState    = errors.New("host booking no-show invalid state")
	ErrClientBookingAccessDenied = errors.New("client booking access denied")
	ErrClientBookingCancelState  = errors.New("client booking cancel invalid state")
)

type CreatePublicBookingTxParams struct {
	LocationID      uuid.UUID
	SlotID          uuid.UUID
	GuestName       string
	GuestEmail      string
	GuestPhone      string
	GuestNotes      string
	CancelTokenHash string
	ClientUsername  string
}

type CreatePublicBookingTxResult struct {
	Booking Booking
}

func (store *SQLStore) CreatePublicBookingTx(ctx context.Context, arg CreatePublicBookingTxParams) (CreatePublicBookingTxResult, error) {
	var result CreatePublicBookingTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		location, err := q.GetLocationByID(ctx, arg.LocationID)
		if err != nil {
			return err
		}
		if !location.IsActive {
			return fmt.Errorf("location inactive")
		}

		slot, err := q.GetSlotForUpdate(ctx, GetSlotForUpdateParams{
			ID:         arg.SlotID,
			LocationID: arg.LocationID,
		})
		if err != nil {
			return err
		}
		if !slot.StartAt.After(time.Now()) {
			return fmt.Errorf("slot in the past")
		}
		if slot.Status != "available" || slot.BookedCount >= slot.Capacity {
			return fmt.Errorf("slot unavailable")
		}

		status := "confirmed"
		if location.BookingRequiresHostApproval {
			status = "pending"
		}

		guestPhone := pgtype.Text{}
		if arg.GuestPhone != "" {
			guestPhone = pgtype.Text{String: arg.GuestPhone, Valid: true}
		}
		guestNotes := pgtype.Text{}
		if arg.GuestNotes != "" {
			guestNotes = pgtype.Text{String: arg.GuestNotes, Valid: true}
		}

		result.Booking, err = q.CreateBooking(ctx, CreateBookingParams{
			LocationID:      arg.LocationID,
			SlotID:          arg.SlotID,
			Status:          status,
			GuestName:       arg.GuestName,
			GuestEmail:      arg.GuestEmail,
			GuestPhone:      guestPhone,
			GuestNotes:      guestNotes,
			CancelTokenHash: arg.CancelTokenHash,
			ClientUsername:  nullablePGText(arg.ClientUsername),
		})
		if err != nil {
			return err
		}

		nextBooked := slot.BookedCount + 1
		nextStatus := slot.Status
		if nextBooked >= slot.Capacity {
			nextStatus = "booked"
		}
		_, err = q.UpdateSlotCounters(ctx, UpdateSlotCountersParams{
			ID:          slot.ID,
			BookedCount: nextBooked,
			Status:      nextStatus,
		})
		return err
	})

	return result, err
}

func nullablePGText(v string) pgtype.Text {
	if strings.TrimSpace(v) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(v), Valid: true}
}

type CancelPublicBookingTxParams struct {
	BookingID    uuid.UUID
	CancelToken  string
	CancelReason string
	Now          time.Time
}

type CancelPublicBookingTxResult struct {
	Booking Booking
}

func (store *SQLStore) CancelPublicBookingTx(ctx context.Context, arg CancelPublicBookingTxParams) (CancelPublicBookingTxResult, error) {
	var result CancelPublicBookingTxResult
	err := store.execTx(ctx, func(q *Queries) error {
		row, err := q.GetBookingForCancelByIDForUpdate(ctx, arg.BookingID)
		if err != nil {
			return err
		}
		if row.Status != "pending" && row.Status != "confirmed" {
			return fmt.Errorf("booking not cancellable in current status")
		}
		if row.CancelTokenHash != arg.CancelToken {
			return fmt.Errorf("invalid cancel token")
		}

		var serviceHours *int32
		if row.ServiceCancellationMinHoursBeforeStart.Valid {
			v := row.ServiceCancellationMinHoursBeforeStart.Int32
			serviceHours = &v
		}
		var locationHours *int32
		if row.LocationCancellationMinHoursBeforeStart.Valid {
			v := row.LocationCancellationMinHoursBeforeStart.Int32
			locationHours = &v
		}

		effective := policy.EffectiveCancellationMinHours(serviceHours, locationHours)
		if !policy.WithinCustomerCancelWindow(arg.Now, row.StartAt, effective) {
			return fmt.Errorf("outside cancellation window")
		}

		cancelReason := pgtype.Text{}
		if arg.CancelReason != "" {
			cancelReason = pgtype.Text{String: arg.CancelReason, Valid: true}
		}
		result.Booking, err = q.MarkBookingCancelled(ctx, MarkBookingCancelledParams{
			ID:           arg.BookingID,
			CancelReason: cancelReason,
		})
		if err != nil {
			return err
		}

		nextBooked := row.BookedCount - 1
		if nextBooked < 0 {
			nextBooked = 0
		}
		nextStatus := "available"
		if nextBooked >= row.Capacity {
			nextStatus = "booked"
		}

		_, err = q.UpdateSlotCounters(ctx, UpdateSlotCountersParams{
			ID:          row.SlotID,
			BookedCount: nextBooked,
			Status:      nextStatus,
		})
		return err
	})

	return result, err
}

func bookingFromHostOpRow(row GetBookingForHostOpByIDForUpdateRow) Booking {
	return Booking{
		ID:              row.ID,
		LocationID:      row.LocationID,
		SlotID:          row.SlotID,
		Status:          row.Status,
		GuestName:       row.GuestName,
		GuestEmail:      row.GuestEmail,
		GuestPhone:      row.GuestPhone,
		GuestNotes:      row.GuestNotes,
		ClientUsername:  row.ClientUsername,
		BookedAt:        row.BookedAt,
		CancelledAt:     row.CancelledAt,
		CancelReason:    row.CancelReason,
		CancelTokenHash: row.CancelTokenHash,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		DeletedAt:       row.DeletedAt,
	}
}

type HostApproveBookingTxParams struct {
	BookingID uuid.UUID
	Actor     *token.Payload
}

type HostApproveBookingTxResult struct {
	Booking Booking
}

func (store *SQLStore) HostApproveBookingTx(ctx context.Context, arg HostApproveBookingTxParams) (HostApproveBookingTxResult, error) {
	var result HostApproveBookingTxResult
	err := store.execTx(ctx, func(q *Queries) error {
		row, err := q.GetBookingForHostOpByIDForUpdate(ctx, arg.BookingID)
		if err != nil {
			return err
		}
		if !access.HostMayAccessLocation(arg.Actor, row.OwnerUsername) {
			return ErrHostBookingAccessDenied
		}
		switch row.Status {
		case "confirmed":
			result.Booking = bookingFromHostOpRow(row)
			return nil
		case "pending":
			b, err := q.ConfirmBooking(ctx, arg.BookingID)
			if err != nil {
				return err
			}
			result.Booking = b
			return nil
		default:
			return ErrHostBookingApproveState
		}
	})
	return result, err
}

type HostRejectBookingTxParams struct {
	BookingID    uuid.UUID
	Actor        *token.Payload
	CancelReason string
}

type HostRejectBookingTxResult struct {
	Booking Booking
}

func (store *SQLStore) HostRejectBookingTx(ctx context.Context, arg HostRejectBookingTxParams) (HostRejectBookingTxResult, error) {
	var result HostRejectBookingTxResult
	err := store.execTx(ctx, func(q *Queries) error {
		row, err := q.GetBookingForHostOpByIDForUpdate(ctx, arg.BookingID)
		if err != nil {
			return err
		}
		if !access.HostMayAccessLocation(arg.Actor, row.OwnerUsername) {
			return ErrHostBookingAccessDenied
		}
		if row.Status != "pending" {
			return ErrHostBookingRejectState
		}
		cancelReason := pgtype.Text{}
		if arg.CancelReason != "" {
			cancelReason = pgtype.Text{String: arg.CancelReason, Valid: true}
		}
		result.Booking, err = q.MarkBookingCancelled(ctx, MarkBookingCancelledParams{
			ID:           arg.BookingID,
			CancelReason: cancelReason,
		})
		if err != nil {
			return err
		}
		nextBooked := row.BookedCount - 1
		if nextBooked < 0 {
			nextBooked = 0
		}
		nextStatus := "available"
		if nextBooked >= row.Capacity {
			nextStatus = "booked"
		}
		_, err = q.UpdateSlotCounters(ctx, UpdateSlotCountersParams{
			ID:          row.SlotID,
			BookedCount: nextBooked,
			Status:      nextStatus,
		})
		return err
	})
	return result, err
}

type HostCancelBookingTxParams struct {
	BookingID    uuid.UUID
	Actor        *token.Payload
	CancelReason string
}

type HostCancelBookingTxResult struct {
	Booking Booking
}

func (store *SQLStore) HostCancelBookingTx(ctx context.Context, arg HostCancelBookingTxParams) (HostCancelBookingTxResult, error) {
	var result HostCancelBookingTxResult
	err := store.execTx(ctx, func(q *Queries) error {
		row, err := q.GetBookingForHostOpByIDForUpdate(ctx, arg.BookingID)
		if err != nil {
			return err
		}
		if !access.HostMayAccessLocation(arg.Actor, row.OwnerUsername) {
			return ErrHostBookingAccessDenied
		}
		if row.Status != "pending" && row.Status != "confirmed" {
			return ErrHostBookingCancelState
		}
		cancelReason := pgtype.Text{}
		if arg.CancelReason != "" {
			cancelReason = pgtype.Text{String: arg.CancelReason, Valid: true}
		}
		result.Booking, err = q.MarkBookingCancelled(ctx, MarkBookingCancelledParams{
			ID:           arg.BookingID,
			CancelReason: cancelReason,
		})
		if err != nil {
			return err
		}
		nextBooked := row.BookedCount - 1
		if nextBooked < 0 {
			nextBooked = 0
		}
		nextStatus := "available"
		if nextBooked >= row.Capacity {
			nextStatus = "booked"
		}
		_, err = q.UpdateSlotCounters(ctx, UpdateSlotCountersParams{
			ID:          row.SlotID,
			BookedCount: nextBooked,
			Status:      nextStatus,
		})
		return err
	})
	return result, err
}

type HostSetBookingNoShowTxParams struct {
	BookingID uuid.UUID
	NoShow    bool
	Actor     *token.Payload
}

type HostSetBookingNoShowTxResult struct {
	Booking Booking
}

func (store *SQLStore) HostSetBookingNoShowTx(ctx context.Context, arg HostSetBookingNoShowTxParams) (HostSetBookingNoShowTxResult, error) {
	var result HostSetBookingNoShowTxResult
	err := store.execTx(ctx, func(q *Queries) error {
		row, err := q.GetBookingForHostOpByIDForUpdate(ctx, arg.BookingID)
		if err != nil {
			return err
		}
		if !access.HostMayAccessLocation(arg.Actor, row.OwnerUsername) {
			return ErrHostBookingAccessDenied
		}
		if arg.NoShow {
			if row.Status != "confirmed" {
				return ErrHostBookingNoShowState
			}
			b, err := q.MarkBookingNoShow(ctx, arg.BookingID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrHostBookingNoShowState
				}
				return err
			}
			result.Booking = b
			return nil
		}
		if row.Status != "no_show" {
			return ErrHostBookingNoShowState
		}
		b, err := q.ClearBookingNoShow(ctx, arg.BookingID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrHostBookingNoShowState
			}
			return err
		}
		result.Booking = b
		return nil
	})
	return result, err
}

type CancelMyBookingTxParams struct {
	BookingID    uuid.UUID
	Actor        *token.Payload
	ActorEmail   string
	CancelReason string
	Now          time.Time
}

type CancelMyBookingTxResult struct {
	Booking Booking
}

func (store *SQLStore) CancelMyBookingTx(ctx context.Context, arg CancelMyBookingTxParams) (CancelMyBookingTxResult, error) {
	var result CancelMyBookingTxResult
	err := store.execTx(ctx, func(q *Queries) error {
		row, err := q.GetBookingForCancelByIDForUpdate(ctx, arg.BookingID)
		if err != nil {
			return err
		}
		usernameMatch := row.ClientUsername.Valid && row.ClientUsername.String == arg.Actor.Username
		emailMatch := strings.TrimSpace(arg.ActorEmail) != "" && strings.EqualFold(strings.TrimSpace(row.GuestEmail), strings.TrimSpace(arg.ActorEmail))
		if !usernameMatch && !emailMatch {
			return ErrClientBookingAccessDenied
		}
		if row.Status != "pending" && row.Status != "confirmed" {
			return ErrClientBookingCancelState
		}

		var serviceHours *int32
		if row.ServiceCancellationMinHoursBeforeStart.Valid {
			v := row.ServiceCancellationMinHoursBeforeStart.Int32
			serviceHours = &v
		}
		var locationHours *int32
		if row.LocationCancellationMinHoursBeforeStart.Valid {
			v := row.LocationCancellationMinHoursBeforeStart.Int32
			locationHours = &v
		}
		effective := policy.EffectiveCancellationMinHours(serviceHours, locationHours)
		if !policy.WithinCustomerCancelWindow(arg.Now, row.StartAt, effective) {
			return fmt.Errorf("outside cancellation window")
		}

		cancelReason := pgtype.Text{}
		if arg.CancelReason != "" {
			cancelReason = pgtype.Text{String: arg.CancelReason, Valid: true}
		}
		result.Booking, err = q.MarkBookingCancelled(ctx, MarkBookingCancelledParams{
			ID:           arg.BookingID,
			CancelReason: cancelReason,
		})
		if err != nil {
			return err
		}

		nextBooked := row.BookedCount - 1
		if nextBooked < 0 {
			nextBooked = 0
		}
		nextStatus := "available"
		if nextBooked >= row.Capacity {
			nextStatus = "booked"
		}
		_, err = q.UpdateSlotCounters(ctx, UpdateSlotCountersParams{
			ID:          row.SlotID,
			BookedCount: nextBooked,
			Status:      nextStatus,
		})
		return err
	})
	return result, err
}
