# VSG - Vault Secrets Generator

A lightweight, cloud-agnostic CLI tool that generates and populates secrets in HashiCorp Vault from various sources including Terraform state files, generated passwords, and static values.

## Features

- **Terraform State Integration**: Extract outputs from Terraform state files stored in S3, GCS, or local filesystem
- **Password Generation**: Generate secure passwords with configurable policies (length, digits, symbols, etc.)
- **Declarative Configuration**: YAML-based configuration for GitOps workflows
- **Idempotent Operations**: Generated passwords are preserved unless explicitly forced to regenerate
- **Dry-Run Support**: Preview changes before applying them
- **Multiple Auth Methods**: Token, Kubernetes, and AppRole authentication

## Installation

### From Source

```bash
go install github.com/pavlenkoa/vault-secrets-generator@latest
```

### Build from Source

```bash
git clone https://github.com/pavlenkoa/vault-secrets-generator.git
cd vault-secrets-generator
go build -o vsg .
```

### Build with Version Info

```bash
go build -ldflags "-X github.com/pavlenkoa/vault-secrets-generator/internal/command.Version=1.0.0 \
  -X github.com/pavlenkoa/vault-secrets-generator/internal/command.Commit=$(git rev-parse HEAD) \
  -X github.com/pavlenkoa/vault-secrets-generator/internal/command.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o vsg .
```

## Quick Start

1. Create a configuration file `config.yaml`:

```yaml
env:
  env: prod
  region: us-east-1

secrets:
  main:
    path: kv/myapp
    data:
      db/password: generate(length=32)
      db/host: s3://terraform-state/prod/rds/terraform.tfstate#output.endpoint
      api/key: generate
      app/environment: "production"
```

2. Set up Vault credentials:

```bash
export VAULT_ADDR="https://vault.example.com"
export VAULT_TOKEN="hvs.xxxxx"
```

3. Preview changes:

```bash
vsg diff --config config.yaml
```

4. Apply secrets:

```bash
vsg apply --config config.yaml
```

## CLI Reference

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Config file path (or set `VSG_CONFIG` env var) |
| `--verbose` | `-v` | Enable verbose output |

### Commands

#### `vsg apply`

Apply secrets to Vault.

```bash
vsg apply --config config.yaml [flags]
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be done without making changes |
| `--force` | Force regeneration of generated secrets |
| `--fail-fast` | Stop on first error |
| `--only <block>` | Only process this secret block |
| `--key <key>` | Only process this key (requires `--only`) |

#### `vsg diff`

Show differences between current and desired state.

```bash
vsg diff --config config.yaml [flags]
```

| Flag | Description |
|------|-------------|
| `--output` | Output format: `text` (default) or `json` |
| `--only <block>` | Only process this secret block |

#### `vsg delete`

Delete secrets from Vault.

```bash
vsg delete [block-name] --config config.yaml [flags]
```

| Flag | Description |
|------|-------------|
| `--all` | Delete all blocks defined in config |
| `--force` | Skip confirmation prompt |
| `--permanently` | Permanently delete (removes all versions in KV v2) |

#### `vsg version`

Print version information.

```bash
vsg version
```

## Configuration

### Full Example

```yaml
# Variables for substitution throughout the config
env:
  env: prod
  region: us-east-1
  project: myapp

# Vault connection settings
vault:
  address: https://vault.example.com
  namespace: admin  # Optional, for Vault Enterprise
  auth:
    method: token   # token, kubernetes, or approle
    # For token auth:
    # token: hvs.xxx  # Or use VAULT_TOKEN env var

    # For kubernetes auth:
    # method: kubernetes
    # role: my-app-role
    # mount_path: kubernetes  # Optional, defaults to "kubernetes"

    # For approle auth:
    # method: approle
    # role_id: xxx      # Or use VAULT_ROLE_ID env var
    # secret_id: xxx    # Or use VAULT_SECRET_ID env var

# Default password generation policy
defaults:
  generate:
    length: 32
    digits: 5
    symbols: 5
    symbolCharacters: "-_$@"
    noUpper: false
    allowRepeat: true

# Secrets definitions
secrets:
  main:
    path: kv/{env}/{project}
    # version: 2  # KV engine version (auto-detected if not set)
    data:
      # From Terraform state
      db/host: s3://terraform-state/{env}/rds/terraform.tfstate#output.endpoint
      db/port: s3://terraform-state/{env}/rds/terraform.tfstate#output.port

      # From local Terraform state
      local/value: file:///path/to/terraform.tfstate#output.some_output

      # Generated passwords with default policy
      db/password: generate

      # Generated with custom policy
      api/key: generate(length=64)
      jwt/secret: generate(length=48, symbols=0)
      simple/token: generate(length=24, digits=0, symbols=0)

      # Static values
      app/environment: "{env}"
      app/version: "1.2.3"

  docker:
    path: kv/{env}-docker
    data:
      registry/url: s3://terraform-state/{env}/ecr/terraform.tfstate#output.registry_url
```

### Value Types

#### Terraform State References

Extract values from Terraform state files:

```yaml
# S3 backend
value: s3://bucket/path/terraform.tfstate#output.output_name

# Local file
value: file:///path/to/terraform.tfstate#output.output_name

# Module outputs
value: s3://bucket/state.tfstate#output.module.rds.endpoint
```

#### Generated Passwords

```yaml
# Use default policy
password: generate

# Custom length
password: generate(length=64)

# Custom policy
password: generate(length=32, digits=10, symbols=0, noUpper=true)
```

**Available parameters:**
- `length` - Total password length (default: 32)
- `digits` - Minimum number of digits (default: 5)
- `symbols` - Minimum number of symbols (default: 5)
- `symbolCharacters` - Allowed symbols (default: `-_$@`)
- `noUpper` - Exclude uppercase letters (default: false)
- `allowRepeat` - Allow repeated characters (default: true)

#### Static Values

Any value that doesn't match the above patterns is treated as a static value:

```yaml
key: "my-static-value"
key: "{env}-config"  # Variables are substituted
```

### Variable Substitution

Variables defined in the `env` section can be used throughout the config with `{variable}` syntax:

```yaml
env:
  env: prod
  region: us-east-1

secrets:
  main:
    path: kv/{env}/{region}  # Becomes: kv/prod/us-east-1
    data:
      key: s3://bucket/{env}/state.tfstate#output.value
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `VAULT_ADDR` | Vault server address |
| `VAULT_TOKEN` | Vault token (for token auth) |
| `VAULT_NAMESPACE` | Vault namespace (Enterprise) |
| `VAULT_ROLE_ID` | AppRole role ID |
| `VAULT_SECRET_ID` | AppRole secret ID |
| `VSG_CONFIG` | Default config file path |
| `AWS_REGION` | AWS region for S3 |
| `AWS_PROFILE` | AWS profile |
| `GOOGLE_APPLICATION_CREDENTIALS` | GCP service account (for GCS) |

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Configuration error |
| 2 | Vault connection/auth error |
| 3 | State file fetch error |
| 4 | Partial failure (some secrets failed) |

## Kubernetes Usage

Example CronJob for periodic secret sync:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: vsg-sync
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

## Implementation Status

### Completed

- [x] **Phase 1**: Config parsing + variable substitution
- [x] **Phase 2**: Password generator with configurable policies
- [x] **Phase 3**: Local file fetcher + Terraform state parser
- [x] **Phase 4**: Vault client (token auth, KV v1/v2)
- [x] **Phase 5**: S3 fetcher (AWS SDK v2)
- [x] **Phase 6**: Reconciliation engine + dry-run
- [x] **Phase 7**: CLI with cobra

### Planned

- [ ] **Phase 8**: Additional features
  - [ ] GCS fetcher
  - [ ] Kubernetes auth
  - [ ] AppRole auth (implemented in client, needs testing)
  - [ ] Helm chart
  - [ ] Dockerfile
  - [ ] GitHub Actions CI/CD
  - [ ] goreleaser configuration

## Development

### Running Tests

```bash
go test ./...
```

### Running Integration Tests

Integration tests require a running Vault server:

```bash
export VAULT_ADDR="https://vault.example.com"
export VAULT_TOKEN="hvs.xxxxx"
go test ./... -v
```

### Project Structure

```
vault-secrets-generator/
├── main.go                         # CLI entrypoint
├── internal/
│   ├── command/                    # CLI commands
│   │   ├── root.go
│   │   ├── apply.go
│   │   ├── diff.go
│   │   ├── delete.go
│   │   └── version.go
│   ├── config/                     # Configuration parsing
│   │   ├── config.go
│   │   ├── types.go
│   │   └── variables.go
│   ├── fetcher/                    # State file fetchers
│   │   ├── fetcher.go
│   │   ├── local.go
│   │   └── s3.go
│   ├── tfstate/                    # Terraform state parser
│   │   └── parser.go
│   ├── generator/                  # Password generation
│   │   └── password.go
│   ├── vault/                      # Vault client
│   │   ├── client.go
│   │   └── writer.go
│   └── engine/                     # Reconciliation engine
│       ├── reconcile.go
│       ├── resolver.go
│       └── diff.go
├── examples/
│   └── config.yaml
├── go.mod
├── go.sum
└── README.md
```

## License

MIT License - see LICENSE file for details.
