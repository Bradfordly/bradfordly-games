# Internet-facing NLB for Minecraft Java. Health checks use the gateway
# admin /healthz port, never 25565. The gateway Deployment is not created here.

resource "kubernetes_service" "gateway" {
  metadata {
    name      = "gateway"
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name

    labels = {
      "app.kubernetes.io/name"      = "gateway"
      "app.kubernetes.io/part-of"   = "bradfordly-games"
      "app.kubernetes.io/component" = "edge-gateway"
    }

    annotations = {
      "service.beta.kubernetes.io/aws-load-balancer-type"                                = "external"
      "service.beta.kubernetes.io/aws-load-balancer-nlb-target-type"                     = "ip"
      "service.beta.kubernetes.io/aws-load-balancer-scheme"                              = "internet-facing"
      "service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"                = "HTTP"
      "service.beta.kubernetes.io/aws-load-balancer-healthcheck-path"                    = "/healthz"
      "service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"                    = tostring(var.gateway_admin_port)
      "service.beta.kubernetes.io/aws-load-balancer-healthcheck-success-codes"           = "200"
      "service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules" = "true"
    }
  }

  spec {
    type                = "LoadBalancer"
    load_balancer_class = "service.k8s.aws/nlb"

    selector = {
      "app.kubernetes.io/name" = "gateway"
    }

    port {
      name        = "minecraft"
      port        = var.gateway_minecraft_port
      target_port = var.gateway_minecraft_port
      protocol    = "TCP"
    }
  }

  depends_on = [kubernetes_deployment.lbc]
}
