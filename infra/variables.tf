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

variable "tags" {
  type        = map(string)
  description = "Tags applied to every AWS resource via the provider default_tags."
  default = {
    Project   = "bradfordly-games"
    ManagedBy = "terraform"
  }
}
