# Build stage
FROM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build

WORKDIR /app

# Install git for version info
RUN apk add --no-cache git

# Download dependencies first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
        -X github.com/pavlenkoa/vault-secrets-generator/internal/command.Version=${VERSION} \
        -X github.com/pavlenkoa/vault-secrets-generator/internal/command.Commit=${COMMIT} \
        -X github.com/pavlenkoa/vault-secrets-generator/internal/command.BuildDate=${BUILD_DATE}" \
    -o vsg .

# Final stage
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# Install CA certificates for HTTPS connections
RUN apk add --no-cache ca-certificates tzdata openssl

# Create non-root user
RUN adduser -D -g '' vsg

# Copy the binary
COPY --from=build /app/vsg /usr/local/bin/vsg

# Use non-root user
USER vsg

# Set the entrypoint
ENTRYPOINT ["vsg"]
