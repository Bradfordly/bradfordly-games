#!/usr/bin/env python3
"""Install Docker, Caddy, and the data-directory layout on the option G host.

Cloud-init user-data rendered here is what issue #57 attaches to the public
EC2 instance. This module does not replace the EKS stack in infra/.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path
from typing import Any, Callable

PANEL_HOSTNAME = "games.bradfordly.com"
DATA_LABEL = "games-data"
DATA_MOUNT = Path("/mnt/games-data")
HOST_INSTALL_DIR = Path("/opt/bradfordly-games")
CADDY_IMAGE = "caddy:2-alpine"
PANEL_IMAGE = "bradfordly-games-panel:local"
GATEWAY_IMAGE = "bradfordly-games-gateway:local"
PANEL_LISTEN_PORT = 8080
GATEWAY_PLAYER_PORT = 25565
GATEWAY_ADMIN_PORT = 8080
HTTPS_PORT = 443
COMPOSE_VERSION = "2.32.4"
COMPOSE_PLUGIN_URL = (
    "https://github.com/docker/compose/releases/download/"
    f"v{COMPOSE_VERSION}/docker-compose-linux-x86_64"
)

PANEL_STATE_RELATIVE = Path("panel")
WORLDS_RELATIVE = Path("worlds")
CADDY_DATA_RELATIVE = Path("caddy") / "data"
CADDY_CONFIG_RELATIVE = Path("caddy") / "config"

HOST_UNITS = ("docker.service",)
# Panel and gateway are compose services, never host systemd units.
FORBIDDEN_HOST_UNITS = ("panel.service", "gateway.service")

Runner = Callable[..., subprocess.CompletedProcess[str]]


def layout_paths(root: Path | None = None) -> dict[str, Path]:
    """Return the data-volume directory layout (world saves and panel state)."""
    base = Path(root) if root is not None else DATA_MOUNT
    return {
        "mount": base,
        "panel": base / PANEL_STATE_RELATIVE,
        "worlds": base / WORLDS_RELATIVE,
        "caddy_data": base / CADDY_DATA_RELATIVE,
        "caddy_config": base / CADDY_CONFIG_RELATIVE,
    }


def apply_layout(root: Path | None = None) -> list[Path]:
    """Create the data-volume directories. Idempotent."""
    created: list[Path] = []
    for path in layout_paths(root).values():
        path.mkdir(parents=True, exist_ok=True)
        created.append(path)
    return created


def published_https_ports() -> tuple[int, ...]:
    """Caddy publishes HTTPS only. Port 80 stays closed for TLS-ALPN."""
    return (HTTPS_PORT,)


def systemd_host_units() -> tuple[str, ...]:
    return HOST_UNITS


def select_data_device(
    devices: list[dict[str, Any]],
    *,
    root_source: str,
    root_pkname: str = "",
) -> str | None:
    """Choose the unused whole disk that is not the root volume.

    `devices` matches a subset of `lsblk -J` blockdevice objects.
    """
    root_name = Path(root_source).name
    root_disk = root_pkname or root_name.rstrip("0123456789")
    if root_name.startswith("nvme") and "p" in root_name:
        root_disk = root_pkname or root_name.rsplit("p", 1)[0]

    for device in devices:
        if device.get("type") != "disk":
            continue
        name = str(device.get("name") or "")
        if not name or name == root_disk:
            continue
        if device.get("fstype") or device.get("mountpoint"):
            continue
        children = device.get("children") or []
        if any(child.get("fstype") or child.get("mountpoint") for child in children):
            continue
        return f"/dev/{name}"
    return None


def parse_lsblk(payload: dict[str, Any], *, root_source: str, root_pkname: str = "") -> str | None:
    return select_data_device(
        list(payload.get("blockdevices") or []),
        root_source=root_source,
        root_pkname=root_pkname,
    )


def render_caddyfile() -> str:
    """HTTPS for the panel hostname. Let's Encrypt via TLS-ALPN on 443 only."""
    return (
        f"""\
{{
	auto_https disable_redirects
}}

{PANEL_HOSTNAME} {{
	reverse_proxy panel:{PANEL_LISTEN_PORT}
}}
"""
    )


def render_compose(*, data_root: Path | None = None) -> str:
    """Panel and gateway are containers. Caddy publishes 443 only."""
    root = Path(data_root) if data_root is not None else DATA_MOUNT
    paths = layout_paths(root)
    install = HOST_INSTALL_DIR
    return (
        f"""\
# Panel and gateway run on the host Docker daemon, not as systemd units.
services:
  caddy:
    image: {CADDY_IMAGE}
    restart: unless-stopped
    ports:
      - "{HTTPS_PORT}:{HTTPS_PORT}"
    volumes:
      - {install / "Caddyfile"}:/etc/caddy/Caddyfile:ro
      - {paths["caddy_data"]}:/data
      - {paths["caddy_config"]}:/config

  panel:
    image: {PANEL_IMAGE}
    restart: unless-stopped
    volumes:
      - {paths["panel"]}:/var/lib/bradfordly-games/panel
      - /var/run/docker.sock:/var/run/docker.sock
    expose:
      - "{PANEL_LISTEN_PORT}"

  gateway:
    image: {GATEWAY_IMAGE}
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    ports:
      - "{GATEWAY_PLAYER_PORT}:{GATEWAY_PLAYER_PORT}"
    expose:
      - "{GATEWAY_ADMIN_PORT}"
"""
    )


def render_bootstrap_script() -> str:
    """Shell run by cloud-init on Amazon Linux 2023."""
    mount = DATA_MOUNT
    label = DATA_LABEL
    install = HOST_INSTALL_DIR
    return (
        f"""\
#!/bin/bash
set -euo pipefail

DATA_MOUNT="{mount}"
LABEL="{label}"
INSTALL_DIR="{install}"

mkdir -p "$DATA_MOUNT"

if ! findmnt -n "$DATA_MOUNT" >/dev/null 2>&1; then
  if ! blkid -L "$LABEL" >/dev/null 2>&1; then
    root_src="$(findmnt -n -o SOURCE /)"
    root_disk="$(lsblk -no PKNAME "$root_src" | head -1)"
    if [ -z "$root_disk" ]; then
      root_disk="$(lsblk -no NAME "$root_src" | head -1)"
    fi
    while read -r name type; do
      [ "$type" = "disk" ] || continue
      [ "$name" = "$root_disk" ] && continue
      fstype="$(lsblk -dn -o FSTYPE "/dev/$name" || true)"
      if [ -z "$fstype" ]; then
        mkfs.xfs -L "$LABEL" "/dev/$name"
        break
      fi
    done < <(lsblk -dn -o NAME,TYPE)
  fi
  if ! grep -q "LABEL=$LABEL" /etc/fstab; then
    echo "LABEL=$LABEL $DATA_MOUNT xfs defaults,nofail 0 2" >> /etc/fstab
  fi
  mount "$DATA_MOUNT" || mount -a
fi

mkdir -p "$DATA_MOUNT/{PANEL_STATE_RELATIVE}" \\
  "$DATA_MOUNT/{WORLDS_RELATIVE}" \\
  "$DATA_MOUNT/{CADDY_DATA_RELATIVE}" \\
  "$DATA_MOUNT/{CADDY_CONFIG_RELATIVE}"

dnf install -y docker xfsprogs
if ! dnf install -y docker-compose-plugin; then
  mkdir -p /usr/local/lib/docker/cli-plugins
  curl -fsSL "{COMPOSE_PLUGIN_URL}" -o /usr/local/lib/docker/cli-plugins/docker-compose
  chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
fi

systemctl enable --now docker
usermod -aG docker ec2-user || true

# Caddy is the only compose service started here. Panel and gateway images
# are supplied by later issues; they must not become host systemd units.
docker compose -f "$INSTALL_DIR/compose.yml" up -d caddy
"""
    )


def _indent_block(text: str, spaces: int) -> str:
    prefix = " " * spaces
    return "\n".join(prefix + line if line else prefix.rstrip() for line in text.splitlines())


def render_user_data() -> str:
    """Cloud-init user-data for the #57 host. Do not apply the EKS stack."""
    caddyfile = render_caddyfile()
    compose = render_compose()
    script = render_bootstrap_script()
    return (
        f"""\
#cloud-config
# Attach as EC2 user_data on the option G host created by issue #57.
# Do not apply the EKS/Fargate stack in infra/.

package_update: true
packages:
  - docker
  - xfsprogs

write_files:
  - path: {HOST_INSTALL_DIR / "Caddyfile"}
    permissions: "0644"
    content: |
{_indent_block(caddyfile, 6)}
  - path: {HOST_INSTALL_DIR / "compose.yml"}
    permissions: "0644"
    content: |
{_indent_block(compose, 6)}
  - path: {HOST_INSTALL_DIR / "bootstrap.sh"}
    permissions: "0755"
    content: |
{_indent_block(script, 6)}

runcmd:
  - [ {HOST_INSTALL_DIR / "bootstrap.sh"} ]
"""
    )


def write_artifacts(dest: Path) -> dict[str, Path]:
    """Write the files issue #57 can attach or copy onto the instance."""
    dest.mkdir(parents=True, exist_ok=True)
    written = {
        "user-data": dest / "user-data.yaml",
        "caddyfile": dest / "Caddyfile",
        "compose": dest / "compose.yml",
        "bootstrap": dest / "bootstrap.sh",
    }
    written["user-data"].write_text(render_user_data())
    written["caddyfile"].write_text(render_caddyfile())
    written["compose"].write_text(render_compose())
    written["bootstrap"].write_text(render_bootstrap_script())
    written["bootstrap"].chmod(written["bootstrap"].stat().st_mode | 0o111)
    return written


def _run(
    runner: Runner,
    args: list[str],
    *,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    return runner(args, check=check, text=True, capture_output=True)


def ensure_data_volume(
    *,
    mount: Path = DATA_MOUNT,
    runner: Runner = subprocess.run,
    fstab: Path = Path("/etc/fstab"),
) -> Path:
    """Format (once) and mount the extra EBS volume at the data mount."""
    mount.mkdir(parents=True, exist_ok=True)
    if _already_mounted(mount, runner=runner):
        return mount

    labeled = _run(runner, ["blkid", "-L", DATA_LABEL], check=False)
    if labeled.returncode != 0:
        device = _discover_data_device(runner=runner)
        if device is None:
            raise RuntimeError("no unused data volume found")
        _run(runner, ["mkfs.xfs", "-L", DATA_LABEL, device])

    line = f"LABEL={DATA_LABEL} {mount} xfs defaults,nofail 0 2\n"
    if fstab.exists() and line.strip() not in fstab.read_text():
        with fstab.open("a", encoding="utf-8") as handle:
            handle.write(line)
    elif not fstab.exists():
        fstab.write_text(line)
    _run(runner, ["mount", str(mount)], check=False)
    if not _already_mounted(mount, runner=runner):
        _run(runner, ["mount", "-a"])
    return mount


def _already_mounted(mount: Path, *, runner: Runner) -> bool:
    result = _run(runner, ["findmnt", "-n", str(mount)], check=False)
    return result.returncode == 0


def _discover_data_device(*, runner: Runner) -> str | None:
    lsblk = _run(runner, ["lsblk", "-J", "-o", "NAME,TYPE,FSTYPE,MOUNTPOINT,PKNAME"])
    root = _run(runner, ["findmnt", "-n", "-o", "SOURCE", "/"])
    root_source = (root.stdout or "").strip()
    pk = _run(runner, ["lsblk", "-no", "PKNAME", root_source], check=False)
    payload = json.loads(lsblk.stdout or "{}")
    return parse_lsblk(payload, root_source=root_source, root_pkname=(pk.stdout or "").strip())


def ensure_docker(
    *,
    runner: Runner = subprocess.run,
    plugin_dir: Path = Path("/usr/local/lib/docker/cli-plugins"),
) -> None:
    """Install Docker Engine and the compose plugin on Amazon Linux 2023."""
    _run(runner, ["dnf", "install", "-y", "docker", "xfsprogs"])
    plugin = _run(runner, ["dnf", "install", "-y", "docker-compose-plugin"], check=False)
    if plugin.returncode != 0:
        plugin_dir.mkdir(parents=True, exist_ok=True)
        dest = plugin_dir / "docker-compose"
        _run(runner, ["curl", "-fsSL", COMPOSE_PLUGIN_URL, "-o", str(dest)])
        if dest.exists():
            dest.chmod(dest.stat().st_mode | 0o111)
    _run(runner, ["systemctl", "enable", "--now", "docker"])
    _run(runner, ["usermod", "-aG", "docker", "ec2-user"], check=False)


def start_caddy(*, runner: Runner = subprocess.run) -> None:
    """Start Caddy as a container. Do not start panel or gateway here."""
    compose = HOST_INSTALL_DIR / "compose.yml"
    _run(runner, ["docker", "compose", "-f", str(compose), "up", "-d", "caddy"])


def write_runtime_files(install_dir: Path = HOST_INSTALL_DIR) -> dict[str, Path]:
    install_dir.mkdir(parents=True, exist_ok=True)
    files = {
        "caddyfile": install_dir / "Caddyfile",
        "compose": install_dir / "compose.yml",
    }
    files["caddyfile"].write_text(render_caddyfile())
    files["compose"].write_text(render_compose())
    return files


def bootstrap(
    *,
    runner: Runner = subprocess.run,
    mount: Path = DATA_MOUNT,
    install_dir: Path = HOST_INSTALL_DIR,
    fstab: Path = Path("/etc/fstab"),
) -> dict[str, Path]:
    """Full host entry point used when this module is copied onto the instance."""
    ensure_data_volume(mount=mount, runner=runner, fstab=fstab)
    apply_layout(mount)
    paths = dict(layout_paths(mount))
    files = write_runtime_files(install_dir)
    ensure_docker(runner=runner)
    start_caddy(runner=runner)
    paths.update(files)
    return paths


def default_artifact_dir() -> Path:
    return Path(__file__).resolve().parent


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    render = sub.add_parser("render", help="Write user-data, Caddyfile, and compose files")
    render.add_argument("--dest", type=Path, default=default_artifact_dir())

    layout = sub.add_parser("apply-layout", help="Create the data-directory layout")
    layout.add_argument("--root", type=Path, default=DATA_MOUNT)

    sub.add_parser("user-data", help="Print cloud-init user-data")
    sub.add_parser("bootstrap", help="Format/mount data volume, install Docker, start Caddy")

    args = parser.parse_args(argv)
    if args.command == "render":
        write_artifacts(args.dest)
        return 0
    if args.command == "apply-layout":
        apply_layout(args.root)
        return 0
    if args.command == "user-data":
        sys.stdout.write(render_user_data())
        return 0
    if args.command == "bootstrap":
        bootstrap()
        return 0
    return 2


if __name__ == "__main__":
    sys.exit(main())
