import shutil
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INFRA = ROOT / "infra"


def infra_text() -> str:
    parts = [path.read_text() for path in sorted(INFRA.glob("*.tf"))]
    parts.extend(path.read_text() for path in sorted((INFRA / "k8s").glob("**/*")) if path.is_file())
    return "\n".join(parts)


class EksFargateCluster(unittest.TestCase):
    def test_fargate_profiles_cover_required_namespaces(self) -> None:
        text = infra_text()
        for namespace in ("kube-system", "games-system", "games-worlds"):
            self.assertIn(f'"{namespace}"', text)

    def test_creates_application_namespaces(self) -> None:
        text = infra_text()
        self.assertIn("kubernetes_namespace", text)
        self.assertIn("games-system", text)
        self.assertIn("games-worlds", text)

    def test_fargate_pods_use_private_subnets(self) -> None:
        eks = (INFRA / "eks.tf").read_text()
        profile = eks.split('resource "aws_eks_fargate_profile"')[1]
        self.assertIn("aws_subnet.private", profile)
        self.assertNotIn("aws_subnet.public", profile)

    def test_single_nat_gateway(self) -> None:
        text = infra_text()
        self.assertEqual(text.count("resource \"aws_nat_gateway\""), 1)

    def test_rejects_ec2_node_groups(self) -> None:
        text = infra_text()
        self.assertNotIn("aws_eks_node_group", text)
        self.assertNotIn("eks_managed_node_group", text)
        self.assertNotIn("self_managed_node_group", text)
        self.assertNotIn("aws_instance", text)

    def test_rejects_hostport_and_daemonsets(self) -> None:
        text = infra_text()
        self.assertNotIn("HostPort", text)
        self.assertNotIn("hostPort", text)
        self.assertNotIn("kind: DaemonSet", text)
        self.assertNotIn('resource "kubernetes_daemon_set', text)

    def test_no_game_workloads(self) -> None:
        text = infra_text()
        self.assertNotIn("kind: StatefulSet", text)
        self.assertNotIn("kubernetes_stateful_set", text)
        self.assertNotIn("aws_eks_addon.kube_proxy", text)

    def test_terraform_fmt(self) -> None:
        terraform = shutil.which("terraform") or shutil.which("tofu")
        if terraform is None:
            self.skipTest("terraform/tofu is not installed")
        result = subprocess.run(
            [terraform, "fmt", "-check", "-recursive"],
            cwd=INFRA,
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_terraform_validate(self) -> None:
        terraform = shutil.which("terraform") or shutil.which("tofu")
        if terraform is None or not (INFRA / ".terraform").is_dir():
            self.skipTest("terraform init has not been run")
        result = subprocess.run(
            [terraform, "validate", "-no-color"],
            cwd=INFRA,
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


class EfsWorldSaves(unittest.TestCase):
    def test_access_point_convention(self) -> None:
        efs = (INFRA / "efs.tf").read_text()
        self.assertIn("aws_efs_file_system", efs)
        self.assertIn("aws_efs_access_point", efs)
        self.assertIn("/worlds/${each.value}", efs)
        self.assertIn("owner_uid   = 1000", efs)
        self.assertIn("owner_gid   = 1000", efs)
        self.assertIn("var.world_ids", efs)

    def test_mount_targets_are_private(self) -> None:
        efs = (INFRA / "efs.tf").read_text()
        self.assertIn("aws_efs_mount_target", efs)
        self.assertIn("aws_subnet.private", efs)
        self.assertNotIn("aws_subnet.public", efs)

    def test_static_pv_example(self) -> None:
        example = (INFRA / "k8s" / "examples" / "world-save.yaml").read_text()
        self.assertIn("namespace: games-worlds", example)
        self.assertIn("efs.csi.aws.com", example)
        self.assertIn("FILE_SYSTEM_ID::ACCESS_POINT_ID", example)
        self.assertIn("storageClassName: efs-static", example)
        self.assertNotIn("kind: StatefulSet", example)
        self.assertNotIn("kind: Deployment", example)


class PanelAlb(unittest.TestCase):
    def test_acm_covers_panel_hostname(self) -> None:
        acm = (INFRA / "acm.tf").read_text()
        variables = (INFRA / "variables.tf").read_text()
        self.assertIn("aws_acm_certificate", acm)
        self.assertIn("var.panel_hostname", acm)
        self.assertIn("*.${var.panel_hostname}", acm)
        self.assertIn("games.bradfordly.com", variables)

    def test_ingress_is_internet_facing_ip_with_healthz(self) -> None:
        alb = (INFRA / "panel_alb.tf").read_text()
        self.assertIn("internet-facing", alb)
        self.assertIn("alb.ingress.kubernetes.io/target-type", alb)
        self.assertIn('"ip"', alb)
        self.assertIn("/healthz", alb)
        self.assertIn("aws_acm_certificate.panel.arn", alb)
        self.assertIn("games-system", alb)
        self.assertNotIn("HostPort", alb)
        self.assertNotIn("hostPort", alb)

    def test_load_balancer_controller_is_a_fargate_deployment(self) -> None:
        lbc = (INFRA / "lbc.tf").read_text()
        self.assertIn("kubernetes_deployment", lbc)
        self.assertIn("aws-load-balancer-controller", lbc)
        self.assertIn("kube-system", lbc)
        self.assertNotIn("DaemonSet", lbc)
        self.assertNotIn("hostNetwork", lbc)


class GatewayNlb(unittest.TestCase):
    def test_nlb_is_internet_facing_ip_on_25565(self) -> None:
        nlb = (INFRA / "gateway_nlb.tf").read_text()
        self.assertIn("internet-facing", nlb)
        self.assertIn("aws-load-balancer-nlb-target-type", nlb)
        self.assertIn("ip", nlb)
        self.assertIn("var.gateway_minecraft_port", nlb)
        self.assertIn("LoadBalancer", nlb)
        self.assertIn("games-system", nlb)

    def test_health_checks_use_admin_healthz_not_minecraft(self) -> None:
        nlb = (INFRA / "gateway_nlb.tf").read_text()
        variables = (INFRA / "variables.tf").read_text()
        self.assertIn("/healthz", nlb)
        self.assertIn("var.gateway_admin_port", nlb)
        self.assertIn("HTTP", nlb)
        self.assertNotIn('healthcheck-port"                 = "25565"', nlb)
        self.assertNotIn("healthcheck-port\"                 = 25565", nlb)
        self.assertIn("gateway_admin_port != var.gateway_minecraft_port", variables)
        self.assertIn("25565", variables)
        self.assertIn("8080", variables)


class PanelGatewayRbac(unittest.TestCase):
    def test_roles_are_namespaced_in_games_worlds(self) -> None:
        rbac = (INFRA / "rbac.tf").read_text()
        self.assertIn('resource "kubernetes_service_account" "panel"', rbac)
        self.assertIn('resource "kubernetes_service_account" "gateway"', rbac)
        self.assertIn('resource "kubernetes_role" "panel"', rbac)
        self.assertIn('resource "kubernetes_role" "gateway"', rbac)
        self.assertIn('games-worlds', rbac)
        self.assertIn("games-system", rbac)
        self.assertNotIn('name      = "cluster-admin"', rbac)
        self.assertNotIn("kubernetes_cluster_role", rbac)
        self.assertNotIn("ClusterRole", rbac)

    def test_panel_manages_world_objects(self) -> None:
        panel = (INFRA / "rbac.tf").read_text().split('resource "kubernetes_role" "gateway"')[0]
        self.assertIn("games.bradfordly.com", panel)
        self.assertIn("worlds", panel)
        self.assertIn("services", panel)
        self.assertIn("statefulsets", panel)
        self.assertIn("persistentvolumeclaims", panel)
        self.assertIn("create", panel)
        self.assertIn("delete", panel)

    def test_gateway_can_watch_and_patch_replicas_only(self) -> None:
        gateway = (INFRA / "rbac.tf").read_text().split('resource "kubernetes_role" "gateway"')[1]
        self.assertIn("get", gateway)
        self.assertIn("list", gateway)
        self.assertIn("watch", gateway)
        self.assertIn("patch", gateway)
        self.assertIn("statefulsets/scale", gateway)
        self.assertNotIn("create", gateway)
        self.assertNotIn("delete", gateway)


if __name__ == "__main__":
    unittest.main()




