# Build stage
FROM golang:1.26-alpine@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS build

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
FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

# Install CA certificates for HTTPS connections
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' vsg

# Copy the binary
COPY --from=build /app/vsg /usr/local/bin/vsg

# Use non-root user
USER vsg

# Set the entrypoint
ENTRYPOINT ["vsg"]
