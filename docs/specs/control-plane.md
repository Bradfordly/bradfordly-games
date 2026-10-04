# Control plane

## Purpose

The control plane is the authenticated application at `games.bradfordly.com`. Operators create worlds, edit idle and boot settings, and see gateway state. The process creates and updates Docker containers. It does not proxy player traffic.

Decisions: [ADR-0005](../ADRs/ADR-0005-identity-for-games-bradfordly.md), [ADR-0006](../ADRs/ADR-0006-pack-on-public-ec2.md).

## Audience

Invite-only. Bradfordly and named friends. Not a public host. Every allowlisted user can see and operate every world in v1.

## Identity

- All HTML routes and JSON APIs require a session, except `GET /healthz`.
- Browser: OIDC authorization code flow (GitHub or Google; pick one in the first implementation issue).
- After the provider returns, the server checks the email or subject against an allowlist (environment or ConfigMap). Fail closed.
- API clients that are browsers use the session cookie (`Secure`, `HttpOnly`, `SameSite=Lax`) on `games.bradfordly.com`.
- The gateway talks to the Docker API on the host, not OIDC user tokens.
- Logout clears the session.

There is no self-serve invite. Editing the allowlist is an operations change.

## Surface

v1 pages:

1. **Login** — redirect to the provider.
2. **World list** — name, game, state, allocation, player count if known, idle timer if in `idle_wait`.
3. **World detail** — settings form, last wake cause, recent transitions, manual start/stop.
4. **Denied** — authenticated but not allowlisted.

v1 does not include: file manager, SFTP, live console, user admin UI, billing.

## World model

A world is the unit of scale. Fields:

| Field | Notes |
| --- | --- |
| `id` | Stable identifier. |
| `name` | Display name. |
| `game` | `minecraft-java` (v1). Later: `valheim`, `palworld`. |
| `image` | Dedicated-server image reference. Default per game. |
| `allocation.host` | Public hostname players type (Minecraft). |
| `allocation.port` | Public port (25565 shared for Minecraft; dedicated for UDP). |
| `allocation.protocol` | `tcp` or `udp`. |
| `backend.container` | Docker container name. |
| `idle_timeout` | Duration. Minimum 1m. Default 15m. |
| `start_timeout` | Default 10m. |
| `stop_timeout` | Default 2m. |
| `occupy_mode` | See [edge-gateway.md](edge-gateway.md). |
| `asleep_motd` / `starting_motd` | Minecraft only. |
| `wake_whitelist` | Optional Minecraft names. |
| `env` | Extra image environment (EULA, difficulty, server password). Never store panel OIDC secrets here. |
| `volume` | Host path for the EBS bind mount. |

State (`asleep`, `starting`, `online`, `idle_wait`, `stopping`, `failed`) is owned by the gateway. The panel reads it; it does not invent a second state machine.

## API (illustrative)

All routes under `/api` require a session.

| Method | Path | Action |
| --- | --- | --- |
| `GET` | `/api/worlds` | List worlds plus gateway state. |
| `POST` | `/api/worlds` | Create world and ensure a stopped container. |
| `GET` | `/api/worlds/:id` | Detail. |
| `PATCH` | `/api/worlds/:id` | Update settings. Changing `idle_timeout` is live; the gateway reloads. |
| `DELETE` | `/api/worlds/:id` | Drain, stop, remove container, keep or delete the save directory per request (default keep). |
| `POST` | `/api/worlds/:id/power` | `{ "action": "start" \| "stop" }`. Forwards to the gateway so drain stays correct. |

Do not expose a public "wake" URL that skips login. Players wake through the game client.

## Reconcile

On create, the control plane ensures:

1. A save directory exists on the data (or root) volume.
2. A Docker container from the game profile, stopped, with the save path bind-mounted, memory/CPU from the profile, no published RCON.
3. Minecraft: `allocation.host` is a name that resolves to the Elastic IP (wildcard DNS is enough).
4. UDP (later): a host port reserved in the allocation table and opened on the security group.
5. Gateway config reload (watch or push).

On settings change, update container env and notify the gateway. Do not recreate the save directory.

On delete, ask the gateway to stop, then remove the container. Keep the save directory unless the operator asked to destroy the world.

The control plane is the only writer of world *spec*. The gateway is the only writer of container running state during play.

## Docker access

- Panel and gateway use the local Docker API (`/var/run/docker.sock` or a locked-down TCP socket on localhost).
- Do not expose the Docker socket on the Elastic IP.

## Persistence of panel data

World records need a store the panel can read after restart. v1 options, pick the smallest that works:

1. A SQLite file on the data volume, or
2. A JSON file on the data volume.

Do not add RDS, Aurora, or DynamoDB in v1. YAGNI.

Allowlist stays in SSM or a file, not in the world store.

## Cost comparison

The panel adds no AWS line of its own. It shares the G host (~$38/month asleep). A managed database would add $12–$43 and is rejected in [ADR-0006](../ADRs/ADR-0006-pack-on-public-ec2.md).

## UI notes

- Show the allocation string players need (`host:port`) on the list and detail pages.
- Show "asleep — join the game to start" rather than a red error when the container is stopped.
- Manual stop while players are online requires a confirm.
- Do not hide `failed`; show the last error from the gateway.

## Out of scope

- Per-world RBAC.
- Open registration.
- Impersonating a Docker or machine dashboard.
- Editing files inside the save directory from the browser (later).
