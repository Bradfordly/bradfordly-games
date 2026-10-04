# EKS OIDC issuer for IRSA. The load balancer controller assumes a role
# from kube-system; Fargate does not support EKS Pod Identity.

resource "aws_iam_openid_connect_provider" "cluster" {
  url            = aws_eks_cluster.this.identity[0].oidc[0].issuer
  client_id_list = ["sts.amazonaws.com"]
}
