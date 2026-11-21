package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shehbaaz/db-utopia/internal/constants"
	"github.com/shehbaaz/db-utopia/internal/models"
)

type PanelZoneDataCache struct {
	redis *RedisClient
}

func NewPanelZoneDataCache(redis *RedisClient) *PanelZoneDataCache {
	return &PanelZoneDataCache{
		redis: redis,
	}
}

func (c *PanelZoneDataCache) key(
	panelID,
	clientID string,
) string {

	return fmt.Sprintf(
		"%s:%s:%s",
		constants.RedisZoneData,
		panelID,
		clientID,
	)
}

func (c *PanelZoneDataCache) Get(
	ctx context.Context,
	panelID,
	clientID string,
) (
	map[string]models.ZoneConfig,
	bool,
	error,
) {

	key := c.key(
		panelID,
		clientID,
	)

	val, err := c.redis.Client.Get(
		ctx,
		key,
	).Result()

	if err != nil {
		return nil, false, nil
	}

	var cfg map[string]models.ZoneConfig

	if err := json.Unmarshal(
		[]byte(val),
		&cfg,
	); err != nil {

		return nil, false, err
	}

	return cfg, true, nil
}

func (c *PanelZoneDataCache) Set(
	ctx context.Context,
	panelID,
	clientID string,
	cfg map[string]models.ZoneConfig,
) error {

	key := c.key(
		panelID,
		clientID,
	)

	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return c.redis.Client.Set(
		ctx,
		key,
		b,
		24*time.Hour,
	).Err()
}
