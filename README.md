# terraform-provider-uptrace

Minimal Terraform provider for managing Uptrace resources.

## Build

```bash
go build -o terraform-provider-uptrace .
```

## Setup

Create a dev override file so Terraform uses the local binary:

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

Export it:

```bash
export TF_CLI_CONFIG_FILE=/path/to/terraform-provider-uptrace/.terraformrc
```

## Usage

```hcl
terraform {
  required_providers {
    uptrace = {
      source = "uptrace/uptrace"
    }
  }
}

provider "uptrace" {
  endpoint = "http://localhost:14318"
  token    = "user1_secret"
}

resource "uptrace_org" "org1" {
  name   = "Org1"
  budget = 100
}
```

Provider config can also be set via environment variables:

- `UPTRACE_ENDPOINT`
- `UPTRACE_TOKEN`

## Testing

The project has two layers of tests:

### Unit tests

Unit tests run without any external dependencies and are always safe to run:

```bash
make test
```

These cover pure mapping and request-building helpers plus import and ID-validation paths, and run in CI on every push and pull request.

### Acceptance tests

Acceptance tests exercise the full Terraform lifecycle (plan, apply, import,
destroy) against a real Uptrace API. They follow the
[HashiCorp acceptance test conventions](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests)
and are gated behind the `TF_ACC` environment variable.

1. Copy the credentials template and fill it in:

```bash
cp .env.example .env
```

2. Run acceptance tests:

```bash
make testacc
```

Without `TF_ACC=1`, acceptance tests are automatically skipped.

### Writing tests

- **Unit tests** go in `*_unit_test.go` files with `package <name>` (internal).
  Use these for pure functions that don't need a running API.
- **Acceptance tests** go in `*_acc_test.go` files with `package <name>_test` (external).
  Use the `TestAcc` prefix and `resource.TestCase` with `testutil.ProtoV6ProviderFactories`.
  Always include `PreCheck`, `CheckDestroy`, and an import step.

## Commands

```bash
terraform plan      # preview changes
terraform apply     # create/update resources
terraform destroy   # delete resources
terraform state list   # list managed resources
```

## Resources

### uptrace_org

Manages an Uptrace organization.

| Field  | Type   | Required | Note                           |
|--------|--------|----------|--------------------------------|
| name   | string | yes      | Updatable                      |
| budget | float  | no       | Updatable. Uses the API default when omitted on create. Removing it later keeps the current API budget because Uptrace does not expose an unset/reset operation. |
| id     | string | computed |                                |

### uptrace_project

Manages an Uptrace project scoped under an organization.

| Field                  | Type   | Required | Note                                                                        |
|------------------------|--------|----------|-----------------------------------------------------------------------------|
| org_id                 | string | yes      | Forces replacement on change.                                               |
| name                   | string | yes      | Updatable.                                                                  |
| group_by_env           | bool   | no       | Updatable.                                                                  |
| group_funcs_by_service | bool   | no       | Updatable.                                                                  |
| semconv_version        | string | no       | One of `none`, `v1.25.0`, `v1.33.0`.                                        |
| display_log_severity   | bool   | no       | Updatable.                                                                  |
| count_distinct         | bool   | no       | Updatable.                                                                  |
| span_time_range        | string | no       | Duration (e.g. `"24h"`). Default query time range for spans.                |
| log_time_range         | string | no       | Duration. Default query time range for logs.                                |
| event_time_range       | string | no       | Duration. Default query time range for events.                              |
| span_retention         | string | no       | Duration (e.g. `"30d"`, `"4w"`). Server minimum applies.                    |
| log_retention          | string | no       | Duration.                                                                   |
| event_retention        | string | no       | Duration.                                                                   |
| metric_retention       | string | no       | Duration.                                                                   |
| id                     | string | computed |                                                                             |

Duration strings accept the stdlib units `ns`, `us`, `ms`, `s`, `m`, `h` plus `d` (day) and `w` (week).

### uptrace_project_token

Manages an ingest token for an Uptrace project.

| Field      | Type   | Required | Note                                                              |
|------------|--------|----------|-------------------------------------------------------------------|
| project_id | string | yes      | Forces replacement on change.                                     |
| name       | string | no       | Updatable. Removing the attribute clears the name on the server.  |
| id         | string | computed |                                                                   |
| token      | string | computed | Sensitive. Generated server-side; never supplied by the user.     |
| dsn        | string | computed | Sensitive. Ingest URL with the token embedded.                    |

Import with `<project_id>:<token_id>`.

### uptrace_team / uptrace_team_project / uptrace_team_user

Teams group users for project-scoped access control within an organization. Teams are a Premium feature; the backend returns `403` for unlicensed organizations.

`uptrace_team` manages the team itself. `uptrace_team_project` and `uptrace_team_user` attach projects and users to a team; each is a separate membership resource with the composite identity `(team_id, project_id)` or `(team_id, org_user_id)`.

#### uptrace_team

Manages a team within an organization.

| Field      | Type   | Required | Note                                                                                   |
|------------|--------|----------|----------------------------------------------------------------------------------------|
| org_id     | string | yes      | Forces replacement on change.                                                          |
| name       | string | yes      | Updatable. 1–255 characters.                                                           |
| perm_level | string | no       | Updatable. One of `none`, `view`, `edit`, `admin`. Computed when omitted — the backend fills the default (currently `view`). Removing the attribute preserves the current value; it cannot be cleared back to null via Terraform. |
| id         | string | computed |                                                                                        |

Import with `<org_id>:<team_id>`.

#### uptrace_team_project

Grants a team access to a project. Idempotent: creating twice is a no-op server-side. There is no update — changing any field forces replacement.

| Field      | Type   | Required | Note                                         |
|------------|--------|----------|----------------------------------------------|
| org_id     | string | yes      | Forces replacement on change.                |
| team_id    | string | yes      | Forces replacement on change.                |
| project_id | string | yes      | Forces replacement on change.                |
| id         | string | computed | Equals `project_id`. Unique within the team. |

Import with `<org_id>:<team_id>:<project_id>`.

#### uptrace_team_user

Adds an organization user to a team. Idempotent. There is no update — changing any field forces replacement.

`org_user_id` is the ID of the OrgUser record linking the user to the organization (not the User ID). The API does not expose a way to create OrgUsers; invite users through the Uptrace UI first, then reference the resulting `org_user_id` here.

| Field       | Type   | Required | Note                                             |
|-------------|--------|----------|--------------------------------------------------|
| org_id      | string | yes      | Forces replacement on change.                    |
| team_id     | string | yes      | Forces replacement on change.                    |
| org_user_id | string | yes      | Forces replacement on change.                    |
| id          | string | computed | Equals `org_user_id`. Unique within the team.    |

Import with `<org_id>:<team_id>:<org_user_id>`.

### Notification channels

Each notification-channel type is its own resource. All resources share the same top-level fields; the type-specific fields differ per resource.

Shared fields (all 12 resources):

| Field       | Type         | Required | Note                                                                         |
|-------------|--------------|----------|------------------------------------------------------------------------------|
| project_id  | string       | yes      | Forces replacement on change.                                                |
| name        | string       | yes      | Updatable.                                                                   |
| priorities  | list(string) | yes      | Alert priorities to match. Each one of `info`, `low`, `medium`, `high`.      |
| match_all   | bool         | no       | Defaults to `true`. When `false`, `monitor_ids` must be set and non-empty.   |
| monitor_ids | list(string) | no       | Required when `match_all = false`.                                           |
| condition   | string       | no       | Alert condition expression.                                                  |
| id          | string       | computed |                                                                              |
| status      | string       | computed | One of `delivering`, `paused`, `disabled`, `draft`.                          |

Type-specific fields per resource:

| Resource                         | Fields                                                                                                                           |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| `uptrace_slack_channel`          | `auth_method` (`webhook` or `token`), `webhook_url`, `token`, `channel`. Fields required depend on `auth_method`.                |
| `uptrace_google_chat_channel`    | `webhook_url`                                                                                                                    |
| `uptrace_mattermost_channel`     | `webhook_url`                                                                                                                    |
| `uptrace_teams_channel`          | `webhook_url`                                                                                                                    |
| `uptrace_pagerduty_channel`      | `routing_key`, `severity` (`critical`, `error`, `warning`, `info`)                                                               |
| `uptrace_opsgenie_channel`       | `api_key`, `priority` (`P1`–`P5`)                                                                                                |
| `uptrace_telegram_channel`       | `chat_id` (int64)                                                                                                                |
| `uptrace_pushover_channel`       | `token`, `user_key`; optional `priority` (int, -2 to 2), `sound`                                                                 |
| `uptrace_webhook_channel`        | `url`; optional `payload` (JSON object string — use `jsonencode()`)                                                              |
| `uptrace_alertmanager_channel`   | `url`; optional `auth_method` (`none`, `basic_auth`, `bearer`), `username`, `password`, `token`. Credential fields required depend on `auth_method`. |
| `uptrace_incidentio_channel`     | `url`, `api_key`                                                                                                                 |
| `uptrace_servicenow_channel`     | `url`, `username`, `password`; optional `category`, `subcategory`, `impact` (`1`-`3`), `urgency` (`1`-`3`), `severity` (`1`-`5`), `caller_id`, `group`, `assigned_to`, `opened_by`, `notify` (`1` or `2`), `due_date` |

Import each resource with `<project_id>:<channel_id>`.

### uptrace_error_monitor / uptrace_metric_monitor

Error and metric monitors are exposed as two separate resources that share the same set of top-level fields. Each takes a single `params` block whose shape differs per resource.

Shared fields (both resources):

| Field                     | Type              | Required | Note                                                                                                |
|---------------------------|-------------------|----------|-----------------------------------------------------------------------------------------------------|
| project_id                | string            | yes      | Forces replacement on change.                                                                       |
| name                      | string            | yes      | Updatable.                                                                                          |
| notify_everyone_by_email  | bool              | no       | Defaults to `false`. Updatable.                                                                     |
| trend_agg_func            | string            | no       | Defaults to `sum`. One of `sum`, `avg`, `median`, `last`.                                           |
| trend_sensitivity         | string            | no       | Defaults to `medium`. One of `low`, `medium`, `high`.                                               |
| team_ids                  | set(string)       | no       | Team IDs to notify when the monitor fires.                                                          |
| channel_ids               | set(string)       | no       | Notification channel IDs (e.g. `uptrace_slack_channel.x.id`). No `tonumber()` wrapper needed.       |
| id                        | string            | computed |                                                                                                     |
| status                    | string            | computed | One of `active`, `paused`, `firing`, `no_data`, `disabled`.                                         |

#### uptrace_error_monitor

Watches trend anomalies in an MQL query and fires alerts through attached notification channels and/or team email.

`params`:

| Field                     | Type              | Required | Note                                                                                                |
|---------------------------|-------------------|----------|-----------------------------------------------------------------------------------------------------|
| metrics                   | list of objects   | yes      | At least one metric. Each: `{ name = "...", alias = "$..." }`. Aliases must start with `$`.         |
| query                     | string            | yes      | MQL query expression. The backend normalizes MQL; the provider preserves the user's input form.     |

#### uptrace_metric_monitor

Evaluates an MQL query on a schedule with a manual threshold or automatic trend-based detector.

`params`:

| Field           | Type            | Required | Note                                                                                               |
|-----------------|-----------------|----------|----------------------------------------------------------------------------------------------------|
| metrics         | list of objects | yes      | At least one metric. Each: `{ name = "...", alias = "$..." }`.                                     |
| query           | string          | yes      | MQL query expression.                                                                              |
| column          | object          | no       | `{ name = "...", unit = "milliseconds" }`. The result column the detector evaluates.               |
| resolution      | number          | no       | Evaluation resolution in milliseconds.                                                             |
| num_eval_points | number          | no       | Number of consecutive evaluation points that must breach the threshold.                            |
| absent_points   | string          | no       | One of `ignore`, `alert`, `zero`.                                                                  |
| time_offset     | number          | no       | Time offset in milliseconds applied to the query before evaluation.                                |
| detector        | object          | yes      | Exactly one of `manual {}` or `auto {}`.                                                           |

`params.detector.manual`:

| Field      | Type   | Required | Note                                                                 |
|------------|--------|----------|----------------------------------------------------------------------|
| min_value  | number | no       | Alert when value falls below this threshold.                         |
| max_value  | number | no       | Alert when value rises above this threshold.                         |
| recovery   | object | no       | Hysteresis `{ min_value, max_value }` used to clear an active alert. |

`params.detector.auto`:

| Field            | Type   | Required | Note                                               |
|------------------|--------|----------|----------------------------------------------------|
| tolerance        | string | no       | One of `low`, `medium`, `high`.                    |
| training_period  | number | no       | Training period in milliseconds.                   |
| min_dev_fraction | number | no       | Minimum deviation as a fraction of the baseline.   |
| min_dev_absolute | number | no       | Minimum absolute deviation from the baseline.      |

Not yet exposed on either resource: `repeat_interval` (shared oneOf of `default` / `fixed` / `linear` / `exponential`). Follow-up work.

## Data sources

### uptrace_org_user

Resolves an existing organization user to their `OrgUser` ID, so you can reference them from membership resources like `uptrace_team_user`. Membership is managed outside Terraform — the user must already be a member of the organization at plan time. Errors if no match is found.

| Field   | Type   | Required | Note                                                                    |
|---------|--------|----------|-------------------------------------------------------------------------|
| org_id  | string | yes      | Organization to search within.                                          |
| email   | string | yes      | Matched case-insensitively against the server value.                    |
| id      | string | computed | OrgUser ID. Use where another resource wants an `org_user_id`.          |
| user_id | string | computed | Underlying User ID (distinct from `id`).                                |
| role    | string | computed | `owner`, `admin`, `member`, `viewer`, `billing_manager`, `collaborator`.|
| name    | string | computed | Display name from the user's profile.                                   |

## Files not in git

The `.gitignore` excludes files generated locally:

| File | What it is | How to get it |
|------|-----------|---------------|
| `terraform-provider-uptrace` | Binary | `go build -o terraform-provider-uptrace .` |
| `.terraformrc` / `.tofurc` | Dev override config | Create manually (see Setup) |
| `*.tfstate` | Terraform state | Created by `terraform apply` |
| `.terraform/` | Provider cache | Created by `terraform init` |
| `.terraform.lock.hcl` | Dependency lock | Created by `terraform init` |
| `.env` | Local credentials | `cp .env.example .env` |
