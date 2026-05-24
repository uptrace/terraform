# Terraform Provider for Uptrace

Manage [Uptrace](https://uptrace.dev) organizations, projects, users, teams, monitors, and
notification channels with Terraform.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.0

## Quick start

```hcl
terraform {
  required_providers {
    uptrace = {
      source = "uptrace/uptrace"
    }
  }
}

provider "uptrace" {
  endpoint = "https://api.uptrace.dev"
  token    = var.uptrace_token
}

resource "uptrace_org" "main" {
  name = "My Organization"
}

resource "uptrace_project" "api" {
  org_id         = uptrace_org.main.id
  name           = "api"
  span_retention = "30d"
}

resource "uptrace_project_token" "api" {
  project_id = uptrace_project.api.id
  name       = "default"
}
```

```bash
terraform init
terraform plan
terraform apply
```

## Provider configuration

| Argument   | Environment variable | Description              |
|------------|----------------------|--------------------------|
| `endpoint` | `UPTRACE_ENDPOINT`   | Uptrace API endpoint URL |
| `token`    | `UPTRACE_TOKEN`      | API authentication token |

Arguments take precedence over environment variables.

## Resources

### uptrace_org

Manages an organization.

| Field  | Type   | Required | Description                        |
|--------|--------|----------|------------------------------------|
| name   | string | yes      | Organization name. Updatable.      |
| budget | float  | no       | Budget limit. Updatable. Defaults to the server value when omitted; removing it later preserves the current value. |
| id     | string | computed |                                    |

### uptrace_project

Manages a project scoped under an organization.

| Field                  | Type   | Required | Description                                                      |
|------------------------|--------|----------|------------------------------------------------------------------|
| org_id                 | string | yes      | Organization ID. Forces replacement on change.                   |
| name                   | string | yes      | Project name. Updatable.                                         |
| group_by_env           | bool   | no       | Group spans by environment. Updatable.                           |
| group_funcs_by_service | bool   | no       | Group functions by service. Updatable.                           |
| semconv_version        | string | no       | One of `none`, `v1.25.0`, `v1.33.0`.                            |
| display_log_severity   | bool   | no       | Updatable.                                                       |
| count_distinct         | bool   | no       | Updatable.                                                       |
| span_time_range        | string | no       | Default query time range for spans (e.g. `"24h"`).              |
| log_time_range         | string | no       | Default query time range for logs.                               |
| event_time_range       | string | no       | Default query time range for events.                             |
| span_retention         | string | no       | Data retention for spans (e.g. `"30d"`, `"4w"`). Server minimum applies. |
| log_retention          | string | no       | Data retention for logs.                                         |
| event_retention        | string | no       | Data retention for events.                                       |
| metric_retention       | string | no       | Data retention for metrics.                                      |
| id                     | string | computed |                                                                  |

Duration strings accept `ns`, `us`, `ms`, `s`, `m`, `h`, `d` (day), and `w` (week).

### uptrace_project_token

Manages an ingest token for a project.

| Field      | Type   | Required | Description                                               |
|------------|--------|----------|-----------------------------------------------------------|
| project_id | string | yes      | Project ID. Forces replacement on change.                 |
| name       | string | no       | Token name. Updatable. Removing it clears the name.       |
| id         | string | computed |                                                           |
| token      | string | computed | Sensitive. The generated ingest token.                    |
| dsn        | string | computed | Sensitive. Ingest URL with the token embedded.            |

Import: `terraform import uptrace_project_token.x <project_id>:<token_id>`

### uptrace_user

Invites a user to Uptrace. The backend sends an account-invitation email. If the email already
belongs to a confirmed user, the existing user is adopted (no duplicate is created).

| Field | Type   | Required | Description                                               |
|-------|--------|----------|-----------------------------------------------------------|
| email | string | yes      | Lowercase, trimmed. Forces replacement on change.         |
| id    | string | computed | Numeric user ID.                                          |

Delete removes the resource from state only -- the underlying user remains on the server.

### uptrace_org_user

Adds a user to an organization with a role. Pair with `uptrace_user` to manage the full
invite-to-membership lifecycle.

| Field   | Type   | Required | Description                                                                    |
|---------|--------|----------|--------------------------------------------------------------------------------|
| org_id  | string | yes      | Organization ID. Forces replacement on change.                                 |
| user_id | string | yes      | User ID (from `uptrace_user`). Forces replacement on change.                   |
| role    | string | yes      | One of `owner`, `admin`, `member`, `viewer`, `billing_manager`, `collaborator`. Updatable. |
| id      | string | computed | OrgUser ID. Pass to `uptrace_team_user.org_user_id`.                           |

Import: `terraform import uptrace_org_user.x <org_id>:<org_user_id>`

**Example -- invite a user and grant org membership:**

```hcl
resource "uptrace_user" "alice" {
  email = "alice@example.com"
}

resource "uptrace_org_user" "alice" {
  org_id  = uptrace_org.main.id
  user_id = uptrace_user.alice.id
  role    = "admin"
}
```

### uptrace_team

Manages a team within an organization. Teams are a **Premium** feature.

| Field      | Type   | Required | Description                                                    |
|------------|--------|----------|----------------------------------------------------------------|
| org_id     | string | yes      | Organization ID. Forces replacement on change.                 |
| name       | string | yes      | Team name (1-255 characters). Updatable.                       |
| perm_level | string | no       | One of `none`, `view`, `edit`, `admin`. Server fills the default when omitted. |
| id         | string | computed |                                                                |

Import: `terraform import uptrace_team.x <org_id>:<team_id>`

### uptrace_team_project

Grants a team access to a project. All fields force replacement (no in-place update).

| Field      | Type   | Required | Description                           |
|------------|--------|----------|---------------------------------------|
| org_id     | string | yes      | Organization ID.                      |
| team_id    | string | yes      | Team ID.                              |
| project_id | string | yes      | Project ID.                           |
| id         | string | computed | Equals `project_id`.                  |

Import: `terraform import uptrace_team_project.x <org_id>:<team_id>:<project_id>`

### uptrace_team_user

Adds an organization user to a team. All fields force replacement.

| Field       | Type   | Required | Description                                     |
|-------------|--------|----------|-------------------------------------------------|
| org_id      | string | yes      | Organization ID.                                |
| team_id     | string | yes      | Team ID.                                        |
| org_user_id | string | yes      | OrgUser ID (from `uptrace_org_user.id`).        |
| id          | string | computed | Equals `org_user_id`.                           |

Import: `terraform import uptrace_team_user.x <org_id>:<team_id>:<org_user_id>`

### Notification channels

Each notification-channel type is its own resource. All share a common set of fields plus
type-specific configuration.

**Shared fields:**

| Field       | Type         | Required | Description                                                           |
|-------------|--------------|----------|-----------------------------------------------------------------------|
| project_id  | string       | yes      | Project ID. Forces replacement on change.                             |
| name        | string       | yes      | Channel name. Updatable.                                              |
| priorities  | list(string) | yes      | Alert priorities to match: `info`, `low`, `medium`, `high`.          |
| match_all   | bool         | no       | Defaults to `true`. Set `false` to restrict to `monitor_ids`.        |
| monitor_ids | list(string) | no       | Required when `match_all = false`.                                    |
| condition   | string       | no       | Alert condition expression.                                           |
| id          | string       | computed |                                                                       |
| status      | string       | computed | One of `delivering`, `paused`, `disabled`, `draft`.                  |

**Type-specific fields:**

| Resource                         | Fields                                                                                                                           |
|----------------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| `uptrace_slack_channel`          | `auth_method` (`webhook` or `token`), `webhook_url`, `token`, `channel`. Required fields depend on `auth_method`.                |
| `uptrace_google_chat_channel`    | `webhook_url`                                                                                                                    |
| `uptrace_mattermost_channel`     | `webhook_url`                                                                                                                    |
| `uptrace_teams_channel`          | `webhook_url`                                                                                                                    |
| `uptrace_pagerduty_channel`      | `routing_key`, `severity` (`critical`, `error`, `warning`, `info`)                                                               |
| `uptrace_opsgenie_channel`       | `api_key`, `priority` (`P1`-`P5`)                                                                                                |
| `uptrace_telegram_channel`       | `chat_id` (int64)                                                                                                                |
| `uptrace_pushover_channel`       | `token`, `user_key`; optional `priority` (int, -2 to 2), `sound`                                                                |
| `uptrace_webhook_channel`        | `url`; optional `payload` (JSON string -- use `jsonencode()`)                                                                    |
| `uptrace_alertmanager_channel`   | `url`; optional `auth_method` (`none`, `basic_auth`, `bearer`), `username`, `password`, `token`                                  |
| `uptrace_incidentio_channel`     | `url`, `api_key`                                                                                                                 |
| `uptrace_servicenow_channel`     | `url`, `username`, `password`; optional `category`, `subcategory`, `impact` (`1`-`3`), `urgency` (`1`-`3`), `severity` (`1`-`5`), `caller_id`, `group`, `assigned_to`, `opened_by`, `notify` (`1` or `2`), `due_date` |

Import any channel: `terraform import uptrace_<type>_channel.x <project_id>:<channel_id>`

**Example -- Slack webhook:**

```hcl
resource "uptrace_slack_channel" "alerts" {
  project_id = uptrace_project.api.id
  name       = "alerts-slack"
  priorities = ["high", "medium"]

  auth_method = "webhook"
  webhook_url = "https://hooks.slack.com/services/T00/B00/XXXX"
}
```

### uptrace_error_monitor

Watches trend anomalies in an MQL query and fires alerts.

### uptrace_metric_monitor

Evaluates an MQL query on a schedule with a manual threshold or automatic trend-based detector.

**Shared fields (both monitors):**

| Field                     | Type        | Required | Description                                                              |
|---------------------------|-------------|----------|--------------------------------------------------------------------------|
| project_id                | string      | yes      | Project ID. Forces replacement on change.                                |
| name                      | string      | yes      | Monitor name. Updatable.                                                 |
| notify_everyone_by_email  | bool        | no       | Defaults to `false`. Updatable.                                          |
| trend_agg_func            | string      | no       | One of `sum`, `avg`, `median`, `last`. Defaults to `sum`.                |
| trend_sensitivity         | string      | no       | One of `low`, `medium`, `high`. Defaults to `medium`.                    |
| team_ids                  | set(string) | no       | Team IDs to notify.                                                      |
| channel_ids               | set(string) | no       | Notification channel IDs.                                                |
| id                        | string      | computed |                                                                          |
| status                    | string      | computed | One of `active`, `paused`, `firing`, `no_data`, `disabled`.              |

**`params` block -- error monitor:**

| Field   | Type            | Required | Description                                                    |
|---------|-----------------|----------|----------------------------------------------------------------|
| metrics | list of objects | yes      | `{ name = "...", alias = "$..." }`. Aliases must start with `$` followed by an identifier. |
| query   | string          | yes      | MQL query expression.                                          |

**`params` block -- metric monitor:**

| Field           | Type            | Required | Description                                                    |
|-----------------|-----------------|----------|----------------------------------------------------------------|
| metrics         | list of objects | yes      | `{ name = "...", alias = "$..." }`. Aliases must start with `$` followed by an identifier. |
| query           | string          | yes      | MQL query expression.                                          |
| column          | object          | no       | `{ name = "...", unit = "milliseconds" }`.                     |
| resolution      | number          | no       | Evaluation resolution in milliseconds.                         |
| num_eval_points | number          | no       | Consecutive breach points before alerting.                     |
| absent_points   | string          | no       | One of `ignore`, `alert`, `zero`.                              |
| time_offset     | number          | no       | Time offset in milliseconds.                                   |
| detector        | object          | yes      | Exactly one of `manual {}` or `auto {}`.                       |

`detector.manual`: optional `min_value`, `max_value`, `recovery { min_value, max_value }`.

`detector.auto`: optional `tolerance` (`low`/`medium`/`high`), `training_period` (ms),
`min_dev_fraction`, `min_dev_absolute`.

Import either monitor: `terraform import uptrace_<error|metric>_monitor.x <project_id>:<monitor_id>`

## Importing existing resources

To bring existing Uptrace resources under Terraform management, use `terraform import`.
The import ID format is listed under each resource above.

```bash
terraform import uptrace_org.main 123
terraform import uptrace_project.api 456
terraform import uptrace_slack_channel.alerts 456:789
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for building from source, running tests, and development
setup.
