package utils

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandomInt(t *testing.T) {
	min := int64(1)
	max := int64(100)

	for i := 0; i < 100; i++ {
		result := RandomInt(min, max)
		require.GreaterOrEqual(t, result, min)
		require.LessOrEqual(t, result, max)
	}
}

func TestRandomString(t *testing.T) {
	length := 10
	result := RandomString(length)
	require.Len(t, result, length)

	// Check that it only contains lowercase letters
	matched, err := regexp.MatchString(`^[a-z]+$`, result)
	require.NoError(t, err)
	require.True(t, matched)
}

func TestRandomOwner(t *testing.T) {
	result := RandomOwner()
	require.Len(t, result, 6)

	// Check that it only contains lowercase letters
	matched, err := regexp.MatchString(`^[a-z]+$`, result)
	require.NoError(t, err)
	require.True(t, matched)
}

func TestRandomMoney(t *testing.T) {
	for i := 0; i < 100; i++ {
		result := RandomMoney()
		require.GreaterOrEqual(t, result, int64(0))
		require.LessOrEqual(t, result, int64(1000))
	}
}

func TestRandomCurrency(t *testing.T) {
	validCurrencies := map[string]bool{
		"USD": true,
		"EUR": true,
		"GBP": true,
	}

	for i := 0; i < 100; i++ {
		result := RandomCurrency()
		require.True(t, validCurrencies[result], "currency should be one of USD, EUR, GBP")
	}
}

func TestRandomEmail(t *testing.T) {
	result := RandomEmail()
	require.Contains(t, result, "@email.com")
	require.Greater(t, len(result), len("@email.com"))

	// Extract the prefix
	prefix := result[:len(result)-len("@email.com")]
	require.Len(t, prefix, 6)

	// Check that prefix only contains lowercase letters
	matched, err := regexp.MatchString(`^[a-z]+$`, prefix)
	require.NoError(t, err)
	require.True(t, matched)
}
