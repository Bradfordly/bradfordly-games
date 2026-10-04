# Panel and gateway ServiceAccounts live with the pods in games-system.
# Roles stay in games-worlds so neither account is cluster-wide.

resource "kubernetes_service_account" "panel" {
  metadata {
    name      = "panel"
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name

    labels = {
      "app.kubernetes.io/name"    = "panel"
      "app.kubernetes.io/part-of" = "bradfordly-games"
    }
  }
}

resource "kubernetes_service_account" "gateway" {
  metadata {
    name      = "gateway"
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name

    labels = {
      "app.kubernetes.io/name"    = "gateway"
      "app.kubernetes.io/part-of" = "bradfordly-games"
    }
  }
}

resource "kubernetes_role" "panel" {
  metadata {
    name      = "panel"
    namespace = kubernetes_namespace.this["games-worlds"].metadata[0].name
  }

  rule {
    api_groups = ["games.bradfordly.com"]
    resources  = ["worlds"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }

  rule {
    api_groups = [""]
    resources  = ["services", "persistentvolumeclaims"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }

  rule {
    api_groups = ["apps"]
    resources  = ["statefulsets", "deployments"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }

  rule {
    api_groups = [""]
    resources  = ["pods"]
    verbs      = ["get", "list", "watch"]
  }
}

resource "kubernetes_role" "gateway" {
  metadata {
    name      = "gateway"
    namespace = kubernetes_namespace.this["games-worlds"].metadata[0].name
  }

  rule {
    api_groups = ["games.bradfordly.com"]
    resources  = ["worlds"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = [""]
    resources  = ["services", "persistentvolumeclaims"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["apps"]
    resources  = ["statefulsets", "deployments"]
    verbs      = ["get", "list", "watch", "patch"]
  }

  rule {
    api_groups = ["apps"]
    resources  = ["statefulsets/scale", "deployments/scale"]
    verbs      = ["get", "patch", "update"]
  }
}

resource "kubernetes_role_binding" "panel" {
  metadata {
    name      = "panel"
    namespace = kubernetes_namespace.this["games-worlds"].metadata[0].name
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.panel.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.panel.metadata[0].name
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name
  }
}

resource "kubernetes_role_binding" "gateway" {
  metadata {
    name      = "gateway"
    namespace = kubernetes_namespace.this["games-worlds"].metadata[0].name
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.gateway.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.gateway.metadata[0].name
    namespace = kubernetes_namespace.this["games-system"].metadata[0].name
  }
}
