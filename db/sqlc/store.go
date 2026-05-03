package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Querier
	CreateUserTx(ctx context.Context, arg CreateUserTxParams) (CreateUserTxResult, error)
	VerifyEmailTx(ctx context.Context, arg VerifyEmailTxParams) (VerifyEmailTxResult, error)
	CreatePublicBookingTx(ctx context.Context, arg CreatePublicBookingTxParams) (CreatePublicBookingTxResult, error)
	CancelPublicBookingTx(ctx context.Context, arg CancelPublicBookingTxParams) (CancelPublicBookingTxResult, error)
	HostApproveBookingTx(ctx context.Context, arg HostApproveBookingTxParams) (HostApproveBookingTxResult, error)
	HostRejectBookingTx(ctx context.Context, arg HostRejectBookingTxParams) (HostRejectBookingTxResult, error)
	HostCancelBookingTx(ctx context.Context, arg HostCancelBookingTxParams) (HostCancelBookingTxResult, error)
	HostSetBookingNoShowTx(ctx context.Context, arg HostSetBookingNoShowTxParams) (HostSetBookingNoShowTxResult, error)
	CancelMyBookingTx(ctx context.Context, arg CancelMyBookingTxParams) (CancelMyBookingTxResult, error)
}

type SQLStore struct {
	connPool *pgxpool.Pool
	*Queries
}

func NewStore(connPool *pgxpool.Pool) Store {
	return &SQLStore{
		connPool: connPool,
		Queries:  New(connPool),
	}
}
