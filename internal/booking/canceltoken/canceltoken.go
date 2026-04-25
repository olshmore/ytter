package canceltoken

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash returns a 64-character lowercase hex SHA-256 of the raw cancel secret for storage in bookings.cancel_token_hash.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
