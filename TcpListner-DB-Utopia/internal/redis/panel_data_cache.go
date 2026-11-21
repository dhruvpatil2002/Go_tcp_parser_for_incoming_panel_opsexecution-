package redis

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shehbaaz/db-utopia/internal/constants"
	"github.com/shehbaaz/db-utopia/internal/models"
	"golang.org/x/sync/singleflight"
)

type PanelDataCache struct {
	mu    sync.RWMutex
	data  map[string]*models.PanelData // key = panelID|clientID
	group singleflight.Group

	redis *RedisClient
}

func NewPanelDataCache(redis *RedisClient) *PanelDataCache {
	return &PanelDataCache{
		data:  make(map[string]*models.PanelData),
		redis: redis,
	}
}

func (c *PanelDataCache) setLocal(key string, value *models.PanelData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = value
}

func (r *RedisClient) GetPanelData(ctx context.Context, key string) (*models.PanelData, error) {
	val, err := r.Client.Get(ctx, constants.RedisPanelDataPrefix+key).Result()
	if err == redis.Nil { 
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var data models.PanelData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func (r *RedisClient) SetPanelData(ctx context.Context, key string, value *models.PanelData) error {
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.Client.Set(ctx, constants.RedisPanelDataPrefix+key, val, time.Hour).Err() // no expiry
}

func (c *PanelDataCache) Get(
	ctx context.Context,
	panelID, clientID string,
	fetch func(context.Context, string, string) (*models.PanelData, error),
) (*models.PanelData, error) {

	key := makeKey(panelID, clientID)

	// 🔹 1. LOCAL CACHE (optional but fast)
	c.mu.RLock()
	if val, ok := c.data[key]; ok {
		c.mu.RUnlock()
		return val, nil
	}
	c.mu.RUnlock()

	// 🔹 2. SINGLEFLIGHT
	val, err, _ := c.group.Do(key, func() (interface{}, error) {

		// 🔹 2.1 REDIS
		panelData, err := c.redis.GetPanelData(ctx, key)
		if err == nil && panelData != nil {
			c.setLocal(key, panelData)
			return panelData, nil
		}

		// 🔹 2.2 DB FETCH
		panelData, err = fetch(ctx, panelID, clientID)
		if err != nil {
			return nil, err
		}

		// 🔹 2.3 SAVE (ONLY IF VALID)
		if panelData != nil {
			_ = c.redis.SetPanelData(ctx, key, panelData)
			c.setLocal(key, panelData)
		}

		return panelData, nil
	})

	if err != nil {
		return nil, err
	}

	result, _ := val.(*models.PanelData)
	return result, nil
}
