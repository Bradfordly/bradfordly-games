output "dlm_lifecycle_policy_id" {
  description = "Scheduled snapshot policy for the tagged data volume."
  value       = aws_dlm_lifecycle_policy.data_volume.id
}

output "volume_role_tag" {
  description = "Role tag value #57 must set on the 50 GB data volume."
  value       = var.volume_role_tag
}

output "on_demand_snapshot_policy_arn" {
  description = "IAM policy ARN to attach to the #57 instance profile for snapshot-before-delete."
  value       = aws_iam_policy.on_demand_snapshot.arn
}

output "on_demand_snapshot_policy_json" {
  description = "IAM policy JSON if #57 inlines the snapshot-before-delete permissions."
  value       = data.aws_iam_policy_document.on_demand_snapshot.json
}

output "data_volume_id_env" {
  description = "Environment variable the panel reads for on-demand snapshots."
  value       = "GAMES_DATA_VOLUME_ID"
}
