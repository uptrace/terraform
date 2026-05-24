output "project_dsn" {
  description = "Ingest DSN for the project — pass this to your OpenTelemetry SDK"
  value       = uptrace_project_token.api.dsn
  sensitive   = true
}
