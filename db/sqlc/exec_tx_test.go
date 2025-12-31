package db

import (
	"testing"
)

func TestExecTxIndirectly(t *testing.T) {
	// execTx is tested indirectly through:
	// - CreateUserTx (tested in user_test.go)
	// - VerifyEmailTx (tested in user_test.go)
	// These tests cover:
	// - Successful transaction commit
	// - Rollback on error
	// - Commit error handling
	t.Skip("execTx is tested indirectly through CreateUserTx and VerifyEmailTx")
}
