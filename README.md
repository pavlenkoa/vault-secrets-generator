# VSG - Vault Secrets Generator

A lightweight, cloud-agnostic CLI tool that generates and populates secrets in HashiCorp Vault from various sources including remote files, generated passwords, commands, and static values.

## Features

- **HCL Configuration**: Declarative HCL-based configuration with custom functions
- **Remote File Integration**: Extract values from JSON/YAML files stored in S3, GCS, or local filesystem
- **Password Generation**: Generate secure passwords with configurable policies
- **Command Execution**: Generate values by running shell commands
- **Copy Between Paths**: Copy secrets between Vault paths using `vault()` function
- **Strategy System**: Control when values are created vs updated with `create` and `update` strategies
- **Prune Support**: Optionally delete unmanaged keys from Vault paths
- **Idempotent Operations**: Generated passwords are preserved unless strategy or force flag overrides
- **Dry-Run Support**: Preview changes before applying them
- **Multiple Auth Methods**: Token, Kubernetes, and AppRole authentication

## Installation

### Homebrew (macOS/Linux)

```bash
brew install --cask pavlenkoa/tap/vsg
```

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

## Quick Start

1. Create a configuration file `config.hcl`:

```hcl
vault {
  address = "https://vault.example.com"
  auth {
    method = "token"
  }
}

secret "secret/dev/database" {
  host     = json("s3://terraform-state/dev/rds.tfstate", ".outputs.endpoint.value")
  password = generate()
  port     = "5432"
}

secret "secret/dev/app" {
  api_key     = generate()
  environment = "dev"
}
```

2. Set up Vault credentials:

```bash
export VAULT_ADDR="https://vault.example.com"
export VAULT_TOKEN="hvs.xxxxx"
```

3. Preview changes:

```bash
vsg diff --config config.hcl
```

4. Apply secrets:

```bash
vsg apply --config config.hcl
```

## CLI Reference

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Config file path (or set `VSG_CONFIG` env var) |
| `--var` | | Set variable KEY=VALUE (can be repeated) |
| `--verbose` | `-v` | Enable verbose output |

### Commands

#### `vsg apply`

Apply secrets to Vault.

```bash
vsg apply --config config.hcl [flags]
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be done without making changes |
| `--force` | Force regeneration of generated secrets |
| `--var KEY=VALUE` | Set variable (can be repeated) |

#### `vsg diff`

Show differences between current and desired state.

```bash
vsg diff --config config.hcl [flags]
```

| Flag | Description |
|------|-------------|
| `--output` | Output format: `text` (default) or `json` |
| `--var KEY=VALUE` | Set variable (can be repeated) |

#### `vsg delete`

Delete secrets from Vault. Works directly with Vault paths (no config required).

```bash
vsg delete <path> [flags]
```

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |
| `--hard` | Destroy version data permanently (KV v2 only) |
| `--full` | Remove all versions and metadata (KV v2 only) |
| `--keys` | Comma-separated list of keys to delete |

Examples:

```bash
# Soft delete (recoverable in KV v2)
vsg delete secret/myapp

# Delete specific keys only
vsg delete secret/myapp --keys old_key,deprecated_key

# Destroy version data permanently
vsg delete secret/myapp --hard

# Remove all versions and metadata
vsg delete secret/myapp --full
```

#### `vsg version`

Print version information.

```bash
vsg version
```

## Configuration

### Full Example

```hcl
# Vault connection settings
vault {
  address = "https://vault.example.com"
  # namespace = "admin"  # Optional, for Vault Enterprise

  auth {
    method = "kubernetes"
    role   = "vsg"
    # mount_path = "kubernetes"  # Optional, defaults to "kubernetes"

    # For token auth:
    # method = "token"
    # token = "hvs.xxx"  # Or use VAULT_TOKEN env var

    # For approle auth:
    # method = "approle"
    # role_id = "xxx"    # Or use VAULT_ROLE_ID env var
    # secret_id = "xxx"  # Or use VAULT_SECRET_ID env var
  }
}

# Default settings for all secrets
defaults {
  # Default strategies per value type
  strategy {
    generate = "create"  # Don't regenerate existing passwords
    json     = "update"  # Keep in sync with source
    yaml     = "update"  # Keep in sync with source
    raw      = "update"  # Keep in sync with source
    static   = "update"  # Update if changed
    command  = "update"  # Re-run and update
    vault    = "update"  # Keep in sync with source
  }

  # Default password generation policy
  generate {
    length     = 32
    digits     = 5
    symbols    = 5
    symbol_set = "-_$@"
    no_upper   = false
  }
}

# Database secrets
secret "secret/dev/database" {
  prune = true  # Delete keys in Vault not defined here

  # Extract from Terraform state file in S3
  host     = json("s3://terraform-state/dev/rds.tfstate", ".outputs.endpoint.value")
  port     = json("s3://terraform-state/dev/rds.tfstate", ".outputs.port.value")
  username = json("s3://terraform-state/dev/rds.tfstate", ".outputs.username.value")

  # Generated password - won't regenerate if exists (create strategy)
  password = generate()

  # Static value
  database = "myapp"
}

# Application secrets
secret "secret/dev/app" {
  # api_key with default policy (32 chars)
  api_key = generate()

  # jwt_secret with custom policy (64 chars, no symbols)
  jwt_secret = generate({length = 64, symbols = 0})

  # webhook_token with custom policy and strategy override
  webhook_token = generate({length = 48, symbols = 0, strategy = "update"})

  # Copy from another Vault path
  shared_key = vault("secret/shared", "api_key")

  # From command execution
  password_hash = command("echo -n 'secret123' | sha256sum | cut -d' ' -f1")

  # Environment value from env() function
  environment = env("ENV")

  # Static values
  log_level = "info"
  version   = "1.2.3"
}

# SSH keys from raw files
secret "secret/dev/ssh" {
  # Raw file content (no extraction)
  deploy_key = raw("s3://keys/dev/deploy.pem")

  # With create strategy - won't update if key exists
  backup_key = raw("s3://keys/dev/backup.pem", {strategy = "create"})
}

# Config extracted from YAML file
secret "secret/dev/config" {
  redis_host = yaml("s3://configs/dev/app.yaml", ".redis.host")
  redis_port = yaml("s3://configs/dev/app.yaml", ".redis.port")
}
```

### Value Types (HCL Functions)

| Type | Syntax | Description |
|------|--------|-------------|
| Static | `key = "value"` | Static string value |
| Generate | `generate()` | Generate password with default policy |
| Generate (custom) | `generate({length = 64, ...})` | Generate with custom policy |
| JSON | `json(url, query)` | Extract from JSON file |
| YAML | `yaml(url, query)` | Extract from YAML file |
| Raw | `raw(url)` | Raw file content |
| Vault | `vault(path, key)` | Copy from another Vault path |
| Command | `command(cmd)` | Execute shell command |
| Env | `env(name)` | Environment variable |

All functions support optional strategy parameter via object literal:

```hcl
db_host  = json("s3://...", ".outputs.db_host.value", {strategy = "create"})
ssh_key  = raw("s3://bucket/key.pem", {strategy = "create"})
password = generate({length = 64, strategy = "update"})
```

### URL Schemes

For `json()`, `yaml()`, and `raw()` functions:

| Scheme | Source |
|--------|--------|
| `s3://bucket/path` | AWS S3 |
| `gcs://bucket/path` | Google Cloud Storage |
| `az://container/path` | Azure Blob Storage |
| `/path/to/file` | Local file (no scheme) |
| `file:///path` | Local file (explicit) |

### Generate Options

| Option | Default | Description |
|--------|---------|-------------|
| `length` | 32 | Total password length |
| `digits` | 5 | Minimum digit characters |
| `symbols` | 5 | Minimum symbol characters |
| `symbol_set` | `-_$@` | Allowed symbol characters |
| `no_upper` | false | Exclude uppercase letters |

### Strategies

| Strategy | Key missing | Key exists, same value | Key exists, different value |
|----------|-------------|------------------------|----------------------------|
| `create` | Create | Skip | Skip |
| `update` | Create | Skip | Update |

Default strategies by value type:

| Value type | Default strategy | Reasoning |
|------------|-----------------|-----------|
| `generate` | `create` | Don't regenerate existing passwords |
| `json` | `update` | Keep in sync with source |
| `yaml` | `update` | Keep in sync with source |
| `raw` | `update` | Keep in sync with source |
| `static` | `update` | Update if changed |
| `command` | `update` | Re-run and update |
| `vault` | `update` | Keep in sync with source |

### Prune

When `prune = true` is set on a secret block, keys in Vault that are not defined in the config will be deleted.

| `prune` | Key in config | Key in Vault only |
|---------|---------------|-------------------|
| `false` (default) | Create/Update | Warn, keep |
| `true` | Create/Update | Delete |

### Variables with env()

Use the `env()` function to reference environment variables or CLI variables:

```hcl
secret "secret/dev/app" {
  region = env("AWS_REGION")
}
```

CLI variables override environment variables:

```bash
# From environment
AWS_REGION=us-east-1 vsg apply

# From CLI (overrides environment)
vsg apply --var AWS_REGION=us-west-2
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
| 3 | Source file fetch error |
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

## License

Apache License 2.0 - see [LICENSE](LICENSE) file for details.
