package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shehbaaz/db-utopia/internal/logger"
)

type RedisClient struct {
	Client *redis.Client
}

func NewRedisClient(host, port, password string, db int) *RedisClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:         host + ":" + port,
		Password:     password,
		DB:           db,
		PoolSize:     5, // production tuned
		MinIdleConns: 1,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	res := rdb.Ping(context.Background())
	if res.Err() != nil {
		panic("Failed to connect to Redis: " + res.Err().Error())
	}

	return &RedisClient{Client: rdb}
}

func (r *RedisClient) Close() error {
	return r.Client.Close()
}

// DeleteKeysByPrefix deletes all Redis keys matching the prefix

func (r *RedisClient) DeleteKeysByPrefix(
	ctx context.Context,
) error {
	var prefix = "tcp_alert"

	var cursor uint64

	for {
		keys, nextCursor, err := r.Client.Scan(
			ctx,
			cursor,
			prefix+"*",
			1000,
		).Result()

		if err != nil {
			return err
		}

		if len(keys) > 0 {
			// Faster and non-blocking
			if err := r.Client.Unlink(ctx, keys...).Err(); err != nil {
				return err
			}

			logger.Infof(
				"Deleted %d keys for prefix=%s",
				len(keys),
				prefix,
			)
		}

		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return nil
}

func makeKey(panelID, clientID string) string {
	return panelID + "|" + clientID
}
