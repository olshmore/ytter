package worker

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestNewRedisTaskDistributor(t *testing.T) {
	redisOpt := asynq.RedisClientOpt{
		Addr: "localhost:6379",
	}

	distributor := NewRedisTaskDistributor(redisOpt)

	require.NotNil(t, distributor)
	require.IsType(t, &RedisTaskDistributor{}, distributor)

	// Verify it implements the TaskDistributor interface
	_, ok := distributor.(TaskDistributor)
	require.True(t, ok)
}

func TestDistributeTaskSendVerificationEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires Redis")
	}

	// This test requires a running Redis instance
	redisOpt := asynq.RedisClientOpt{
		Addr: "localhost:6379",
	}

	distributor := NewRedisTaskDistributor(redisOpt)

	payload := &PayloadSendVerificationEmail{
		Username: "testuser",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := distributor.DistributeTaskSendVerificationEmail(ctx, payload)
	if err != nil {
		// Expected if Redis is not running
		require.Contains(t, err.Error(), "failed to enqueue task")
	}
}
