locals {
  oidc_issuer_host = replace(aws_eks_cluster.this.identity[0].oidc[0].issuer, "https://", "")
  lbc_sa           = "aws-load-balancer-controller"
  lbc_namespace    = "kube-system"
}

data "aws_iam_policy_document" "lbc_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.cluster.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_issuer_host}:sub"
      values   = ["system:serviceaccount:${local.lbc_namespace}:${local.lbc_sa}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_issuer_host}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lbc" {
  name               = "${var.cluster_name}-lbc"
  assume_role_policy = data.aws_iam_policy_document.lbc_assume.json
}

resource "aws_iam_policy" "lbc" {
  name   = "${var.cluster_name}-lbc"
  policy = file("${path.module}/iam/aws-load-balancer-controller.json")
}

resource "aws_iam_role_policy_attachment" "lbc" {
  role       = aws_iam_role.lbc.name
  policy_arn = aws_iam_policy.lbc.arn
}

resource "kubernetes_service_account" "lbc" {
  metadata {
    name      = local.lbc_sa
    namespace = local.lbc_namespace

    annotations = {
      "eks.amazonaws.com/role-arn" = aws_iam_role.lbc.arn
    }

    labels = {
      "app.kubernetes.io/name"      = local.lbc_sa
      "app.kubernetes.io/component" = "controller"
    }
  }

  depends_on = [aws_iam_role_policy_attachment.lbc]
}

resource "kubernetes_cluster_role" "lbc" {
  metadata {
    name = local.lbc_sa
  }

  rule {
    api_groups = [""]
    resources  = ["endpoints", "events", "namespaces", "nodes", "pods", "secrets", "services"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = [""]
    resources  = ["events"]
    verbs      = ["create", "patch"]
  }

  rule {
    api_groups = ["elbv2.k8s.aws"]
    resources  = ["ingressclassparams", "targetgroupbindings"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["elbv2.k8s.aws"]
    resources  = ["targetgroupbindings"]
    verbs      = ["create", "update", "patch", "delete"]
  }

  rule {
    api_groups = ["elbv2.k8s.aws"]
    resources  = ["targetgroupbindings/status"]
    verbs      = ["update", "patch"]
  }

  rule {
    api_groups = ["networking.k8s.io"]
    resources  = ["ingressclasses", "ingresses"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["networking.k8s.io"]
    resources  = ["ingresses/status"]
    verbs      = ["update", "patch"]
  }

  rule {
    api_groups = ["discovery.k8s.io"]
    resources  = ["endpointslices"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["coordination.k8s.io"]
    resources  = ["leases"]
    verbs      = ["create", "get", "list", "update", "watch"]
  }
}

resource "kubernetes_cluster_role_binding" "lbc" {
  metadata {
    name = local.lbc_sa
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.lbc.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.lbc.metadata[0].name
    namespace = local.lbc_namespace
  }
}

resource "kubernetes_deployment" "lbc" {
  metadata {
    name      = local.lbc_sa
    namespace = local.lbc_namespace

    labels = {
      "app.kubernetes.io/name" = local.lbc_sa
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        "app.kubernetes.io/name" = local.lbc_sa
      }
    }

    template {
      metadata {
        labels = {
          "app.kubernetes.io/name" = local.lbc_sa
        }
      }

      spec {
        service_account_name = kubernetes_service_account.lbc.metadata[0].name

        container {
          name  = "controller"
          image = var.lbc_image

          args = [
            "--cluster-name=${aws_eks_cluster.this.name}",
            "--ingress-class=alb",
            "--aws-vpc-id=${aws_vpc.this.id}",
            "--aws-region=${var.region}",
          ]

          resources {
            requests = {
              cpu    = "100m"
              memory = "200Mi"
            }
          }
        }
      }
    }
  }

  depends_on = [
    aws_eks_fargate_profile.this,
    kubernetes_cluster_role_binding.lbc,
  ]
}

resource "kubernetes_ingress_class_v1" "alb" {
  metadata {
    name = "alb"
  }

  spec {
    controller = "ingress.k8s.aws/alb"
  }
}
