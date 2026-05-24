# Quickstart

This example creates an organization, a project with 30-day span retention, an ingest token,
an error monitor, and a webhook notification channel.

## Usage

Set your API token (get one from [Uptrace](https://app.uptrace.dev)):

```bash
export TF_VAR_token="YOUR_API_TOKEN"
```

Then run:

```bash
terraform init
terraform plan
terraform apply
```

To retrieve the project ingest DSN:

```bash
terraform output -raw project_dsn
```

Pass the DSN to your OpenTelemetry SDK to start sending telemetry data.

## Variables

| Name          | Description                       | Default                    |
|---------------|-----------------------------------|----------------------------|
| `endpoint`    | Uptrace API endpoint URL          | `https://api.uptrace.dev`  |
| `token`       | API authentication token          | —                          |
| `webhook_url` | URL to receive alert notifications | `https://example.com/hooks/alert` |

## Customization

- To change the alerting destination, replace the `uptrace_webhook_channel` resource with any
  other channel type (Slack, PagerDuty, Telegram, etc.). See the
  [provider docs](../../README.md#notification-channels) for all options.
- To remove alerting entirely, delete the `uptrace_error_monitor` and `uptrace_webhook_channel`
  resources and the `webhook_url` variable.
