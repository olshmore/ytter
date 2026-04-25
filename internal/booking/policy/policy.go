// package policy implements policy enforcement for booking cancellation notice and customer cancel windows.
package policy

import "time"

// DefaultCancellationMinHours is used when both location and service policy fields are NULL.
// override via deployment config if needed.
const DefaultCancellationMinHours = 24

// EffectiveCancellationMinHours returns service override if set, else location value if set, else the platform default.
func EffectiveCancellationMinHours(serviceOverride *int32, locationHours *int32) int {
	switch {
	case serviceOverride != nil:
		return int(*serviceOverride)
	case locationHours != nil:
		return int(*locationHours)
	default:
		return DefaultCancellationMinHours
	}
}

// WithinCustomerCancelWindow reports whether a customer may cancel at instant now against slotStart.
func WithinCustomerCancelWindow(now, slotStart time.Time, effectiveMinWholeHours int) bool {
	if effectiveMinWholeHours < 0 {
		effectiveMinWholeHours = 0
	}
	if slotStart.IsZero() || !slotStart.After(now) {
		return false
	}
	if effectiveMinWholeHours == 0 {
		return true
	}
	minDur := time.Duration(effectiveMinWholeHours) * time.Hour
	return slotStart.Sub(now) >= minDur
}
