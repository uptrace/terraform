variable "endpoint" {
  description = "Uptrace API endpoint URL"
  type        = string
  default     = "https://api.uptrace.dev"
}

variable "token" {
  description = "Uptrace API authentication token"
  type        = string
  sensitive   = true
}

variable "webhook_url" {
  description = "URL to receive alert notifications (remove the webhook channel resource if not needed)"
  type        = string
  default     = "https://example.com/hooks/alert"
}
