# Releasing

This document describes how to publish a new version of the Terraform Uptrace CE
provider to the Terraform Registry.

## One-time setup

### 1. Generate a GPG signing key

The Terraform Registry requires release artifacts to be signed with GPG.

```bash
gpg --full-generate-key
```

Choose RSA 4096-bit with no expiration (or your preferred expiration). Use an
email associated with your GitHub account.

### 2. Add the public key to the Terraform Registry

Export the public key and add it to the provider's signing keys page on the
Terraform Registry:

```bash
gpg --armor --export <key-id>
```

See https://developer.hashicorp.com/terraform/registry/providers/publishing#preparing-and-adding-a-signing-key

### 3. Configure repository secrets

In the GitHub repository settings under **Settings > Secrets and variables > Actions**,
add the following secrets:

| Secret            | Value                                                        |
| ----------------- | ------------------------------------------------------------ |
| `GPG_PRIVATE_KEY` | Armor-exported private key (`gpg --armor --export-secret-keys <key-id>`) |
| `PASSPHRASE`      | Passphrase for the GPG key                                   |

## Creating a release

1. Make sure all changes are merged to the main branch and CI is green.

2. Choose a version following [Semantic Versioning](https://semver.org/):
   - **patch** (v0.1.1) — bug fixes
   - **minor** (v0.2.0) — new resources or data sources, new attributes
   - **major** (v1.0.0) — breaking changes to existing resources or attributes

3. Create and push an annotated tag:

   ```bash
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```

4. The **Release** GitHub Actions workflow triggers automatically. It builds
   binaries for all supported platforms, signs the checksum file, and creates a
   **draft** GitHub release.

5. Review the draft release on the GitHub releases page. Edit the release notes
   if needed, then publish it.

6. The Terraform Registry detects the published release via webhook and makes
   the new version available.

## Verifying

After the release is published, confirm the version appears on the registry:

```
https://registry.terraform.io/providers/uptrace/uptrace-ce/latest
```

You can also verify locally:

```bash
terraform init -upgrade
terraform providers
```
