package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/banking/api-gateway/internal/config"
	"github.com/banking/api-gateway/internal/infrastructure"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type AuthMiddleware struct {
	cfg         *config.Config
	logger      *zap.Logger
	redisClient *infrastructure.RedisClient
}

func NewAuthMiddleware(cfg *config.Config, logger *zap.Logger, redisClient *infrastructure.RedisClient) *AuthMiddleware {
	return &AuthMiddleware{
		cfg:         cfg,
		logger:      logger,
		redisClient: redisClient,
	}
}

func (m *AuthMiddleware) ValidateToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Missing authorization header"})
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid authorization format"})
		}
		tokenString := parts[1]

		// Check Blacklist if Redis is available
		if m.redisClient != nil {
			isBlacklisted, err := m.redisClient.IsTokenBlacklisted(c.Request().Context(), tokenString)
			if err != nil {
				m.logger.Error("Failed to check token blacklist", zap.Error(err))
				// Fail open or closed? Security says closed, but availability says open.
				// Given strict security requirement: Increase security.
				// However, if Redis is down, we might block everyone.
				// Let's log and proceed, but for strict security, verify requirement.
				// "Make sure you can only increase the security and cannot reduce it."
				// "No functionalities can be removed."
				// If I block on Redis down, I remove functionality (availability).
				// So I should probably log and proceed, OR strictly fail.
				// I'll proceed for now, as blacklist is an enhancement.
			}
			if isBlacklisted {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Token has been revoked"})
			}
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validate Signing Method
			// For this implementation, we assume HMAC (HS256) for simplicity via Shared Secret.
			// Production should use RSA/ECDSA with Public Key.
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(m.cfg.Security.JWTSecret), nil
		})

		if err != nil {
			m.logger.Warn("Token validation failed", zap.Error(err))
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
		}

		if !token.Valid {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Token is invalid"})
		}

		// Extract Claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_claims", claims)
			if sub, ok := claims["sub"].(string); ok {
				c.Set("user_id", sub)
				// Log user for traceability
				m.logger.Debug("Request authenticated", zap.String("user_id", sub))
			}
		}

		return next(c)
	}
}
