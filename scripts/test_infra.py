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
        self.assertNotIn("kind: Deployment", text)
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


if __name__ == "__main__":
    unittest.main()

