package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/shehbaaz/db-utopia/internal/constants"
)

type ZoneAlertCache struct {
	redis *RedisClient
}

func NewZoneAlertCache(redis *RedisClient) *ZoneAlertCache {
	return &ZoneAlertCache{
		redis: redis,
	}
}

func (c *ZoneAlertCache) key(
	panelID,
	clientID,
	zoneNo string,
) string {

	return fmt.Sprintf(
		"%s:%s:%s:%s",
		constants.RedisZoneAlertCount,
		panelID,
		clientID,
		zoneNo,
	)
}

// Increment count for this zone.
// TTL automatically expires after 1 hour.
func (c *ZoneAlertCache) Increment(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo string,
) (int64, error) {

	if c == nil {
		return 0, fmt.Errorf("zone alert cache is nil")
	}

	if c.redis == nil {
		return 0, fmt.Errorf("redis client is nil")
	}

	if c.redis.Client == nil {
		return 0, fmt.Errorf("redis client connection is nil")
	}

	key := c.key(panelID, clientID, zoneNo)

	count, err := c.redis.Client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// Reset TTL on every alert (Sliding Window)
	if err := c.redis.Client.
		Expire(ctx, key, time.Hour).
		Err(); err != nil {
		return 0, err
	}

	return count, nil
}

func (c *ZoneAlertCache) GetCount(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo string,
) int64 {

	if c == nil || c.redis == nil || c.redis.Client == nil {
		return 0
	}

	key := c.key(panelID, clientID, zoneNo)

	count, err := c.redis.Client.Get(ctx, key).Int64()
	if err != nil {
		return 0
	}

	return count
}

func (c *ZoneAlertCache) Reset(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo string,
) error {

	if c == nil || c.redis == nil || c.redis.Client == nil {
		return fmt.Errorf("zone alert cache not initialized")
	}

	key := c.key(panelID, clientID, zoneNo)

	return c.redis.Client.Del(ctx, key).Err()
}
