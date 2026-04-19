# Build stage
FROM golang:1.26-alpine@sha256:f85330846cde1e57ca9ec309382da3b8e6ae3ab943d2739500e08c86393a21b1 AS build

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
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

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
