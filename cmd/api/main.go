package main

import (
	"log"

	"github.com/banking/api-gateway/internal/config"
	"github.com/banking/api-gateway/internal/infrastructure"
	"github.com/banking/api-gateway/internal/server"
	"go.uber.org/zap"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Initialize Logger
	var logger *zap.Logger
	if cfg.Server.Environment == "development" {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	logger.Info("Initializing Banking API Gateway",
		zap.String("version", "1.0.0"),
		zap.String("environment", cfg.Server.Environment),
	)

	// 3. Initialize Redis (optional, graceful degradation if unavailable)
	var redisClient *infrastructure.RedisClient
	redisClient, err = infrastructure.NewRedisClient(&cfg.Redis, logger)
	if err != nil {
		logger.Warn("Redis connection failed, rate limiting disabled", zap.Error(err))
		redisClient = nil
	}

	// 4. Create Server
	srv := server.New(cfg, logger, redisClient)

	// 5. Start Server
	if err := srv.Start(); err != nil {
		logger.Fatal("Server start failed", zap.Error(err))
	}
}
