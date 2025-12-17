# VSG - Vault Secrets Generator

## Project Overview

A lightweight, cloud-agnostic CLI tool that generates and populates secrets in HashiCorp Vault from various sources including remote files (Terraform state, configs), generated passwords, commands, and static values.

**Primary use cases:**
- Extract values from remote JSON/YAML files (S3, GCS, Azure, local) and write them to Vault
- Generate secrets with configurable password policies
- Run commands to generate values (e.g., password hashing)
- Copy secrets between Vault paths
- Declarative HCL configuration for GitOps workflows

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
│   │   ├── delete.go               # Delete command
│   │   ├── diff.go                 # Diff command
│   │   └── version.go              # Version command
│   ├── config/
│   │   ├── config.go               # HCL config parsing
│   │   └── types.go                # Config structs
│   ├── fetcher/
│   │   ├── fetcher.go              # Fetcher interface
│   │   ├── s3.go                   # S3 backend
│   │   ├── gcs.go                  # GCS backend
│   │   ├── azure.go                # Azure Blob Storage backend
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
│   └── config.hcl                  # Example HCL configuration
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

The tool uses a declarative HCL configuration (v2.0 format):

```hcl
vault {
  address = "https://vault.example.com"

  auth {
    method = "kubernetes"
    role   = "vsg"
  }
}

defaults {
  mount   = "secret"   # default mount if not specified per-secret
  version = 2          # KV version: 1 or 2, auto-detect if omitted

  strategy {
    generate = "create"
    json     = "update"
    yaml     = "update"
    raw      = "update"
    static   = "update"
    command  = "update"
    vault    = "update"
  }

  generate {
    length     = 32
    digits     = 5
    symbols    = 5
    symbol_set = "-_$@"
    no_upper   = false
  }
}

secret "dev-app" {
  path  = "${env(\"ENV\")}/app"  # path supports interpolation
  prune = true

  content {
    api_key     = generate()
    jwt_secret  = generate({length = 64, symbols = 0})
    db_host     = json("s3://bucket/dev/terraform.tfstate", ".outputs.db_host.value")
    db_port     = "5432"
    config_host = yaml("gcs://bucket/config.yaml", ".database.host")
    ssh_key     = raw("s3://bucket/keys/deploy.pem")
    shared_key  = vault("secret/shared", "api_key")
    caddy_hash  = command("caddy hash-password --plaintext mypassword")

    # Per-key strategy override
    special     = generate({length = 64, strategy = "update"})
  }
}
```

One secret block = one Vault path = multiple key-value pairs inside.

## Value Types (HCL Functions)

| Type | Syntax | Example |
|------|--------|---------|
| Static | `key = "value"` | `db_port = "5432"` |
| Generate | `key = generate()` | `api_key = generate()` |
| Generate (custom) | `key = generate({...})` | `jwt_secret = generate({length = 64, symbols = 0})` |
| JSON | `key = json(url, query)` | `db_host = json("s3://...", ".outputs.db_host.value")` |
| YAML | `key = yaml(url, query)` | `config = yaml("gcs://...", ".database.host")` |
| Raw | `key = raw(url)` | `ssh_key = raw("s3://bucket/key.pem")` |
| Vault | `key = vault(path, key)` | `shared = vault("secret/shared", "api_key")` |
| Command | `key = command(cmd)` | `hash = command("caddy hash-password ...")` |

All functions support optional `strategy` parameter via object literal:

```hcl
db_host = json("s3://...", ".outputs.db_host.value", {strategy = "create"})
ssh_key = raw("s3://bucket/key.pem", {strategy = "create"})
```

## URL Schemes

For `json()`, `yaml()`, `raw()` functions:

| Scheme | Source |
|--------|--------|
| `s3://bucket/path` | AWS S3 |
| `gcs://bucket/path` | Google Cloud Storage |
| `az://container/path` | Azure Blob Storage |
| `/path/to/file` | Local file (no scheme) |

## Generate Options

| Option | Default | Description |
|--------|---------|-------------|
| `length` | 32 | Total password length |
| `digits` | 5 | Minimum digit characters |
| `symbols` | 5 | Minimum symbol characters |
| `symbol_set` | `-_$@` | Allowed symbol characters |
| `no_upper` | false | Exclude uppercase letters |

## Environment Variables: env() Function

The `env()` function retrieves values from environment variables or `--var` CLI flags:

```hcl
secret "secret/dev/app" {
  db_host = json("s3://my-bucket/dev/terraform.tfstate", ".outputs.db_host.value")
  region  = env("REGION")
}
```

Note: Template interpolation (`${...}`) is not supported in HCL block labels or string values. Use `env()` for individual values.

Usage:

```bash
# From environment
ENV=dev BUCKET=my-terraform-state vsg apply

# From CLI (overrides environment)
vsg apply --var ENV=dev --var BUCKET=my-terraform-state

# Mixed (CLI wins)
ENV=prod vsg apply --var ENV=dev   # uses "dev"
```

**Priority:** `--var` CLI flag > environment variable

## Strategies

| Strategy | Key missing | Key exists, same value | Key exists, different value |
|----------|-------------|------------------------|----------------------------|
| `create` | Create | Skip | Skip |
| `update` | Create | Skip | Update |

### Default Strategies

| Value type | Default strategy | Reasoning |
|------------|-----------------|-----------|
| `generate` | `create` | Don't regenerate existing passwords |
| `json` | `update` | Keep in sync with source |
| `yaml` | `update` | Keep in sync with source |
| `raw` | `update` | Keep in sync with source |
| `static` | `update` | Update if changed |
| `command` | `update` | Re-run and update |
| `vault` | `update` | Keep in sync with source |

Per-key override via `strategy` parameter in any function.

## Prune (Per Secret)

| `prune` | Key in config | Key in Vault only |
|---------|---------------|-------------------|
| `false` (default) | Create/Update | Warn, keep |
| `true` | Create/Update | Delete |

## CLI Interface

```bash
vsg apply                                  # apply config to vault
vsg apply --config config.hcl              # specify config file
vsg apply --dry-run                        # preview changes
vsg apply --force                          # regenerate all passwords
vsg apply --var ENV=dev --var REGION=us    # pass variables

vsg diff                                   # show diff
vsg diff --config config.hcl

# Delete entire secret
vsg delete secret/path                     # soft delete (default, recoverable)
vsg delete secret/path --hard              # destroy version data permanently
vsg delete secret/path --full              # remove all versions + metadata

# Delete specific keys (writes new version without those keys)
vsg delete secret/path --keys key1,key2

vsg version
```

### Delete Flags (KV v2)

| VSG Flag | Vault Equivalent | Effect |
|----------|------------------|--------|
| (default) | `vault kv delete` | Soft delete, recoverable via `vault kv undelete` |
| `--hard` | `vault kv destroy` | Permanently destroy version data, metadata remains |
| `--full` | `vault kv metadata delete` | Remove all versions and metadata completely |

**Note:** `--hard` and `--full` only apply to KV v2. KV v1 delete is always permanent.

### Exit Codes

- `0` - Success, all secrets synced
- `1` - Configuration error
- `2` - Vault connection/auth error
- `3` - Source file fetch error
- `4` - Partial failure (some secrets failed)

## Vault Auto-Detection

VSG auto-detects KV v1 vs v2 by querying `/sys/mounts`. User specifies path (e.g., `secret/dev`), VSG determines engine version automatically.

## Core Logic

### Reconciliation Flow

1. Parse and validate config HCL
2. Resolve all `env()` function calls
3. For each secret block:
   a. Connect to Vault at specified path
   b. Read current secrets (if exist)
   c. For each key in data:
      - If `json()`/`yaml()`/`raw()`: fetch file, extract value
      - If `generate()`: generate password based on strategy
      - If `vault()`: read from source Vault path
      - If `command()`: run command and capture output
      - If static: use value as-is
   d. Apply strategy (create vs update) per key
   e. Write changes to Vault (unless --dry-run)
   f. If `prune = true`: delete keys in Vault not in config
4. Report results

### Important Behaviors

1. **Strategy-based reconciliation:**
   - `create` strategy: only creates new keys, never updates existing
   - `update` strategy: creates new keys and updates changed values
   - `--force` flag: regenerate all passwords regardless of strategy

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

Implementations:
- `s3://` - AWS S3 (use AWS SDK, support IRSA and standard credential chain)
- `gcs://` - Google Cloud Storage
- `az://` - Azure Blob Storage
- Local filesystem (no scheme)

## Password Generator

```go
type PasswordPolicy struct {
    Length           int    // Total length (default: 32)
    Digits           int    // Minimum digits (default: 5)
    Symbols          int    // Minimum symbols (default: 5)
    SymbolCharacters string // Allowed symbols (default: "-_$@")
    NoUpper          bool   // Exclude uppercase (default: false)
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
AZURE_STORAGE_ACCOUNT           # Azure storage account
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
3. **Mock fetchers** for S3/GCS/Azure in tests

## Dependencies (keep minimal)

```
github.com/spf13/cobra          # CLI framework
github.com/hashicorp/hcl/v2     # HCL parser
github.com/hashicorp/vault/api  # Vault client
github.com/aws/aws-sdk-go-v2    # S3 access
cloud.google.com/go/storage     # GCS access
github.com/Azure/azure-sdk-for-go/sdk/storage/azblob  # Azure Blob access
```

## Implementation Order

1. **Phase 1:** HCL config parsing with custom functions
2. **Phase 2:** Password generator
3. **Phase 3:** Local file fetcher + JSON/YAML parser
4. **Phase 4:** Vault client (token auth, KV v2)
5. **Phase 5:** S3 fetcher
6. **Phase 6:** Reconciliation engine with strategies + dry-run
7. **Phase 7:** CLI with cobra
8. **Phase 8:** Additional features (GCS, Azure, command execution, prune)
9. **Phase 9:** Delete command

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

### Commit Message Prefixes

Use these prefixes for proper release note categorization:

| Prefix | Use for | Example |
|--------|---------|---------|
| `feat:` | New features | `feat: add password hashing functions` |
| `fix:` | Bug fixes | `fix: correct KV v2 path resolution` |
| `docs:` | Documentation only | `docs: update README examples` |
| `refactor:` | Code changes that don't add features or fix bugs | `refactor: simplify config parsing` |
| `test:` | Adding/updating tests | `test: add integration tests for S3` |
| `chore:` | Maintenance tasks | `chore: update dependencies` |
| `ci:` | CI/CD changes | `ci: add arm64 builds` |

Commits with `docs:`, `test:`, `chore:`, `ci:` prefixes are excluded from release notes.

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
              args: ["apply", "--config", "/etc/config/secrets.hcl"]
              env:
                - name: VAULT_ADDR
                  value: "http://vault.vault:8200"
                - name: ENV
                  value: "prod"
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

## Current Status

**Version:** v2.0.0

### Implemented Features
- [x] HCL config parsing with custom functions
- [x] `env()` function for environment variables with CLI `--var` override
- [x] `generate()`, `json()`, `yaml()`, `raw()`, `vault()`, `command()` functions
- [x] Strategy system (`create` vs `update`) per value type
- [x] Per-key strategy override via object literal syntax
- [x] Prune logic per secret block
- [x] CLI `--var` flag for variable override
- [x] Path-based delete command with `--hard`, `--full`, `--keys` flags
- [x] Password generator with configurable policies
- [x] Local file fetcher (tested)
- [x] S3 fetcher with AWS SDK v2 (tested with json/yaml/raw extraction)
- [x] Vault client with token auth, KV v1/v2 auto-detection (tested)
- [x] Reconciliation engine + dry-run (tested)
- [x] CLI with cobra
- [x] Helm chart
- [x] Dockerfile
- [x] GitHub Actions CI/CD
- [x] goreleaser configuration with auto-release
- [x] Command execution with multiline output (tested)
- [x] Homebrew tap (`brew install pavlenkoa/tap/vsg`)
- [x] **v2.0.0 config structure** with `content {}` block and path interpolation

### Planned
- [ ] GCS fetcher
- [ ] Azure Blob Storage fetcher
- [ ] Kubernetes auth testing
- [ ] AppRole auth testing
- [ ] Password hashing functions with referential values (see below)

## v2.0.0 Implementation Plan

This is a **breaking change** that restructures the config format to support dynamic paths.

### 1. New Config Structure

```hcl
vault {
  address = "https://vault.example.com"

  auth {
    method = "kubernetes"
    role   = "vsg"
  }
}

defaults {
  mount   = "secret"   # default mount if not specified per-secret
  version = 2          # KV version: 1 or 2, auto-detect if omitted

  strategy {
    generate = "create"
    json     = "update"
    yaml     = "update"
    raw      = "update"
    static   = "update"
    command  = "update"
    vault    = "update"
  }

  generate {
    length     = 32
    digits     = 5
    symbols    = 5
    symbol_set = "-_$@"
    no_upper   = false
  }
}

secret "prod-app" {
  mount   = "secret"                # optional, uses defaults.mount
  path    = "${env("ENV")}/app"     # path within mount, supports interpolation
  version = 2                       # optional, uses defaults.version, then auto-detect
  prune   = true

  content {
    api_key     = generate()
    db_password = generate({length = 64})
    db_host     = json("s3://bucket/terraform.tfstate", ".outputs.db_host.value")
    db_port     = "5432"
    ssh_key     = raw("s3://bucket/keys/deploy.pem")
    shared      = vault("secret/shared", "api_key")
    hash        = command("caddy hash-password --plaintext mypassword")
  }
}

secret "prod-database" {
  path = "prod/database"   # uses defaults.mount and defaults.version

  content {
    password = generate()
    host     = "db.example.com"
  }
}
```

### 2. Key Changes from v1.x

| v1.x | v2.0 |
|------|------|
| `secret "kv/prod/app" { ... }` | `secret "prod-app" { path = "prod/app" content { ... } }` |
| Path in block label | Path as attribute inside block |
| Keys directly in secret block | Keys inside `content {}` block |
| No mount separation | Explicit `mount` attribute |
| Auto-detect KV version only | Optional explicit `version` attribute |

### 3. Validation Rules

Implement these validations at config parse time:

1. **Labels must be unique** - no two `secret "xxx"` with same label
2. **Mount + path must be unique** - no two secrets resolving to same mount + path (after env interpolation)
3. **`path` is required** - error if omitted
4. **`content {}` is required** - error if omitted or empty
5. **`mount` optional** - falls back to `defaults.mount`, then `"secret"`
6. **`version` optional** - falls back to `defaults.version`, then auto-detect from `/sys/mounts`

### 4. Vault Path Resolution

```
KV v2: /{mount}/data/{path}
KV v1: /{mount}/{path}
```

Example with `mount = "secret"`, `path = "prod/app"`:
- KV v2 → `secret/data/prod/app`
- KV v1 → `secret/prod/app`

### 5. Version Resolution Priority

1. `secret.version` (explicit per-secret)
2. `defaults.version` (global default)
3. Auto-detect from `/sys/mounts`

### 6. Implementation Checklist

- [x] Update `internal/config/types.go` - add `Mount` to `Defaults` and `SecretBlock`, rename `Data` to `Content`
- [x] Update `internal/config/hcl.go` - parse new structure with `content {}` block
- [x] Update `internal/engine/reconcile.go` - use mount + path for Vault operations
- [x] Update `internal/engine/diff.go` - update diff logic for new structure
- [x] Update tests for new config structure
- [x] Update `CLAUDE.md` with new config examples
- [x] Update `README.md` with new config examples
- [x] Update `examples/config.hcl`
- [ ] Create `docs/migration-from-terraform.md`
- [ ] Bump version to v2.0.0 (tag release)

### 7. Important Implementation Notes

- **Batch writes**: All keys in `content {}` = single Vault API call (read once, compute, write once)
- **Interpolation**: `${env("VAR")}` works in `path` attribute (it's an HCL expression now)
- **Reserved keys**: `mount`, `path`, `version`, `prune` are metadata; user can have keys with same names inside `content {}`

### 8. Migration Guide: Terraform to VSG

Create `docs/migration-from-terraform.md`:

#### Why Migrate?
- Secrets stored in Terraform state (security risk)
- Terraform state contains plaintext passwords
- VSG generates secrets without storing them anywhere

#### Terraform Resource
```hcl
resource "vault_kv_secret_v2" "db" {
  mount = "secret"
  name  = "prod/db"
  data_json = jsonencode({
    password = random_password.db.result
    host     = aws_rds_instance.db.endpoint
  })
}

output "db_endpoint" {
  value = aws_rds_instance.db.endpoint
}
```

#### VSG Equivalent
```hcl
secret "db" {
  mount = "secret"
  path  = "prod/db"

  content {
    password = generate()
    host     = json("s3://bucket/terraform.tfstate", ".outputs.db_endpoint.value")
  }
}
```

#### Migration Steps
1. `terraform apply` (adds outputs)
2. `vsg apply --dry-run` (verify changes)
3. `vsg apply` (VSG now manages secrets)
4. Remove `vault_kv_secret_v2` from Terraform
5. `terraform apply` (Terraform stops managing)

**Note:** Run VSG before removing Terraform resource to avoid downtime. Generated passwords won't match old ones - coordinate with app deployments.

## Planned Feature: Password Hashing Functions

### Overview

Add native password hashing functions that can reference other keys in the same secret block. This enables generating a password and its hash in a single declarative config.

### Proposed Syntax

```hcl
secret "authelia" {
  path = "authelia"

  content {
    # Generate a random password
    admin_password = generate({length = 32})

    # Hash the generated password with argon2id
    admin_password_hash = argon2({from = "admin_password"})

    # Generate OIDC client secret and its PBKDF2 hash
    oidc_client_plaintext = generate({length = 64, symbols = 0})
    oidc_client_secret = pbkdf2({from = "oidc_client_plaintext", variant = "sha512"})

    # bcrypt example
    api_key = generate()
    api_key_hash = bcrypt({from = "api_key", cost = 12})
  }
}
```

### Supported Algorithms

| Function | Output Format | Use Cases |
|----------|---------------|-----------|
| `bcrypt({from, cost})` | `$2a$cost$salt...hash` | Most web frameworks (Rails, Django, Node.js) |
| `argon2({from, variant, memory, iterations, parallelism})` | `$argon2id$v=19$m=65536,t=3,p=4$salt$hash` | Authelia, Bitwarden, modern apps |
| `pbkdf2({from, variant, iterations})` | `$pbkdf2-sha512$iterations$salt$hash` | Enterprise/FIPS compliance, Authelia OIDC |

### Default Parameters

| Algorithm | Parameter | Default |
|-----------|-----------|---------|
| bcrypt | cost | 12 |
| argon2 | variant | id |
| argon2 | memory | 65536 (64MB) |
| argon2 | iterations | 3 |
| argon2 | parallelism | 4 |
| pbkdf2 | variant | sha512 |
| pbkdf2 | iterations | 310000 |

### Implementation Requirements

1. **Referential Evaluation**: Engine must resolve dependencies in topological order
   - Parse `from` references to build dependency graph
   - Detect cycles and report errors
   - Resolve base values first, then derived values

2. **PHC String Formatting**: Output must match standard Password Hashing Competition format
   - Salt generation using crypto/rand
   - Proper base64 encoding (standard or raw depending on algorithm)
   - Version and parameter encoding

3. **Go Libraries**:
   - bcrypt: `golang.org/x/crypto/bcrypt`
   - argon2: `golang.org/x/crypto/argon2`
   - pbkdf2: `crypto/pbkdf2` (stdlib)

### Strategy Behavior

Hash functions use `create` strategy by default (like `generate()`):
- If the hash already exists in Vault, skip regeneration
- Use `--force` flag to regenerate all passwords and hashes
- Can override with `strategy = "update"` if needed
