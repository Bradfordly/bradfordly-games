# Panel and gateway share games-system. Worlds stay in games-worlds so they
# cannot read panel secrets. No game workloads are created here.

resource "kubernetes_namespace" "this" {
  for_each = toset(["games-system", "games-worlds"])

  metadata {
    name = each.value

    labels = {
      "app.kubernetes.io/part-of" = "bradfordly-games"
    }
  }

  depends_on = [aws_eks_fargate_profile.this]
}
