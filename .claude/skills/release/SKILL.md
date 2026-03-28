---
name: release
description: Use when creating a release, tagging a version, bumping version, or preparing to publish a new VSG version
---

# Creating a VSG Release

## Overview

Releases use `make tag` to bump the Helm chart appVersion and create a git tag in one step. The GitHub Actions release workflow then builds binaries, Docker image, and Homebrew formula automatically.

## Steps

1. Ensure all changes are committed and pushed to main
2. Check the current version from git tags: `git tag --list 'v*' --sort=-v:refname | head -1`
   - Do NOT trust CLAUDE.md or any docs for the current version — git tags are the source of truth
3. Run: `make tag TAG=x.y.z`
   - Updates `helm/vault-secrets-generator/Chart.yaml` appVersion
   - Commits with `chore: bump helm chart appVersion to x.y.z`
   - Creates git tag `vx.y.z`
4. Push: `git push origin main vx.y.z`
5. Verify the [Release workflow](../../.github/workflows/release.yaml) completes successfully

## Never Do

- Never create tags with `git tag` directly — appVersion will be out of sync
- Never manually edit Chart.yaml appVersion — use `make tag`
- Never push the tag without the commit — both must go together

## What Happens After Push

The release workflow (`.github/workflows/release.yaml`) triggers on `v*` tags and:
- Builds binaries for linux/darwin/windows (amd64/arm64) via goreleaser
- Pushes Docker image to `ghcr.io/pavlenkoa/vault-secrets-generator:x.y.z`
- Updates Homebrew tap (`brew install pavlenkoa/tap/vsg`)
- Creates GitHub Release with changelog

## Version Convention

Use semantic versioning: `MAJOR.MINOR.PATCH` (e.g., `2.3.0`). The `v` prefix is added by `make tag` automatically.
