from __future__ import annotations

import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from host.bootstrap import (
    CADDY_IMAGE,
    COMPOSE_PLUGIN_URL,
    DATA_LABEL,
    DATA_MOUNT,
    FORBIDDEN_HOST_UNITS,
    GATEWAY_PLAYER_PORT,
    HOST_INSTALL_DIR,
    HTTPS_PORT,
    PANEL_HOSTNAME,
    PANEL_LISTEN_PORT,
    apply_layout,
    bootstrap,
    default_artifact_dir,
    ensure_data_volume,
    ensure_docker,
    layout_paths,
    main,
    parse_lsblk,
    published_https_ports,
    render_bootstrap_script,
    render_caddyfile,
    render_compose,
    render_user_data,
    select_data_device,
    start_caddy,
    systemd_host_units,
    write_artifacts,
    write_runtime_files,
)


class FakeProcess(subprocess.CompletedProcess[str]):
    def __init__(self, args: list[str], returncode: int = 0, stdout: str = "", stderr: str = "") -> None:
        super().__init__(args=args, returncode=returncode, stdout=stdout, stderr=stderr)


class Layout(unittest.TestCase):
    def test_layout_includes_panel_state_and_world_saves(self) -> None:
        paths = layout_paths()
        self.assertEqual(paths["mount"], DATA_MOUNT)
        self.assertEqual(paths["panel"], DATA_MOUNT / "panel")
        self.assertEqual(paths["worlds"], DATA_MOUNT / "worlds")
        self.assertEqual(paths["caddy_data"], DATA_MOUNT / "caddy" / "data")
        self.assertEqual(paths["caddy_config"], DATA_MOUNT / "caddy" / "config")

    def test_apply_layout_is_idempotent(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "data"
            first = apply_layout(root)
            second = apply_layout(root)
            self.assertEqual(first, second)
            for path in first:
                self.assertTrue(path.is_dir())


class DataDevice(unittest.TestCase):
    def test_selects_unused_nvme_data_disk(self) -> None:
        device = select_data_device(
            [
                {"name": "nvme0n1", "type": "disk", "children": [{"name": "nvme0n1p1", "fstype": "xfs", "mountpoint": "/"}]},
                {"name": "nvme1n1", "type": "disk", "fstype": "", "mountpoint": ""},
            ],
            root_source="/dev/nvme0n1p1",
            root_pkname="nvme0n1",
        )
        self.assertEqual(device, "/dev/nvme1n1")

    def test_skips_formatted_or_mounted_disks(self) -> None:
        device = select_data_device(
            [
                {"name": "nvme0n1", "type": "disk"},
                {"name": "nvme1n1", "type": "disk", "fstype": "xfs"},
                {"name": "nvme2n1", "type": "disk", "children": [{"mountpoint": "/mnt"}]},
                {"name": "loop0", "type": "loop"},
            ],
            root_source="/dev/nvme0n1p1",
            root_pkname="nvme0n1",
        )
        self.assertIsNone(device)

    def test_parse_lsblk_reads_blockdevices(self) -> None:
        payload = {
            "blockdevices": [
                {"name": "xvda", "type": "disk", "fstype": "xfs", "mountpoint": "/"},
                {"name": "xvdb", "type": "disk"},
            ]
        }
        self.assertEqual(parse_lsblk(payload, root_source="/dev/xvda"), "/dev/xvdb")


class CaddyConfig(unittest.TestCase):
    def test_uses_lets_encrypt_on_panel_hostname_without_port_80(self) -> None:
        text = render_caddyfile()
        self.assertIn(PANEL_HOSTNAME, text)
        self.assertIn("auto_https disable_redirects", text)
        self.assertIn(f"reverse_proxy panel:{PANEL_LISTEN_PORT}", text)
        self.assertNotRegex(text, r"(?<!\d):80(?!\d)")
        self.assertNotIn("http://", text)

    def test_https_is_published_on_443_only(self) -> None:
        self.assertEqual(published_https_ports(), (HTTPS_PORT,))


class ComposeStack(unittest.TestCase):
    def test_caddy_panel_and_gateway_are_containers(self) -> None:
        text = render_compose()
        self.assertIn(f"image: {CADDY_IMAGE}", text)
        self.assertIn("image: bradfordly-games-panel:local", text)
        self.assertIn("image: bradfordly-games-gateway:local", text)
        self.assertIn(f'"{HTTPS_PORT}:{HTTPS_PORT}"', text)
        self.assertIn(f'"{GATEWAY_PLAYER_PORT}:{GATEWAY_PLAYER_PORT}"', text)
        self.assertIn(f"{DATA_MOUNT / 'panel'}:/var/lib/bradfordly-games/panel", text)
        self.assertIn("/var/run/docker.sock:/var/run/docker.sock", text)
        self.assertNotIn("80:80", text)
        self.assertNotIn("network_mode: host", text)

    def test_compose_accepts_an_alternate_data_root(self) -> None:
        text = render_compose(data_root=Path("/tmp/data"))
        self.assertIn("/tmp/data/panel", text)
        self.assertIn("/tmp/data/caddy/data", text)

    def test_no_host_systemd_units_for_panel_or_gateway(self) -> None:
        units = systemd_host_units()
        self.assertEqual(units, ("docker.service",))
        for unit in FORBIDDEN_HOST_UNITS:
            self.assertNotIn(unit, units)
            self.assertNotIn(unit, render_user_data())
            self.assertNotIn(unit, render_bootstrap_script())


class UserData(unittest.TestCase):
    def test_cloud_init_installs_docker_and_starts_caddy_only(self) -> None:
        text = render_user_data()
        self.assertTrue(text.startswith("#cloud-config"))
        self.assertIn("Do not apply the EKS/Fargate stack", text)
        self.assertIn("- docker", text)
        self.assertIn(str(HOST_INSTALL_DIR / "bootstrap.sh"), text)
        self.assertIn("docker compose", render_bootstrap_script())
        self.assertIn("up -d caddy", render_bootstrap_script())
        self.assertNotIn("up -d panel", render_bootstrap_script())
        self.assertNotIn("systemctl enable --now panel", render_bootstrap_script())
        self.assertNotIn("systemctl enable --now gateway", render_bootstrap_script())
        self.assertIn(DATA_LABEL, render_bootstrap_script())
        self.assertIn(str(DATA_MOUNT), render_bootstrap_script())
        self.assertIn(COMPOSE_PLUGIN_URL, render_bootstrap_script())

    def test_artifacts_match_the_renderer(self) -> None:
        dest = default_artifact_dir()
        self.assertEqual((dest / "user-data.yaml").read_text(), render_user_data())
        self.assertEqual((dest / "Caddyfile").read_text(), render_caddyfile())
        self.assertEqual((dest / "compose.yml").read_text(), render_compose())
        self.assertEqual((dest / "bootstrap.sh").read_text(), render_bootstrap_script())

    def test_write_artifacts_and_runtime_files(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dest = Path(tmp) / "host"
            written = write_artifacts(dest)
            self.assertEqual(written["user-data"].read_text(), render_user_data())
            self.assertTrue(written["bootstrap"].stat().st_mode & 0o111)
            runtime = write_runtime_files(Path(tmp) / "opt")
            self.assertEqual(runtime["caddyfile"].read_text(), render_caddyfile())
            self.assertEqual(runtime["compose"].read_text(), render_compose())


class Commands(unittest.TestCase):
    def _runner(self, responses: dict[tuple[str, ...], FakeProcess]):
        calls: list[list[str]] = []

        def runner(args: list[str], **_kwargs: object) -> FakeProcess:
            calls.append(args)
            key = tuple(args)
            if key in responses:
                return responses[key]
            for stored, result in responses.items():
                if args[: len(stored)] == list(stored):
                    return result
            return FakeProcess(args)

        return runner, calls

    def test_ensure_data_volume_skips_format_when_mounted(self) -> None:
        runner, calls = self._runner({("findmnt", "-n", "/mnt/games-data"): FakeProcess(["findmnt"], 0)})
        with tempfile.TemporaryDirectory() as tmp:
            mount = Path(tmp) / "data"
            ensure_data_volume(mount=mount, runner=runner, fstab=Path(tmp) / "fstab")
        self.assertEqual(calls, [["findmnt", "-n", str(mount)]])

    def test_ensure_data_volume_formats_and_records_fstab(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            mount = Path(tmp) / "data"
            fstab = Path(tmp) / "fstab"
            fstab.write_text("# existing\n")
            mounted = {"done": False}

            def runner(args: list[str], **_kwargs: object) -> FakeProcess:
                if args[:2] == ["findmnt", "-n"] and args[-1] == str(mount):
                    return FakeProcess(args, 0 if mounted["done"] else 1)
                if args[:3] == ["blkid", "-L", DATA_LABEL]:
                    return FakeProcess(args, 1)
                if args[0] == "lsblk" and args[1] == "-J":
                    return FakeProcess(
                        args,
                        0,
                        json.dumps(
                            {
                                "blockdevices": [
                                    {"name": "nvme0n1", "type": "disk", "children": [{"fstype": "xfs", "mountpoint": "/"}]},
                                    {"name": "nvme1n1", "type": "disk"},
                                ]
                            }
                        ),
                    )
                if args[:3] == ["findmnt", "-n", "-o"]:
                    return FakeProcess(args, 0, "/dev/nvme0n1p1\n")
                if args[:2] == ["lsblk", "-no"]:
                    return FakeProcess(args, 0, "nvme0n1\n")
                if args[:2] == ["mkfs.xfs", "-L"]:
                    return FakeProcess(args, 0)
                if args[0] == "mount":
                    mounted["done"] = True
                    return FakeProcess(args, 0)
                return FakeProcess(args, 1, stderr="unexpected")

            ensure_data_volume(mount=mount, runner=runner, fstab=fstab)
            self.assertIn(f"LABEL={DATA_LABEL} {mount} xfs defaults,nofail 0 2", fstab.read_text())

    def test_ensure_data_volume_errors_without_a_disk(self) -> None:
        def runner(args: list[str], **_kwargs: object) -> FakeProcess:
            if args[:2] == ["findmnt", "-n"] and args[-1] != "/":
                return FakeProcess(args, 1)
            if args[:3] == ["blkid", "-L", DATA_LABEL]:
                return FakeProcess(args, 1)
            if args[0] == "lsblk" and args[1] == "-J":
                return FakeProcess(args, 0, json.dumps({"blockdevices": [{"name": "nvme0n1", "type": "disk"}]}))
            if args[:3] == ["findmnt", "-n", "-o"]:
                return FakeProcess(args, 0, "/dev/nvme0n1p1\n")
            if args[:2] == ["lsblk", "-no"]:
                return FakeProcess(args, 0, "nvme0n1\n")
            return FakeProcess(args, 0)

        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(RuntimeError):
                ensure_data_volume(mount=Path(tmp), runner=runner, fstab=Path(tmp) / "fstab")

    def test_ensure_data_volume_retries_mount_a(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            mount = Path(tmp) / "data"
            fstab = Path(tmp) / "missing-fstab"
            attempts = {"mount": 0}

            def runner(args: list[str], **_kwargs: object) -> FakeProcess:
                if args[:2] == ["findmnt", "-n"] and args[-1] == str(mount):
                    return FakeProcess(args, 0 if attempts["mount"] >= 2 else 1)
                if args[:3] == ["blkid", "-L", DATA_LABEL]:
                    return FakeProcess(args, 0, "/dev/nvme1n1\n")
                if args == ["mount", str(mount)]:
                    attempts["mount"] += 1
                    return FakeProcess(args, 1)
                if args == ["mount", "-a"]:
                    attempts["mount"] += 1
                    return FakeProcess(args, 0)
                return FakeProcess(args, 0)

            ensure_data_volume(mount=mount, runner=runner, fstab=fstab)
            self.assertTrue(fstab.exists())
            self.assertGreaterEqual(attempts["mount"], 2)

    def test_ensure_docker_falls_back_to_the_compose_binary(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            plugin_dir = Path(tmp)
            plugin = plugin_dir / "docker-compose"

            def runner(args: list[str], **_kwargs: object) -> FakeProcess:
                if args[:3] == ["dnf", "install", "-y"] and "docker-compose-plugin" in args:
                    return FakeProcess(args, 1)
                if args[0] == "curl":
                    plugin.write_text("compose")
                    return FakeProcess(args, 0)
                return FakeProcess(args, 0)

            ensure_docker(runner=runner, plugin_dir=plugin_dir)
            self.assertTrue(plugin.exists())
            self.assertTrue(plugin.stat().st_mode & 0o111)

    def test_ensure_docker_uses_the_dnf_plugin_when_present(self) -> None:
        calls: list[list[str]] = []

        def runner(args: list[str], **_kwargs: object) -> FakeProcess:
            calls.append(args)
            return FakeProcess(args, 0)

        ensure_docker(runner=runner)
        self.assertIn(["dnf", "install", "-y", "docker", "xfsprogs"], calls)
        self.assertIn(["systemctl", "enable", "--now", "docker"], calls)
        self.assertFalse(any(call[0] == "curl" for call in calls))

    def test_start_caddy_does_not_start_panel_or_gateway(self) -> None:
        calls: list[list[str]] = []

        def runner(args: list[str], **_kwargs: object) -> FakeProcess:
            calls.append(args)
            return FakeProcess(args, 0)

        start_caddy(runner=runner)
        self.assertEqual(
            calls,
            [["docker", "compose", "-f", str(HOST_INSTALL_DIR / "compose.yml"), "up", "-d", "caddy"]],
        )

    def test_bootstrap_wires_layout_docker_and_caddy(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            mount = Path(tmp) / "data"
            install = Path(tmp) / "opt"
            fstab = Path(tmp) / "fstab"

            def runner(args: list[str], **_kwargs: object) -> FakeProcess:
                if args[:2] == ["findmnt", "-n"] and args[-1] == str(mount):
                    return FakeProcess(args, 0)
                return FakeProcess(args, 0)

            paths = bootstrap(runner=runner, mount=mount, install_dir=install, fstab=fstab)
            self.assertTrue(paths["panel"].is_dir())
            self.assertTrue(paths["worlds"].is_dir())
            self.assertEqual(paths["caddyfile"].read_text(), render_caddyfile())

    def test_main_render_and_apply_layout(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dest = Path(tmp) / "out"
            self.assertEqual(main(["render", "--dest", str(dest)]), 0)
            self.assertTrue((dest / "user-data.yaml").exists())
            root = Path(tmp) / "layout"
            self.assertEqual(main(["apply-layout", "--root", str(root)]), 0)
            self.assertTrue((root / "panel").is_dir())

    def test_main_prints_user_data(self) -> None:
        stdout = io.StringIO()
        with patch("sys.stdout", stdout):
            self.assertEqual(main(["user-data"]), 0)
        self.assertIn("#cloud-config", stdout.getvalue())

    def test_main_bootstrap_invokes_the_host_entry_point(self) -> None:
        with patch("host.bootstrap.bootstrap") as mocked:
            self.assertEqual(main(["bootstrap"]), 0)
            mocked.assert_called_once_with()


if __name__ == "__main__":
    unittest.main()
