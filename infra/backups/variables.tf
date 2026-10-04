variable "region" {
  type        = string
  description = "AWS region. Cost model in docs/specs/operations.md uses us-east-1."
  default     = "us-east-1"
}

variable "name_prefix" {
  type        = string
  description = "Prefix for DLM and IAM resource names."
  default     = "bradfordly-games"
}

variable "volume_role_tag" {
  type        = string
  description = "Value of the Role tag #57 must set on the 50 GB data volume so DLM can find it."
  default     = "data-volume"
}

variable "snapshot_interval_weeks" {
  type        = number
  description = "Weeks between scheduled snapshots. operations.md cost envelope assumes weekly."
  default     = 1
}

variable "snapshot_time_utc" {
  type        = string
  description = "UTC time of day for the weekly snapshot (HH:MM)."
  default     = "08:00"
}

variable "retention_count" {
  type        = number
  description = "How many scheduled snapshots to keep. Four weekly copies stay near the $2–3/month line in ADR-0006."
  default     = 4
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every AWS resource via the provider default_tags."
  default = {
    Project   = "bradfordly-games"
    ManagedBy = "terraform"
  }
}
