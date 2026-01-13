package infrastructure

import (
	"context"
	"time"

	"github.com/banking/api-gateway/internal/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisClient struct {
	client *redis.Client
	logger *zap.Logger
}

func NewRedisClient(cfg *config.RedisConfig, logger *zap.Logger) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Error("Failed to connect to Redis", zap.Error(err))
		return nil, err
	}

	logger.Info("Redis connection established", zap.String("address", cfg.Address))

	return &RedisClient{
		client: client,
		logger: logger,
	}, nil
}

// IncrementWithExpiry increments a key and sets expiry ONLY if it's the new key (count == 1).
// This ensures a fixed window rate limiting strategy.
func (r *RedisClient) IncrementWithExpiry(ctx context.Context, key string, window time.Duration) (int64, error) {
	script := `
		local current = redis.call("INCR", KEYS[1])
		if current == 1 then
			redis.call("EXPIRE", KEYS[1], ARGV[1])
		end
		return current
	`
	// Redis expects expiration in seconds for EXPIRE command
	seconds := int(window.Seconds())
	if seconds == 0 {
		seconds = 1 // Ensure at least 1 second if window is very small
	}

	result, err := r.client.Eval(ctx, script, []string{key}, seconds).Int64()
	if err != nil {
		return 0, err
	}

	return result, nil
}

// GetCount returns the current count for a key.
func (r *RedisClient) GetCount(ctx context.Context, key string) (int64, error) {
	val, err := r.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// TTL returns the remaining time-to-live for a key.
func (r *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, key).Result()
}

// IsTokenBlacklisted checks if a token (JTI or full token hash) is in the blacklist.
func (r *RedisClient) IsTokenBlacklisted(ctx context.Context, tokenIdentifier string) (bool, error) {
	exists, err := r.client.Exists(ctx, "blacklist:"+tokenIdentifier).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// BlacklistToken adds a token to the blacklist with an expiration.
func (r *RedisClient) BlacklistToken(ctx context.Context, tokenIdentifier string, duration time.Duration) error {
	return r.client.Set(ctx, "blacklist:"+tokenIdentifier, "revoked", duration).Err()
}

// Close closes the Redis connection.
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// HealthCheck pings Redis to check connection health.
func (r *RedisClient) HealthCheck(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
