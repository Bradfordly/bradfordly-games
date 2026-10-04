data "aws_iam_policy_document" "dlm_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["dlm.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dlm" {
  name               = "${var.name_prefix}-dlm"
  assume_role_policy = data.aws_iam_policy_document.dlm_assume.json
}

resource "aws_iam_role_policy_attachment" "dlm" {
  role       = aws_iam_role.dlm.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSDataLifecycleManagerServiceRole"
}

data "aws_iam_policy_document" "on_demand_snapshot" {
  statement {
    sid = "CreatePreDeleteSnapshot"

    actions = [
      "ec2:CreateSnapshot",
      "ec2:CreateTags",
      "ec2:DescribeSnapshots",
      "ec2:DescribeVolumes",
    ]

    resources = ["*"]
  }
}

# Attach this policy to the instance profile from #57 so the panel can
# snapshot the data volume before a world's save directory is deleted.
resource "aws_iam_policy" "on_demand_snapshot" {
  name   = "${var.name_prefix}-on-demand-snapshot"
  policy = data.aws_iam_policy_document.on_demand_snapshot.json
}
