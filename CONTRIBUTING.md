# Contributing

Development guide for building and testing the Uptrace Terraform provider.

## Building from source

```bash
go build -o terraform-provider-uptrace .
```

## Development setup

Create a dev override file so Terraform uses the local binary instead of downloading from the
registry:

```bash
cat > .terraformrc << 'EOF'
provider_installation {
  dev_overrides {
    "uptrace/uptrace" = "/path/to/terraform-provider-uptrace"
  }
  direct {}
}
EOF
```

Point Terraform at it:

```bash
export TF_CLI_CONFIG_FILE=/path/to/terraform-provider-uptrace/.terraformrc
```

## Testing

### Unit tests

Run without external dependencies:

```bash
make test
```

Covers pure mapping and request-building helpers, import paths, and ID validation. Runs in CI on
every push and pull request.

### Acceptance tests

Exercise the full Terraform lifecycle (plan, apply, import, destroy) against a real Uptrace API.
Gated behind the `TF_ACC` environment variable.

1. Copy the credentials template and fill it in:

   ```bash
   cp .env.example .env
   ```

2. Run:

   ```bash
   make testacc
   ```

Without `TF_ACC=1`, acceptance tests are automatically skipped.

### Writing tests

- **Unit tests** go in `*_unit_test.go` files with `package <name>` (internal). Use for pure
  functions that don't need a running API.
- **Acceptance tests** go in `*_acc_test.go` files with `package <name>_test` (external). Use the
  `TestAcc` prefix and `resource.TestCase` with `testutil.ProtoV6ProviderFactories`. Always include
  `PreCheck`, `CheckDestroy`, and an import step.

## Generating documentation

Registry documentation in `docs/` is generated from schema descriptions and the `examples/` directory:

```bash
make docs
```

Re-run this after changing resource schemas, descriptions, or example files, and commit the result.

## Files not in git

| File                         | What it is             | How to get it                              |
|------------------------------|------------------------|--------------------------------------------|
| `terraform-provider-uptrace` | Provider binary        | `go build -o terraform-provider-uptrace .` |
| `.terraformrc` / `.tofurc`   | Dev override config    | Create manually (see above)                |
| `*.tfstate`                  | Terraform state        | Created by `terraform apply`               |
| `.terraform/`                | Provider cache         | Created by `terraform init`                |
| `.terraform.lock.hcl`        | Dependency lock        | Created by `terraform init`                |
| `.env`                       | Local credentials      | `cp .env.example .env`                     |

## Releasing

See [RELEASING.md](RELEASING.md).
