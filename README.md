# VSG - Vault Secrets Generator

A lightweight, cloud-agnostic CLI tool that generates and populates secrets in HashiCorp Vault from various sources including remote files, generated passwords, commands, and static values.

> **📢 Version 2.0 Coming Soon:** VSG is migrating from YAML to HCL configuration. Current v1.x uses YAML (documented below). See [CLAUDE.md](CLAUDE.md) for the v2.0 HCL specification.

## Features

- **Remote File Integration**: Extract values from JSON/YAML files stored in S3, GCS, or local filesystem using jq/yq-style paths
- **Password Generation**: Generate secure passwords with configurable policies (length, digits, symbols, etc.)
- **Command Execution**: Generate values by running shell commands (e.g., password hashing)
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
secrets:
  database:
    path: kv/prod/database
    data:
      host:
        source: s3://terraform-state/prod/rds/terraform.tfstate
        json: .outputs.endpoint.value
      password: generate
      port: "5432"

  app:
    path: kv/prod/app
    data:
      api_key: generate
      environment: "production"
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

#### `vsg diff`

Show differences between current and desired state.

```bash
vsg diff --config config.yaml [flags]
```

| Flag | Description |
|------|-------------|
| `--output` | Output format: `text` (default) or `json` |

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
  database:
    path: kv/prod/database
    # version: 2  # KV engine version (auto-detected if not set)
    data:
      # From remote JSON file (Terraform state, API response, etc.)
      host:
        source: s3://terraform-state/prod/rds/terraform.tfstate
        json: .outputs.endpoint.value

      port:
        source: s3://terraform-state/prod/rds/terraform.tfstate
        json: .outputs.port.value

      # Generated password with default policy
      password: generate

  app:
    path: kv/prod/app
    data:
      # From local YAML file
      db_name:
        source: file:///etc/app/config.yaml
        yaml: .database.name

      # Generated with custom policy
      api_key: generate

      jwt_secret:
        generate:
          length: 64

      webhook_token:
        generate:
          length: 48
          symbols: 0

      # From command execution
      password_hash:
        command: htpasswd -nbB admin secret123 | cut -d: -f2

      # Static values
      environment: "production"
      version: "1.2.3"

  docker:
    path: kv/prod/docker
    data:
      registry_url:
        source: s3://terraform-state/prod/ecr/terraform.tfstate
        json: .outputs.registry_url.value

      registry_token:
        generate:
          length: 64
          symbols: 0
```

### Value Types

#### Remote Source with JSON Path

Extract values from remote JSON files using jq-style paths:

```yaml
# From S3
db_host:
  source: s3://bucket/path/terraform.tfstate
  json: .outputs.endpoint.value

# From local file
config_value:
  source: file:///path/to/config.json
  json: .database.host

# Nested paths and arrays
item:
  source: s3://bucket/data.json
  json: .items[0].name
```

#### Remote Source with YAML Path

Extract values from remote YAML files using yq-style paths:

```yaml
db_name:
  source: file:///etc/app/config.yaml
  yaml: .database.name

server_ip:
  source: s3://bucket/servers.yaml
  yaml: .servers[1].ip
```

#### Generated Passwords

```yaml
# Use default policy
password: generate

# Custom policy inline
api_key:
  generate:
    length: 64
    symbols: 0
    digits: 10

# Available options:
#   length: 32           # Total password length
#   digits: 5            # Minimum number of digits
#   symbols: 5           # Minimum number of symbols
#   symbolCharacters: "-_$@"  # Allowed symbols
#   noUpper: false       # Exclude uppercase letters
#   allowRepeat: true    # Allow repeated characters
```

#### Command Execution

Generate values by running shell commands:

```yaml
# Simple command
timestamp:
  command: date +%Y-%m-%d

# Password hashing
password_hash:
  command: htpasswd -nbB admin secret123 | cut -d: -f2

# Complex commands
ssh_key:
  command: ssh-keygen -t ed25519 -f /dev/stdout -N "" -q | head -n 5
```

#### Static Values

Any simple string value is treated as static:

```yaml
environment: "production"
version: "1.2.3"
api_url: "https://api.example.com"
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

## Helm Chart

A Helm chart is available for easy Kubernetes deployment:

```bash
helm install vsg ./helm/vault-secrets-generator \
  --set config.inline.vault.address=https://vault.example.com \
  --set auth.kubernetes.role=vsg
```

See [helm/vault-secrets-generator/values.yaml](helm/vault-secrets-generator/values.yaml) for all options.

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
│   │   └── types.go
│   ├── fetcher/                    # Remote file fetchers
│   │   ├── fetcher.go
│   │   ├── local.go
│   │   └── s3.go
│   ├── parser/                     # JSON/YAML extraction
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
├── helm/                           # Helm chart
│   └── vault-secrets-generator/
├── examples/
│   └── config.yaml
├── go.mod
├── go.sum
└── README.md
```

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details.
