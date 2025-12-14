# VSG v2.0 Example Configuration
#
# This example demonstrates all available features of the HCL configuration format.
# Use env() function for environment variable interpolation.
# Run with: vsg apply --config config.hcl --var ENV=dev

# Vault connection settings
vault {
  address = "https://vault.example.com"
  # namespace = "admin"  # Optional, for Vault Enterprise

  auth {
    method = "kubernetes"
    role   = "vsg"
    # mount_path = "kubernetes"  # Optional, defaults to "kubernetes"

    # For token auth (default):
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

# Database secrets - extracted from Terraform state
# Note: Use static paths in labels, use env() inside values for dynamic content
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
  # Use object literal syntax for named parameters
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
  # Use object literal syntax for options
  backup_key = raw("s3://keys/dev/backup.pem", {strategy = "create"})
}

# Config extracted from YAML file
secret "secret/dev/config" {
  # Extract values from YAML config file
  redis_host = yaml("s3://configs/dev/app.yaml", ".redis.host")
  redis_port = yaml("s3://configs/dev/app.yaml", ".redis.port")
}
