package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type VerifyEmailTxParams struct {
	Email             string
	VerificationToken string
}

type VerifyEmailTxResult struct {
	User              User
	VerificationEmail VerificationEmail
}

func (store *SQLStore) VerifyEmailTx(ctx context.Context, arg VerifyEmailTxParams) (VerifyEmailTxResult, error) {
	var result VerifyEmailTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		verificationEmailRow, err := q.GetVerificationEmailByToken(ctx, arg.VerificationToken)
		if err != nil {
			return err
		}

		result.VerificationEmail, err = q.UpdateVerificationEmail(ctx, UpdateVerificationEmailParams{
			Email:             verificationEmailRow.Email,
			VerificationToken: arg.VerificationToken,
		})
		if err != nil {
			return err
		}

		result.User, err = q.UpdateUserEmailVerified(ctx, UpdateUserEmailVerifiedParams{
			Email: result.VerificationEmail.Email,
			IsEmailVerified: pgtype.Bool{
				Bool:  true,
				Valid: true,
			},
		})

		return err
	})

	return result, err
}
