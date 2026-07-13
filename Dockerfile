# Build stage
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build

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
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

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
