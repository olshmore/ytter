package canceltoken

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHash_lengthAndStability(t *testing.T) {
	t.Parallel()
	h := Hash("secret-token")
	require.Len(t, h, 64)
	require.Equal(t, h, Hash("secret-token"))
	require.NotEqual(t, h, Hash("other"))
}
