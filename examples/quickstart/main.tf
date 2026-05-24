terraform {
  required_providers {
    uptrace = {
      source = "uptrace/uptrace"
    }
  }
}

provider "uptrace" {
  endpoint = var.endpoint
  token    = var.token
}

# --- Organization & project ---

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

# --- Monitoring ---

resource "uptrace_error_monitor" "log_errors" {
  project_id = uptrace_project.api.id
  name       = "Notify on all errors"

  params = {
    metrics = [
      { name = "uptrace_tracing_logs", alias = "$logs" }
    ]
    query = "sum($logs) | where _system in (\"log:error\", \"log:fatal\")"
  }
}

resource "uptrace_webhook_channel" "alerts" {
  project_id = uptrace_project.api.id
  name       = "alerts-webhook"
  priorities = ["high", "medium"]
  url        = var.webhook_url
}
