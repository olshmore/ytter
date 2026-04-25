// package access implements access control for host location ownership and client booking linkage.
package access

import (
	"strings"

	"github.com/olshmore/ytter/pkg/token"
	"github.com/olshmore/ytter/pkg/utils"
)

// HostMayAccessLocation is true when the caller may act on a host-scoped resource for this location:
// admins may access any location; hosts only when they own it (owner username matches JWT username).
func HostMayAccessLocation(payload *token.Payload, locationOwnerUsername string) bool {
	if payload == nil {
		return false
	}
	for _, r := range payload.Roles {
		if r == utils.RoleAdmin {
			return true
		}
		if r == utils.RoleHost && payload.Username == locationOwnerUsername {
			return true
		}
	}
	return false
}

// ClientMayAccessLinkedBooking is true for admins, or for clients when the booking is linked to their username.
// non-linked bookings (nil or empty client_username) are denied for non-admins — use claim/token flows first.
func ClientMayAccessLinkedBooking(payload *token.Payload, bookingClientUsername *string) bool {
	if payload == nil {
		return false
	}
	for _, r := range payload.Roles {
		if r == utils.RoleAdmin {
			return true
		}
	}
	if !hasRole(payload.Roles, utils.RoleClient) {
		return false
	}
	if bookingClientUsername == nil || *bookingClientUsername == "" {
		return false
	}
	return *bookingClientUsername == payload.Username
}

// GuestEmailMatchesUser compares guest and account emails with case-folding.
func GuestEmailMatchesUser(guestEmail, userEmail string) bool {
	return strings.EqualFold(strings.TrimSpace(guestEmail), strings.TrimSpace(userEmail))
}

func hasRole(roles []utils.Role, want utils.Role) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}
