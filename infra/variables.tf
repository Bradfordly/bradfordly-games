variable "region" {
  type        = string
  description = "AWS region for the cluster. Cost model in docs/specs/operations.md uses us-east-1."
  default     = "us-east-1"
}

variable "cluster_name" {
  type        = string
  description = "EKS cluster name. Also used to prefix VPC and IAM resources."
  default     = "bradfordly-games"
}

variable "kubernetes_version" {
  type        = string
  description = "EKS Kubernetes version."
  default     = "1.33"
}

variable "vpc_cidr" {
  type        = string
  description = "VPC CIDR. Public subnets are 10/8 index 0-1; private are 10/8 index 10-11."
  default     = "10.42.0.0/16"
}

variable "cluster_endpoint_public_access_cidrs" {
  type        = list(string)
  description = "CIDRs that may reach the public EKS API. Lock this down after first apply."
  default     = ["0.0.0.0/0"]
}

variable "panel_hostname" {
  type        = string
  description = "Public hostname for the panel ALB and ACM certificate."
  default     = "games.bradfordly.com"
}

variable "route53_zone_id" {
  type        = string
  description = "Optional Route 53 zone for ACM DNS validation. Empty leaves validation records as outputs."
  default     = ""
}

variable "gateway_minecraft_port" {
  type        = number
  description = "Public NLB listener and gateway Minecraft TCP port."
  default     = 25565
}

variable "gateway_admin_port" {
  type        = number
  description = "Gateway admin HTTP port for NLB and Kubernetes health checks. Must not be the Minecraft port."
  default     = 8080

  validation {
    condition     = var.gateway_admin_port != var.gateway_minecraft_port
    error_message = "NLB health checks must not use the Minecraft port (25565)."
  }
}

variable "lbc_image" {
  type        = string
  description = "AWS Load Balancer Controller image. Runs as a Deployment on Fargate, not a DaemonSet."
  default     = "public.ecr.aws/eks/aws-load-balancer-controller:v3.5.0"
}

variable "world_ids" {
  type        = list(string)
  description = "World IDs that get an EFS access point at /worlds/<id>. Leave empty and create access points by hand for the first worlds."
  default     = []
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every AWS resource via the provider default_tags."
  default = {
    Project   = "bradfordly-games"
    ManagedBy = "terraform"
  }
}
