# VSG - Vault Secrets Generator

## Project Overview

A lightweight, cloud-agnostic CLI tool that generates and populates secrets in HashiCorp Vault from various sources including Terraform state files, generated passwords, and static values.

**Primary use cases:**
- Extract outputs from Terraform state files (S3, GCS, local) and write them to Vault
- Generate secrets with configurable password policies
- Copy/sync secrets between Vault paths
- Declarative YAML configuration for GitOps workflows

## Tech Stack

- **Language:** Go 1.22+
- **Target:** Single static binary, minimal dependencies
- **Container:** Alpine-based, <20MB final image

## Project Structure

```
vault-secrets-generator/
├── main.go                         # CLI entrypoint using cobra
├── internal/
│   ├── command/                    # CLI command implementations
│   │   ├── root.go                 # Root command and global flags
│   │   ├── apply.go                # Apply command
│   │   ├── diff.go                 # Diff command
│   │   └── version.go              # Version command
│   ├── config/
│   │   ├── config.go               # YAML config parsing
│   │   ├── types.go                # Config structs
│   │   └── variables.go            # Variable substitution ({env}, etc.)
│   ├── fetcher/
│   │   ├── fetcher.go              # Fetcher interface
│   │   ├── s3.go                   # S3 backend
│   │   ├── gcs.go                  # GCS backend
│   │   └── local.go                # Local file backend
│   ├── tfstate/
│   │   ├── parser.go               # Terraform state JSON parser
│   │   └── testdata/               # Test fixtures for tfstate
│   │       └── terraform.tfstate
│   ├── generator/
│   │   └── password.go             # Password generation with policies
│   ├── vault/
│   │   ├── client.go               # Vault client wrapper
│   │   └── writer.go               # KV v1/v2 write operations
│   └── engine/
│       ├── reconcile.go            # Main reconciliation logic
│       └── diff.go                 # Diff/dry-run logic
├── helm/
│   └── vault-secrets-generator/    # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── examples/
│   └── config.yaml                 # Example configuration file
├── docs/
│   └── configuration.md            # Detailed config documentation
├── .github/
│   └── workflows/                  # CI/CD workflows
├── .goreleaser.yaml                # Release automation
├── .golangci.yaml                  # Linter configuration
├── Makefile                        # Build, test, lint targets
├── Dockerfile                      # Multi-stage build
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── CLAUDE.md
```

## Configuration Format

The tool uses a declarative YAML configuration:

```yaml
# Variables available for substitution throughout the config
env:
  env: dev
  region: eu-west-1
  project: myapp

# Vault connection settings
vault:
  address: https://vault.example.com
  # Auth method: token, kubernetes, approle
  auth:
    method: kubernetes
    role: secret-sync

# Default password generation policy
defaults:
  generate:
    length: 32
    digits: 5
    symbols: 5
    symbolCharacters: "-_$@"
    noUpper: false
    allowRepeat: true

# Secrets definitions grouped by Vault path
secrets:
  main:
    path: kv/{env}
    data:
      # From Terraform state - full S3 URI with output path after #
      mysql/host: s3://terraform-state/{env}/rds/terraform.tfstate#output.endpoint
      mysql/password: s3://terraform-state/{env}/rds/terraform.tfstate#output.admin_password
      mysql/port: s3://terraform-state/{env}/rds/terraform.tfstate#output.port
      
      # Generated passwords - use defaults
      app/api_key: generate
      
      # Generated with custom policy
      app/jwt_secret: generate(length=64)
      app/webhook_token: generate(length=48, symbols=0)
      
      # Static values
      app/environment: "production"
      app/version: "1.2.3"

  docker:
    path: kv/{env}-docker
    data:
      registry/url: s3://terraform-state/{env}/ecr/terraform.tfstate#output.registry_url
      registry/username: s3://terraform-state/{env}/ecr/terraform.tfstate#output.username

  # Override Vault KV version for specific block
  legacy:
    path: kv/{env}-legacy
    version: 1
    data:
      old/token: generate(length=24, symbols=0)
```

## Value Types

The tool must recognize and handle these value patterns:

1. **Terraform State Reference:**
   ```
   s3://bucket/path/to/terraform.tfstate#output.some_output
   gcs://bucket/path/to/terraform.tfstate#output.some_output
   file:///path/to/terraform.tfstate#output.some_output
   ```
   Format: `<backend>://<path>#output.<output_name>`
   
   For nested outputs: `#output.module.rds.endpoint`

2. **Generated Password (default policy):**
   ```
   generate
   ```

3. **Generated Password (custom policy):**
   ```
   generate(length=64)
   generate(length=32, symbols=0)
   generate(length=48, digits=10, noUpper=true)
   ```
   Supported parameters: `length`, `digits`, `symbols`, `symbolCharacters`, `noUpper`, `allowRepeat`

4. **Static Value:**
   ```
   "any string that doesn't match above patterns"
   ```

## CLI Interface

```bash
# Run apply with config file
vsg apply --config config.yaml

# Dry-run mode - show what would change without making changes
vsg apply --config config.yaml --dry-run

# Show diff between current and desired state
vsg diff --config config.yaml

# Apply only specific secret block
vsg apply --config config.yaml --only main

# Apply single secret
vsg apply --config config.yaml --only main --key mysql/host

# Verbose output
vsg apply --config config.yaml -v

# Output format
vsg diff --config config.yaml --output json
vsg diff --config config.yaml --output yaml

# Version
vsg version
```

### Exit Codes

- `0` - Success, all secrets synced
- `1` - Configuration error
- `2` - Vault connection/auth error
- `3` - State file fetch error
- `4` - Partial failure (some secrets failed)

## Core Logic

### Reconciliation Flow

1. Parse and validate config YAML
2. Substitute variables (`{env}`, `{region}`, etc.)
3. For each secret block:
   a. Connect to Vault at specified path
   b. Read current secrets (if exist)
   c. For each secret in data:
      - If terraform reference: fetch state, extract output
      - If generate: generate password (only if secret doesn't exist OR --force)
      - If static: use value as-is
   d. Compare current vs desired
   e. Write changes to Vault (unless --dry-run)
4. Report results

### Important Behaviors

1. **Idempotency for generated secrets:**
   - Generated passwords should only be created if the secret doesn't exist in Vault
   - Use `--force` flag to regenerate existing secrets
   - This prevents password rotation on every run

2. **Terraform state caching:**
   - Cache fetched state files during a single run
   - Multiple secrets from same state file = one fetch

3. **Vault KV version detection:**
   - Auto-detect v1 vs v2 if not specified
   - Allow explicit override per block

4. **Error handling:**
   - Continue on individual secret failures (unless --fail-fast)
   - Report all errors at end
   - Detailed error messages with context

## Fetcher Interface

```go
type Fetcher interface {
    // Fetch retrieves the terraform state file and returns its contents
    Fetch(ctx context.Context, uri string) ([]byte, error)
    
    // Supports returns true if this fetcher handles the given URI scheme
    Supports(uri string) bool
}
```

Implementations needed:
- `s3://` - AWS S3 (use AWS SDK, support IRSA and standard credential chain)
- `gcs://` - Google Cloud Storage
- `file://` - Local filesystem

## Terraform State Parser

Terraform state is JSON. Need to extract outputs from:

```json
{
  "version": 4,
  "outputs": {
    "endpoint": {
      "value": "mydb.123456.us-east-1.rds.amazonaws.com",
      "type": "string"
    }
  }
}
```

Also handle module outputs:
```json
{
  "outputs": {},
  "resources": [...],
  "child_modules": [
    {
      "address": "module.rds",
      "outputs": {
        "endpoint": {"value": "..."}
      }
    }
  ]
}
```

The path `output.module.rds.endpoint` should navigate to child module outputs.

## Password Generator

Based on External Secrets Operator spec:

```go
type PasswordPolicy struct {
    Length           int    // Total length (default: 32)
    Digits           int    // Minimum digits (default: 5)
    Symbols          int    // Minimum symbols (default: 5)
    SymbolCharacters string // Allowed symbols (default: "-_$@")
    NoUpper          bool   // Exclude uppercase (default: false)
    AllowRepeat      bool   // Allow repeated characters (default: true)
}
```

Use `crypto/rand` for secure random generation.

## Vault Client

Support authentication methods:
1. **Token** - from env `VAULT_TOKEN` or config
2. **Kubernetes** - service account JWT auth
3. **AppRole** - role_id/secret_id

Handle both KV v1 and v2:
- v1: `PUT /secret/path` with `{"key": "value"}`
- v2: `PUT /secret/data/path` with `{"data": {"key": "value"}}`

## Environment Variables

```bash
VAULT_ADDR          # Vault server address
VAULT_TOKEN         # Vault token (for token auth)
VAULT_NAMESPACE     # Vault namespace (enterprise)
AWS_REGION          # AWS region for S3
AWS_PROFILE         # AWS profile (optional)
GOOGLE_APPLICATION_CREDENTIALS  # GCP service account (for GCS)
VSG_CONFIG          # Default config file path
```

## Dockerfile

```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o vsg ./cmd/vsg

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /app/vsg /usr/local/bin/
ENTRYPOINT ["vsg"]
```

## Testing Strategy

1. **Unit tests** for each package
2. **Integration tests** with local Vault dev server
3. **Mock fetchers** for S3/GCS in tests

## Dependencies (keep minimal)

```
github.com/spf13/cobra          # CLI framework
github.com/hashicorp/vault/api  # Vault client
github.com/aws/aws-sdk-go-v2    # S3 access
cloud.google.com/go/storage     # GCS access
gopkg.in/yaml.v3                # YAML parsing
```

## Implementation Order

1. **Phase 1:** Config parsing + variable substitution
2. **Phase 2:** Password generator
3. **Phase 3:** Local file fetcher + tfstate parser
4. **Phase 4:** Vault client (token auth, KV v2)
5. **Phase 5:** S3 fetcher
6. **Phase 6:** Reconciliation engine + dry-run
7. **Phase 7:** CLI with cobra
8. **Phase 8:** Additional features (GCS, KV v1, approle, etc.)

## Code Style

- Use standard Go formatting (`gofmt`)
- Keep functions small and focused
- Comprehensive error wrapping with context
- Use `context.Context` for cancellation
- Structured logging (consider `slog` from stdlib)

## Git Conventions

- Stage files explicitly with `git add <file>`
- Commit with `git commit -m 'message'`
- Can use `git commit -am 'message'` for tracked files
- **Never use `git add -A`** - always stage files explicitly

## Example Usage in Kubernetes

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: vsg
spec:
  schedule: "*/10 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: vsg
          containers:
            - name: vsg
              image: ghcr.io/yourname/vsg:latest
              args: ["apply", "--config", "/etc/config/secrets.yaml"]
              env:
                - name: VAULT_ADDR
                  value: "http://vault.vault:8200"
              volumeMounts:
                - name: config
                  mountPath: /etc/config
          volumes:
            - name: config
              configMap:
                name: vsg-config
          restartPolicy: OnFailure
```

## Non-Goals (Out of Scope)

- Secret rotation scheduling (use external scheduler)
- Vault policy management
- Multi-Vault support (single Vault per config)
- Webhooks or event-driven sync
- Web UI
