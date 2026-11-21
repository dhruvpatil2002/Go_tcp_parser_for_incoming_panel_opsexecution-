package redis

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/constants"
	"github.com/shehbaaz/db-utopia/internal/db"
	"golang.org/x/sync/singleflight"
)

type panelEntry struct {
	data      map[string]string
	expiresAt time.Time
}
type PanelSensorCache struct {
	mu    sync.RWMutex
	data  map[string]*panelEntry
	group singleflight.Group
	ttl   time.Duration
	redis *RedisClient
	db    *pgxpool.Pool
}

func NewPanelSensorCache(ttl time.Duration, redis *RedisClient, db *pgxpool.Pool) *PanelSensorCache {
	return &PanelSensorCache{
		data:  make(map[string]*panelEntry),
		ttl:   ttl,
		redis: redis,
		db:    db,
	}
}

func (c *PanelSensorCache) setLocal(panelPK string, data map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[panelPK] = &panelEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
}
func (c *PanelSensorCache) Get(panelPK string) (map[string]string, error) {

	// 1. Local cache
	c.mu.RLock()
	if entry, ok := c.data[panelPK]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.data, nil
	}
	c.mu.RUnlock()

	// 2. Singleflight
	val, err, _ := c.group.Do(panelPK, func() (interface{}, error) {

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 2.1 Redis
		redisData, err := c.redis.GetPanel(ctx, panelPK)
		if err == nil && redisData != nil {
			c.setLocal(panelPK, redisData)
			return redisData, nil
		}

		// 2.2 DB
		dbData, err := db.GetPanelWithSensorFromDB(ctx, c.db, panelPK)
		if err != nil {
			return nil, err
		}

		// 2.3 Save to Redis
		_ = c.redis.SetPanel(ctx, panelPK, dbData, c.ttl)

		// 2.4 Save to local
		c.setLocal(panelPK, dbData)

		return dbData, nil
	})

	if err != nil {
		return nil, err
	}

	return val.(map[string]string), nil
}
func (r *RedisClient) GetPanel(ctx context.Context, panelPK string) (map[string]string, error) {
	key := constants.RedisPanelPrefix + panelPK

	result, err := r.Client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	return result, nil
}

func (r *RedisClient) SetPanel(ctx context.Context, panelPK string, data map[string]string, ttl time.Duration) error {
	key := constants.RedisPanelPrefix + panelPK

	if len(data) == 0 {
		return nil
	}

	if err := r.Client.HSet(ctx, key, data).Err(); err != nil {
		return err
	}

	return r.Client.Expire(ctx, key, ttl).Err()
}

func (r *RedisClient) DeletePanel(ctx context.Context, panelPK string) {
	r.Client.Del(ctx, constants.RedisPanelPrefix+panelPK)
}
