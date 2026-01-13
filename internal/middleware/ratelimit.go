package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/banking/api-gateway/internal/infrastructure"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type RateLimitConfig struct {
	Limit  int64
	Window time.Duration
}

type RateLimiter struct {
	redis  *infrastructure.RedisClient
	logger *zap.Logger
	// Default limits per endpoint category
	authLimit     RateLimitConfig
	transferLimit RateLimitConfig
	defaultLimit  RateLimitConfig
}

func NewRateLimiter(redis *infrastructure.RedisClient, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		logger: logger,
		authLimit: RateLimitConfig{
			Limit:  5,
			Window: 1 * time.Minute,
		},
		transferLimit: RateLimitConfig{
			Limit:  100,
			Window: 1 * time.Hour,
		},
		defaultLimit: RateLimitConfig{
			Limit:  1000,
			Window: 1 * time.Hour,
		},
	}
}

// RateLimitByIP creates middleware that limits by IP address.
// Used for public/auth endpoints.
func (r *RateLimiter) RateLimitByIP(cfg RateLimitConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			path := c.Path()
			key := fmt.Sprintf("ratelimit:ip:%s:%s", ip, path)

			return r.checkLimit(c, next, key, cfg)
		}
	}
}

// RateLimitByUser creates middleware that limits by authenticated user ID.
// Requires auth middleware to run first to populate user_id.
func (r *RateLimiter) RateLimitByUser(cfg RateLimitConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := c.Get("user_id").(string)
			if !ok || userID == "" {
				// Fallback to IP if user not authenticated
				userID = c.RealIP()
			}
			path := c.Path()
			key := fmt.Sprintf("ratelimit:user:%s:%s", userID, path)

			return r.checkLimit(c, next, key, cfg)
		}
	}
}

// AuthRateLimiter returns middleware configured for auth endpoints (5/min by IP).
func (r *RateLimiter) AuthRateLimiter() echo.MiddlewareFunc {
	return r.RateLimitByIP(r.authLimit)
}

// TransferRateLimiter returns middleware configured for transfer endpoints (100/hr by user).
func (r *RateLimiter) TransferRateLimiter() echo.MiddlewareFunc {
	return r.RateLimitByUser(r.transferLimit)
}

// DefaultRateLimiter returns middleware for general endpoints (1000/hr by user).
func (r *RateLimiter) DefaultRateLimiter() echo.MiddlewareFunc {
	return r.RateLimitByUser(r.defaultLimit)
}

func (r *RateLimiter) checkLimit(c echo.Context, next echo.HandlerFunc, key string, cfg RateLimitConfig) error {
	ctx := c.Request().Context()

	count, err := r.redis.IncrementWithExpiry(ctx, key, cfg.Window)
	if err != nil {
		r.logger.Error("Rate limiter Redis error", zap.Error(err))
		// Fail open: allow request if Redis is down (graceful degradation)
		return next(c)
	}

	// Set rate limit headers
	c.Response().Header().Set("X-RateLimit-Limit", strconv.FormatInt(cfg.Limit, 10))
	c.Response().Header().Set("X-RateLimit-Remaining", strconv.FormatInt(max(0, cfg.Limit-count), 10))

	if count > cfg.Limit {
		ttl, _ := r.redis.TTL(ctx, key)
		retryAfter := int(ttl.Seconds())
		if retryAfter <= 0 {
			retryAfter = int(cfg.Window.Seconds())
		}

		c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))

		r.logger.Warn("Rate limit exceeded",
			zap.String("key", key),
			zap.Int64("count", count),
			zap.Int64("limit", cfg.Limit),
		)

		return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
			"error":       "Rate limit exceeded",
			"retry_after": retryAfter,
		})
	}

	return next(c)
}
