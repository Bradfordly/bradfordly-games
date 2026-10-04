# Weekly EBS snapshots of the 50 GB data volume from #57.
# This module does not create the host or the volume. Tag that volume
# Role = var.volume_role_tag (default "data-volume") so DLM can select it.
resource "aws_dlm_lifecycle_policy" "data_volume" {
  description        = "Weekly snapshots of the bradfordly-games data volume"
  execution_role_arn = aws_iam_role.dlm.arn
  state              = "ENABLED"

  policy_details {
    resource_types = ["VOLUME"]
    policy_type    = "EBS_SNAPSHOT_MANAGEMENT"

    target_tags = {
      Role = var.volume_role_tag
    }

    schedule {
      name      = "weekly"
      copy_tags = true

      create_rule {
        interval      = var.snapshot_interval_weeks
        interval_unit = "WEEKS"
        times         = [var.snapshot_time_utc]
      }

      retain_rule {
        count = var.retention_count
      }

      tags_to_add = {
        Purpose = "scheduled"
      }
    }
  }
}
