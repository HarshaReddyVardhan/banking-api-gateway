package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/banking/api-gateway/internal/config"
	"github.com/labstack/echo/v4"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

type ProxyHandler struct {
	cfg      *config.Config
	logger   *zap.Logger
	breakers map[string]*gobreaker.CircuitBreaker
	mu       sync.RWMutex
}

func NewProxyHandler(cfg *config.Config, logger *zap.Logger) *ProxyHandler {
	handler := &ProxyHandler{
		cfg:      cfg,
		logger:   logger,
		breakers: make(map[string]*gobreaker.CircuitBreaker),
	}

	// Initialize circuit breakers for each service
	for name, svc := range cfg.Services {
		if svc.CircuitBreaker {
			handler.breakers[name] = handler.createCircuitBreaker(name)
		}
	}

	return handler
}

func (h *ProxyHandler) createCircuitBreaker(serviceName string) *gobreaker.CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        serviceName,
		MaxRequests: 5,                // Requests allowed in half-open state
		Interval:    10 * time.Second, // Reset failure count interval
		Timeout:     30 * time.Second, // Time in open state before half-open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Open circuit after 5 consecutive failures
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			h.logger.Warn("Circuit breaker state changed",
				zap.String("service", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		},
	}
	return gobreaker.NewCircuitBreaker(settings)
}

func (h *ProxyHandler) Handle(serviceName string) echo.HandlerFunc {
	return func(c echo.Context) error {
		svcConfig, ok := h.cfg.Services[serviceName]
		if !ok {
			h.logger.Error("Service configuration not found", zap.String("service", serviceName))
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Service not configured"})
		}

		targetURL, err := url.Parse(svcConfig.URL)
		if err != nil {
			h.logger.Error("Invalid service URL", zap.String("service", serviceName), zap.String("url", svcConfig.URL), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Configuration error"})
		}

		// Get circuit breaker if enabled for this service
		h.mu.RLock()
		cb, hasBreaker := h.breakers[serviceName]
		h.mu.RUnlock()

		if hasBreaker {
			// Execute request through circuit breaker
			_, err := cb.Execute(func() (interface{}, error) {
				return nil, h.doProxy(c, targetURL, serviceName)
			})

			if err != nil {
				if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
					h.logger.Warn("Circuit breaker open",
						zap.String("service", serviceName),
						zap.String("state", cb.State().String()),
					)
					return c.JSON(http.StatusServiceUnavailable, map[string]string{
						"error":   "Service temporarily unavailable",
						"service": serviceName,
					})
				}
				// Proxy error already handled in doProxy
				return nil
			}
			return nil
		}

		// No circuit breaker, direct proxy
		return h.doProxy(c, targetURL, serviceName)
	}
}

func (h *ProxyHandler) doProxy(c echo.Context, targetURL *url.URL, serviceName string) error {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Optimize Transport
	proxy.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	var proxyErr error

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Path Rewriting: Strip /api prefix
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api")
		if req.URL.Path == "" || !strings.HasPrefix(req.URL.Path, "/") {
			req.URL.Path = "/" + req.URL.Path
		}

		req.Host = targetURL.Host

		// Propagate Tracing Headers
		if traceID := c.Request().Header.Get("X-Request-ID"); traceID != "" {
			req.Header.Set("X-Request-ID", traceID)
		}
		// Forward user ID for backend authorization if present
		if userID, ok := c.Get("user_id").(string); ok && userID != "" {
			req.Header.Set("X-User-ID", userID)
		}
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.logger.Error("Proxy forwarding error", zap.String("service", serviceName), zap.Error(err))
		proxyErr = err

		// Return JSON error response check
		if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"error":"Service Unavailable"}`)
		}
	}

	proxy.ServeHTTP(c.Response(), c.Request())
	return proxyErr
}
