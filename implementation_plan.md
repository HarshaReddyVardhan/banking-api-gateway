# API Gateway Implementation Plan

## Overview
Implement a high-performance Banking API Gateway in Go. This service acts as the central entry point for all client requests, handling routing, authentication, rate limiting, and security enforcement.

## Architecture
- **Language**: Go (Golang) 1.22+
- **Framework**: Echo (consistent with other services) or standard `net/http` with middleware chain. Echo is preferred for its robust middleware support.
- **Data Store**: Redis (for Rate Limiting & Token Blacklist/Caching).
- **Service Discovery**: Consul (integration logic).
- **Communication**: HTTP/1.1 & HTTP/2 (backend), WebSocket (via separate component, or passthrough).

## Core Components

### 1. Project Initialization
- [ ] Initialize `go.mod` (`github.com/banking/api-gateway`)
- [ ] Create directory structure (`cmd/api`, `internal/config`, `internal/server`, `internal/middleware`, `internal/proxy`)

### 2. Configuration Management
- [ ] Implement `internal/config`: Load settings for Server, Redis, Consul, Service Routes (using Viper).

### 3. Core Server & Routing
- [ ] Setup Echo server in `internal/server`.
- [ ] Implement Dynamic Reverse Proxy in `internal/proxy`.
    - Route `/api/transfers` -> Transaction Service
    - Route `/api/auth` -> Auth Service
    - Route `/api/users` -> User Service
    - Route `/api/reporting` -> Reporting Service
    - Route `/api/aml` -> AML Service

### 4. Security Middleware (Layer 2 & 3 & 4)
- [ ] **Input Sanitization**: Middleware to sanitize headers and body.
- [ ] **Authentication**: JWT Validation Middleware.
    - Validate signature (RSA/HMAC).
    - Check Expiration.
    - Check Audience.
    - Cache public keys/validation results in Redis.
- [ ] **Authorization (RBAC)**: Check scopes/roles against route requirements.
- [ ] **Rate Limiting (Layer 5)**: Redis-backed sliding window or token bucket.
    - Per User, Per IP, Per Endpoint.
- [ ] **CORS & Helmets**: Security headers (HSTS, X-Frame-Options, etc.).

### 5. Resiliency & Observability
- [ ] **Circuit Breaker**: Implement per-service circuit breakers (using `gobreaker`).
- [ ] **Logging**: Structured logging with Zap.
- [ ] **Metrics**: Prometheus middleware.
- [ ] **Tracing**: OpenTelemetry integration.

### 6. Infrastructure
- [ ] Dockerfile (Multi-stage build).
- [ ] `docker-compose.yaml` (Dependencies: Redis, Consul).

## Step-by-Step Implementation

1. **Setup**: Init module and folders.
2. **Config**: Create configuration loader.
3. **Basic Proxy**: Implement a simple pass-through proxy.
4. **Auth & Security**: Add JWT and Rate Limiting middleware.
5. **Advanced Routing**: Add Service Discovery and Circuit Breakers.
6. **Dockerize**: Create deployment artifacts.

