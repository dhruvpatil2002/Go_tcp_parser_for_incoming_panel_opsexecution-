package redis

import (
	"context"
	"fmt"

	"github.com/shehbaaz/db-utopia/internal/constants"
)

type PanelZoneStateCache struct {
	redis *RedisClient
}

func NewPanelZoneStateCache(
	redis *RedisClient,
) *PanelZoneStateCache {

	return &PanelZoneStateCache{
		redis: redis,
	}
}

func (c *PanelZoneStateCache) key(
	panelID,
	clientID string,
) string {

	return fmt.Sprintf(
		"%s:%s:%s",
		constants.RedisZoneState,
		panelID,
		clientID,
	)
}

func (c *PanelZoneStateCache) Get(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo string,
) (string, error) {

	return c.redis.Client.HGet(
		ctx,
		c.key(panelID, clientID),
		zoneNo,
	).Result()
}

func (c *PanelZoneStateCache) Exists(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo string,
) (bool, error) {

	return c.redis.Client.HExists(
		ctx,
		c.key(panelID, clientID),
		zoneNo,
	).Result()
}

func (c *PanelZoneStateCache) Set(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo,
	alarmCode string,
) error {

	return c.redis.Client.HSet(
		ctx,
		c.key(panelID, clientID),
		zoneNo,
		alarmCode,
	).Err()
}

func (c *PanelZoneStateCache) Remove(
	ctx context.Context,
	panelID,
	clientID,
	zoneNo string,
) error {

	return c.redis.Client.HDel(
		ctx,
		c.key(panelID, clientID),
		zoneNo,
	).Err()
}
