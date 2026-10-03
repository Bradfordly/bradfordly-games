# Internet-facing ALB for the panel. The Load Balancer Controller creates
# the ALB from this Ingress (IP targets, ACM, /healthz). No panel Deployment
# is created here; the Service selects app.kubernetes.io/name=panel.

resource "kubernetes_service" "panel" {
  metadata {
    name      = "panel"
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name

    labels = {
      "app.kubernetes.io/name"      = "panel"
      "app.kubernetes.io/part-of"   = "bradfordly-games"
      "app.kubernetes.io/component" = "control-plane"
    }
  }

  spec {
    type = "ClusterIP"

    selector = {
      "app.kubernetes.io/name" = "panel"
    }

    port {
      name        = "http"
      port        = 8080
      target_port = 8080
      protocol    = "TCP"
    }
  }
}

resource "kubernetes_ingress_v1" "panel" {
  metadata {
    name      = "panel"
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name

    annotations = {
      "alb.ingress.kubernetes.io/scheme"                              = "internet-facing"
      "alb.ingress.kubernetes.io/target-type"                         = "ip"
      "alb.ingress.kubernetes.io/listen-ports"                        = jsonencode([{ HTTPS = 443 }])
      "alb.ingress.kubernetes.io/certificate-arn"                     = aws_acm_certificate.panel.arn
      "alb.ingress.kubernetes.io/ssl-policy"                          = "ELBSecurityPolicy-TLS13-1-2-2021-06"
      "alb.ingress.kubernetes.io/healthcheck-path"                    = "/healthz"
      "alb.ingress.kubernetes.io/healthcheck-protocol"                = "HTTP"
      "alb.ingress.kubernetes.io/healthcheck-port"                    = "8080"
      "alb.ingress.kubernetes.io/success-codes"                       = "200"
      "alb.ingress.kubernetes.io/manage-backend-security-group-rules" = "true"
    }
  }

  spec {
    ingress_class_name = kubernetes_ingress_class_v1.alb.metadata[0].name

    rule {
      host = var.panel_hostname

      http {
        path {
          path      = "/"
          path_type = "Prefix"

          backend {
            service {
              name = kubernetes_service.panel.metadata[0].name

              port {
                number = 8080
              }
            }
          }
        }
      }
    }
  }

  depends_on = [kubernetes_deployment.lbc]
}
