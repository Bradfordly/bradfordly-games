output "cluster_name" {
  description = "EKS cluster name."
  value       = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  description = "EKS API endpoint."
  value       = aws_eks_cluster.this.endpoint
}

output "cluster_security_group_id" {
  description = "Cluster security group used by Fargate pod ENIs."
  value       = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
}

output "oidc_issuer" {
  description = "OIDC issuer URL for later IRSA (EFS CSI, load balancers)."
  value       = aws_eks_cluster.this.identity[0].oidc[0].issuer
}

output "region" {
  description = "AWS region of the cluster."
  value       = var.region
}

output "vpc_id" {
  description = "VPC that holds the cluster."
  value       = aws_vpc.this.id
}

output "private_subnet_ids" {
  description = "Private subnets used by Fargate profiles."
  value       = [for subnet in aws_subnet.private : subnet.id]
}

output "public_subnet_ids" {
  description = "Public subnets for NAT and later ALB/NLB."
  value       = [for subnet in aws_subnet.public : subnet.id]
}

output "panel_hostname" {
  description = "Panel hostname served by the ALB."
  value       = var.panel_hostname
}

output "panel_certificate_arn" {
  description = "ACM certificate attached to the panel ALB."
  value       = aws_acm_certificate.panel.arn
}

output "panel_certificate_validation" {
  description = "DNS records required to validate the panel certificate."
  value = {
    for dvo in aws_acm_certificate.panel.domain_validation_options : dvo.domain_name => {
      name  = dvo.resource_record_name
      type  = dvo.resource_record_type
      value = dvo.resource_record_value
    }
  }
}

output "gateway_minecraft_port" {
  description = "NLB TCP listener for Minecraft Java."
  value       = var.gateway_minecraft_port
}

output "gateway_admin_port" {
  description = "Gateway admin port used for NLB /healthz checks."
  value       = var.gateway_admin_port
}

output "efs_file_system_id" {
  description = "EFS file system for world saves."
  value       = aws_efs_file_system.worlds.id
}

output "efs_access_point_ids" {
  description = "Access point IDs keyed by world ID. Empty until world_ids is set."
  value       = { for id, ap in aws_efs_access_point.world : id => ap.id }
}

output "namespaces" {
  description = "Application namespaces created on the cluster."
  value       = [for ns in kubernetes_namespace.this : ns.metadata[0].name]
}
