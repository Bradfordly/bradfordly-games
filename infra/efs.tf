# One regional file system for world saves. Fargate mounts EFS through static
# PersistentVolumes (ADR-0004). Access points can be created here or by hand.

resource "aws_efs_file_system" "worlds" {
  encrypted        = true
  performance_mode = "generalPurpose"
  throughput_mode  = "bursting"

  tags = {
    Name = "${var.cluster_name}-worlds"
  }
}

resource "aws_security_group" "efs" {
  name        = "${var.cluster_name}-efs"
  description = "NFS from EKS Fargate pods"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.cluster_name}-efs"
  }
}

resource "aws_vpc_security_group_ingress_rule" "efs_nfs" {
  security_group_id            = aws_security_group.efs.id
  description                  = "NFSv4 from the cluster security group"
  from_port                    = 2049
  to_port                      = 2049
  ip_protocol                  = "tcp"
  referenced_security_group_id = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
}

resource "aws_vpc_security_group_egress_rule" "efs_all" {
  security_group_id = aws_security_group.efs.id
  description       = "Allow all egress"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_efs_mount_target" "worlds" {
  for_each = aws_subnet.private

  file_system_id  = aws_efs_file_system.worlds.id
  subnet_id       = each.value.id
  security_groups = [aws_security_group.efs.id]
}

# Convention: /worlds/<world_id>, uid/gid 1000 (itzg/minecraft-server).
# The access point is the volume root, so the pod mounts the world directory.
resource "aws_efs_access_point" "world" {
  for_each = toset(var.world_ids)

  file_system_id = aws_efs_file_system.worlds.id

  root_directory {
    path = "/worlds/${each.value}"

    creation_info {
      owner_uid   = 1000
      owner_gid   = 1000
      permissions = "755"
    }
  }

  posix_user {
    uid = 1000
    gid = 1000
  }

  tags = {
    Name  = "${var.cluster_name}-world-${each.value}"
    World = each.value
  }
}
