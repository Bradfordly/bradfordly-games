#!/bin/bash
set -euo pipefail

DATA_MOUNT="/mnt/games-data"
LABEL="games-data"
INSTALL_DIR="/opt/bradfordly-games"

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

mkdir -p "$DATA_MOUNT/panel" \
  "$DATA_MOUNT/worlds" \
  "$DATA_MOUNT/caddy/data" \
  "$DATA_MOUNT/caddy/config"

dnf install -y docker xfsprogs
if ! dnf install -y docker-compose-plugin; then
  mkdir -p /usr/local/lib/docker/cli-plugins
  curl -fsSL "https://github.com/docker/compose/releases/download/v2.32.4/docker-compose-linux-x86_64" -o /usr/local/lib/docker/cli-plugins/docker-compose
  chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
fi

systemctl enable --now docker
usermod -aG docker ec2-user || true

# Caddy is the only compose service started here. Panel and gateway images
# are supplied by later issues; they must not become host systemd units.
docker compose -f "$INSTALL_DIR/compose.yml" up -d caddy
