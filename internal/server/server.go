package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/banking/api-gateway/internal/config"
	"github.com/banking/api-gateway/internal/infrastructure"
	"github.com/banking/api-gateway/internal/middleware"
	"github.com/banking/api-gateway/internal/proxy"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

type Server struct {
	echo        *echo.Echo
	cfg         *config.Config
	logger      *zap.Logger
	redisClient *infrastructure.RedisClient
}

func New(cfg *config.Config, logger *zap.Logger, redisClient *infrastructure.RedisClient) *Server {
	e := echo.New()
	e.HideBanner = true

	// Standard Middleware
	e.Use(echoMiddleware.Recover())
	e.Use(echoMiddleware.RequestID())
	e.Use(echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins: cfg.Cors.AllowOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
	}))

	// Security Middleware
	e.Use(echoMiddleware.Secure())
	e.Use(echoMiddleware.BodyLimit("2M"))

	// Structured Logging
	e.Use(echoMiddleware.RequestLoggerWithConfig(echoMiddleware.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogMethod:  true,
		LogLatency: true,
		LogValuesFunc: func(c echo.Context, v echoMiddleware.RequestLoggerValues) error {
			logger.Info("request",
				zap.String("URI", v.URI),
				zap.Int("status", v.Status),
				zap.String("method", v.Method),
				zap.Duration("latency", v.Latency),
			)
			return nil
		},
	}))

	return &Server{
		echo:        e,
		cfg:         cfg,
		logger:      logger,
		redisClient: redisClient,
	}
}

func (s *Server) Start() error {
	s.setupRoutes()

	serverUrl := fmt.Sprintf(":%s", s.cfg.Server.Port)
	s.logger.Info("Starting API Gateway", zap.String("url", serverUrl))

	// Configure Server Timeouts
	s.echo.Server.ReadTimeout = s.cfg.Server.ReadTimeout
	s.echo.Server.WriteTimeout = s.cfg.Server.WriteTimeout
	s.echo.Server.IdleTimeout = 120 * time.Second
	s.echo.Server.MaxHeaderBytes = 1 << 20 // 1MB

	return s.echo.Start(serverUrl)
}

func (s *Server) Stop(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

func (s *Server) setupRoutes() {
	// Health Check
	s.echo.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "UP"})
	})

	// Auth Middleware - Inject Redis Client
	authMiddleware := middleware.NewAuthMiddleware(s.cfg, s.logger, s.redisClient)

	// Rate Limiter (gracefully degrades if Redis is nil)
	var rateLimiter *middleware.RateLimiter
	if s.redisClient != nil {
		rateLimiter = middleware.NewRateLimiter(s.redisClient, s.logger)
	}

	// Proxy Handler with Circuit Breaker
	proxyHandler := proxy.NewProxyHandler(s.cfg, s.logger)

	apiGroup := s.echo.Group("/api")

	// Auth Service Routes (Public, with IP-based rate limiting)
	authRoutes := apiGroup.Group("/auth")
	if rateLimiter != nil {
		authRoutes.Use(rateLimiter.AuthRateLimiter())
	}
	authRoutes.Any("/*", proxyHandler.Handle("auth-service"))

	// Protected Routes
	protected := apiGroup.Group("")
	protected.Use(authMiddleware.ValidateToken)

	// Transfer routes with stricter rate limiting
	transferRoutes := protected.Group("/transfers")
	if rateLimiter != nil {
		transferRoutes.Use(rateLimiter.TransferRateLimiter())
	}
	transferRoutes.Any("/*", proxyHandler.Handle("transaction-service"))

	// Other protected routes with default rate limiting
	if rateLimiter != nil {
		protected.Use(rateLimiter.DefaultRateLimiter())
	}
	protected.Any("/users/*", proxyHandler.Handle("user-service"))
	protected.Any("/reporting/*", proxyHandler.Handle("reporting-service"))
	protected.Any("/aml/*", proxyHandler.Handle("aml-service"))
}
