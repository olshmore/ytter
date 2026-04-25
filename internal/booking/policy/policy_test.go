package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEffectiveCancellationMinHours(t *testing.T) {
	t.Parallel()
	s24 := int32(24)
	s48 := int32(48)
	loc12 := int32(12)

	require.Equal(t, 48, EffectiveCancellationMinHours(&s48, &s24))
	require.Equal(t, 24, EffectiveCancellationMinHours(nil, &s24))
	require.Equal(t, 12, EffectiveCancellationMinHours(nil, &loc12))
	require.Equal(t, DefaultCancellationMinHours, EffectiveCancellationMinHours(nil, nil))
}

func TestWithinCustomerCancelWindow(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	t.Run("zero_hours_requires_future_slot", func(t *testing.T) {
		t.Parallel()
		now := start.Add(-time.Minute)
		require.True(t, WithinCustomerCancelWindow(now, start, 0))
		now = start
		require.False(t, WithinCustomerCancelWindow(now, start, 0))
	})

	t.Run("twenty_four_hours_notice", func(t *testing.T) {
		t.Parallel()
		ok := start.Add(-25 * time.Hour)
		require.True(t, WithinCustomerCancelWindow(ok, start, 24))

		tooLate := start.Add(-23*time.Hour - time.Minute)
		require.False(t, WithinCustomerCancelWindow(tooLate, start, 24))

		exact := start.Add(-24 * time.Hour)
		require.True(t, WithinCustomerCancelWindow(exact, start, 24))
	})
}
