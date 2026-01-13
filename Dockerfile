# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git for dependencies if needed (though we use go modules)
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o api-gateway ./cmd/api

# Run Stage
FROM alpine:latest

WORKDIR /app

# Install CA certificates for HTTPS/TLS
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/api-gateway .
# Copy config.yaml if you want it embedded, or rely on volume mounts.
# We copy it for default behavior.
COPY --from=builder /app/config/config.yaml ./config/config.yaml
# Ensure structure
RUN mkdir -p config

EXPOSE 8080

CMD ["./api-gateway"]
