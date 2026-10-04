import shutil
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BACKUPS = ROOT / "infra" / "backups"
INFRA = ROOT / "infra"


def backups_text() -> str:
    return "\n".join(path.read_text() for path in sorted(BACKUPS.glob("*.tf")))


class DataVolumeBackups(unittest.TestCase):
    def test_schedules_weekly_ebs_snapshots(self) -> None:
        text = backups_text()
        self.assertIn('resource "aws_dlm_lifecycle_policy"', text)
        self.assertIn("WEEKS", text)
        self.assertIn("EBS_SNAPSHOT_MANAGEMENT", text)

    def test_targets_the_data_volume_tag_from_issue_57(self) -> None:
        text = backups_text()
        self.assertIn("data-volume", text)
        self.assertIn("target_tags", text)
        self.assertIn("Role", text)

    def test_does_not_create_the_option_g_host_or_volume(self) -> None:
        text = backups_text()
        self.assertNotIn("aws_instance", text)
        self.assertNotIn("aws_ebs_volume", text)
        self.assertNotIn("aws_eks_cluster", text)
        self.assertNotIn("aws_eip", text)

    def test_leaves_the_eks_stack_in_place(self) -> None:
        self.assertTrue((INFRA / "eks.tf").is_file())
        self.assertNotIn('resource "aws_eks_cluster"', backups_text())

    def test_on_demand_policy_allows_create_snapshot(self) -> None:
        text = backups_text()
        self.assertIn("ec2:CreateSnapshot", text)
        self.assertIn("GAMES_DATA_VOLUME_ID", text)

    def test_retention_stays_near_the_two_to_three_dollar_line(self) -> None:
        text = backups_text()
        self.assertIn("retention_count", text)
        self.assertIn("default     = 4", (BACKUPS / "variables.tf").read_text())

    def test_terraform_fmt(self) -> None:
        terraform = shutil.which("terraform") or shutil.which("tofu")
        if terraform is None:
            self.skipTest("terraform/tofu is not installed")
        result = subprocess.run(
            [terraform, "fmt", "-check", "-recursive"],
            cwd=BACKUPS,
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_terraform_validate(self) -> None:
        terraform = shutil.which("terraform") or shutil.which("tofu")
        if terraform is None or not (BACKUPS / ".terraform").is_dir():
            self.skipTest("terraform init has not been run")
        result = subprocess.run(
            [terraform, "validate", "-no-color"],
            cwd=BACKUPS,
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
