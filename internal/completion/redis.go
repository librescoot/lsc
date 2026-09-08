package completion

import (
	"context"
	"time"

	redisclient "librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

const redisTimeout = 750 * time.Millisecond

// WithRedis runs a bounded read-only lookup for shell completion. Completion
// bypasses the root command's normal Redis connection so static completions
// remain available when the scooter is offline.
func WithRedis(cmd *cobra.Command, lookup func(context.Context, *redisclient.Client) ([]string, error)) []string {
	addr := "192.168.7.1:6379"
	if flag := cmd.Root().Flag("redis-addr"); flag != nil {
		addr = flag.Value.String()
	}

	client := redisclient.NewClient(addr)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()

	values, err := lookup(ctx, client)
	if err != nil {
		return nil
	}
	return values
}
