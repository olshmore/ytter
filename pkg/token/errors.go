package token

import "errors"

// Types of error returned by the VerifyToken function
var (
	ErrInvalidKeySize = errors.New("invalid key size")
	ErrInvalidToken   = errors.New("token is invalid")
	ErrExpiredToken   = errors.New("token has expired")
)
