# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git for dependencies if needed
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the application with security flags
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o api-gateway ./cmd/api

# Run Stage
FROM alpine:3.19

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -S appuser -u 1001 -G appgroup

WORKDIR /app

# Install CA certificates for HTTPS/TLS
RUN apk --no-cache add ca-certificates

# Copy binary with proper ownership
COPY --from=builder --chown=appuser:appgroup /app/api-gateway .

# Copy config.yaml with proper ownership
COPY --from=builder --chown=appuser:appgroup /app/config/config.yaml ./config/config.yaml

# Switch to non-root user
USER appuser

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./api-gateway"]
