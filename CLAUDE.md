# VSG - Vault Secrets Generator

## Project Overview

A lightweight, cloud-agnostic CLI tool that generates and populates secrets in HashiCorp Vault from various sources including remote files (Terraform state, configs), generated passwords, commands, and static values.

**Primary use cases:**
- Extract values from remote JSON/YAML files (S3, GCS, local) and write them to Vault
- Generate secrets with configurable password policies
- Run commands to generate values (e.g., password hashing)
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
│   │   └── types.go                # Config structs
│   ├── fetcher/
│   │   ├── fetcher.go              # Fetcher interface
│   │   ├── s3.go                   # S3 backend
│   │   ├── gcs.go                  # GCS backend
│   │   └── local.go                # Local file backend
│   ├── parser/
│   │   └── parser.go               # JSON/YAML parser with jq/yq syntax
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
vault:
  address: https://vault.example.com
  auth:
    method: kubernetes
    role: vsg

defaults:
  generate:
    length: 32
    digits: 5
    symbols: 5

secrets:
  dev:
    path: secret/dev
    data:
      api_key: generate
      jwt_secret:
        generate:
          length: 64
      db_host:
        source: s3://bucket/terraform.tfstate
        json: .outputs.db_host.value
      db_port: "5432"
      caddy_hash:
        command: caddy hash-password --plaintext "mypassword"

  prod:
    path: secret/prod
    data:
      api_key: generate
      db_host:
        source: s3://bucket/prod/terraform.tfstate
        json: .outputs.db_host.value
```

One secret block = one Vault path = multiple key-value pairs inside.

## Value Types

| Type | Syntax | Example |
|------|--------|---------|
| Static | `key: "value"` | `db_port: "5432"` |
| Generate (defaults) | `key: generate` | `api_key: generate` |
| Generate (custom) | `key: {generate: {length: 64}}` | `jwt_secret: {generate: {length: 64, symbols: 0}}` |
| Remote JSON | `key: {source: ..., json: ...}` | `db_host: {source: s3://bucket/file.tfstate, json: .outputs.db_host.value}` |
| Remote YAML | `key: {source: ..., yaml: ...}` | `config: {source: s3://bucket/config.yaml, yaml: .database.host}` |
| Command | `key: {command: ...}` | `hash: {command: caddy hash-password --plaintext "xxx"}` |

**Source types supported:**
- `s3://bucket/path`
- `gcs://bucket/path`
- `file:///path`

**Parser syntax:**
- `json:` uses jq-style dot notation
- `yaml:` uses yq-style dot notation

## CLI Interface

```bash
vsg apply --config config.yaml
vsg apply --config config.yaml --force      # regenerate all passwords
vsg apply --config config.yaml --dry-run    # preview changes
vsg diff --config config.yaml               # show diff
vsg version
```

### Exit Codes

- `0` - Success, all secrets synced
- `1` - Configuration error
- `2` - Vault connection/auth error
- `3` - Source file fetch error
- `4` - Partial failure (some secrets failed)

## Reconciliation Strategy

| Scenario | Behavior |
|----------|----------|
| Key in config, not in Vault | Create |
| Key in config (generate), exists in Vault | Skip (keep existing) |
| Key in config (generate) + `--force` flag | Regenerate |
| Key in config (source), value changed | Update |
| Key in config (static), value changed | Update |
| Key in config (command) | Run command, update if changed |
| Key in Vault, not in config | Warn (log unmanaged key) |

No deletion support - use vault CLI directly for that.

## Vault Auto-Detection

VSG auto-detects KV v1 vs v2 by querying `/sys/mounts`. User just specifies the path (e.g., `secret/dev`), VSG figures out the engine version and uses the appropriate API.

## Core Logic

### Reconciliation Flow

1. Parse and validate config YAML
2. For each secret block:
   a. Connect to Vault at specified path
   b. Read current secrets (if exist)
   c. For each secret in data:
      - If source reference: fetch file, extract value using json/yaml parser
      - If generate: generate password (only if secret doesn't exist OR --force)
      - If command: run command and capture output
      - If static: use value as-is
   d. Compare current vs desired
   e. Write changes to Vault (unless --dry-run)
3. Report results

### Important Behaviors

1. **Idempotency for generated secrets:**
   - Generated passwords should only be created if the secret doesn't exist in Vault
   - Use `--force` flag to regenerate existing secrets
   - This prevents password rotation on every run

2. **Source file caching:**
   - Cache fetched files during a single run
   - Multiple secrets from same source file = one fetch

3. **Error handling:**
   - Continue on individual secret failures
   - Report all errors at end
   - Detailed error messages with context

## Fetcher Interface

```go
type Fetcher interface {
    // Fetch retrieves a file and returns its contents
    Fetch(ctx context.Context, uri string) ([]byte, error)

    // Supports returns true if this fetcher handles the given URI scheme
    Supports(uri string) bool
}
```

Implementations needed:
- `s3://` - AWS S3 (use AWS SDK, support IRSA and standard credential chain)
- `gcs://` - Google Cloud Storage
- `file://` - Local filesystem

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
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o vsg .

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

1. **Phase 1:** Config parsing
2. **Phase 2:** Password generator
3. **Phase 3:** Local file fetcher + JSON/YAML parser
4. **Phase 4:** Vault client (token auth, KV v2)
5. **Phase 5:** S3 fetcher
6. **Phase 6:** Reconciliation engine + dry-run
7. **Phase 7:** CLI with cobra
8. **Phase 8:** Additional features (GCS, command execution, etc.)

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
              image: ghcr.io/pavlenkoa/vault-secrets-generator:latest
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
- Automatic deletion of unmanaged keys
