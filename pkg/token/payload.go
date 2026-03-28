package token

import (
	"time"

	"github.com/google/uuid"
	"github.com/olshmore/ytter/pkg/utils"
)

// Payload contains the payload data of the token
type Payload struct {
	ID        uuid.UUID  `json:"id"`
	Username  string     `json:"username"`
	Roles     []utils.Role `json:"roles"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiredAt time.Time  `json:"expired_at"`
}

// NewPayload creates a new token payload with a specific username and duration
func NewPayload(username string, roles []utils.Role, duration time.Duration) (*Payload, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	normalizedRoles := normalizeRoles(roles)
	if len(normalizedRoles) == 0 {
		normalizedRoles = []utils.Role{utils.RoleClient}
	}

	payload := &Payload{
		ID:        tokenID,
		Username:  username,
		Roles:     normalizedRoles,
		IssuedAt:  time.Now(),
		ExpiredAt: time.Now().Add(duration),
	}
	return payload, nil
}

func normalizeRoles(roles []utils.Role) []utils.Role {
	roleSet := map[utils.Role]struct{}{}
	normalized := make([]utils.Role, 0, len(roles))

	for _, role := range roles {
		if !utils.IsCanonicalRole(role) {
			continue
		}
		if _, exists := roleSet[role]; exists {
			continue
		}
		roleSet[role] = struct{}{}
		normalized = append(normalized, role)
	}

	return normalized
}

// Valid checks if the token payload is valid or not
func (payload *Payload) Valid() error {
	if time.Now().After(payload.ExpiredAt) {
		return ErrExpiredToken
	}
	return nil
}
