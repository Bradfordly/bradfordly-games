# Control plane

## Purpose

The control plane is the authenticated application at `games.bradfordly.com`. Operators create worlds, edit idle and boot settings, and see gateway state. The process reconciles Kubernetes objects. It does not proxy player traffic.

Decisions: [ADR-0002](../ADRs/ADR-0002-reject-wings-use-eks-fargate.md), [ADR-0005](../ADRs/ADR-0005-identity-for-games-bradfordly.md).

## Audience

Invite-only. Bradfordly and named friends. Not a public host. Every allowlisted user can see and operate every world in v1.

## Identity

- All HTML routes and JSON APIs require a session, except `GET /healthz`.
- Browser: OIDC authorization code flow (GitHub or Google; pick one in the first implementation issue).
- After the provider returns, the server checks the email or subject against an allowlist (environment or ConfigMap). Fail closed.
- API clients that are browsers use the session cookie (`Secure`, `HttpOnly`, `SameSite=Lax`) on `games.bradfordly.com`.
- The gateway and controllers use Kubernetes service accounts, not OIDC user tokens.
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
| `backend.service` | ClusterIP name. |
| `backend.workload` | StatefulSet or Deployment name. Minecraft scaler needs StatefulSet name = `serviceName`. |
| `idle_timeout` | Duration. Minimum 1m. Default 15m. |
| `start_timeout` | Default 10m. |
| `stop_timeout` | Default 2m. |
| `occupy_mode` | See [edge-gateway.md](edge-gateway.md). |
| `asleep_motd` / `starting_motd` | Minecraft only. |
| `wake_whitelist` | Optional Minecraft names. |
| `env` | Extra image environment (EULA, difficulty, server password). Never store panel OIDC secrets here. |
| `volume` | PVC name for the EFS bind. |

State (`asleep`, `starting`, `online`, `idle_wait`, `stopping`, `failed`) is owned by the gateway. The panel reads it; it does not invent a second state machine.

## API (illustrative)

All routes under `/api` require a session.

| Method | Path | Action |
| --- | --- | --- |
| `GET` | `/api/worlds` | List worlds plus gateway state. |
| `POST` | `/api/worlds` | Create world and reconcile Kubernetes. |
| `GET` | `/api/worlds/:id` | Detail. |
| `PATCH` | `/api/worlds/:id` | Update settings. Changing `idle_timeout` is live; the gateway reloads. |
| `DELETE` | `/api/worlds/:id` | Drain, scale to 0, delete workload, keep or delete PVC per request (default keep). |
| `POST` | `/api/worlds/:id/power` | `{ "action": "start" \| "stop" }`. Forwards to the gateway so drain stays correct. |

Do not expose a public "wake" URL that skips login. Players wake through the game client.

## Reconcile

On create, the control plane ensures:

1. EFS access point or directory exists (manual is acceptable for the first worlds; record the id on the world).
2. Static PV and PVC.
3. ClusterIP Service on the game port.
4. Workload with `replicas: 0`, resource requests from a game profile, EFS mount on the save path, no `HostPort`.
5. Minecraft: DNS CNAME for `allocation.host` to the NLB, or a documented external-DNS annotation.
6. UDP (later): an NLB listener port reserved in the allocation table.
7. Gateway config reload (watch or push).

On settings change, patch the workload env and notify the gateway. Do not recreate the PVC.

On delete, ask the gateway to stop, then delete the workload and Service. Keep the PVC unless the operator asked to destroy the world.

The control plane is the only writer of world *spec*. The gateway is the only writer of replica count during play.

## Kubernetes access

- In-cluster config.
- RBAC limited to the game namespace: get/list/watch/patch/create/delete on the world Services, StatefulSets/Deployments, PVCs; get on pods.
- No cluster-admin.
- The gateway ServiceAccount needs patch on the world workloads and get/list/watch on those objects.

## Persistence of panel data

World records need a store the panel can read after restart. v1 options, pick the smallest that works:

1. Kubernetes Custom Resource per world (spec in etcd, no extra database), or
2. A single ConfigMap/Secret plus EFS for anything larger.

Do not add RDS in v1 unless CRs prove insufficient. YAGNI.

Allowlist stays in a ConfigMap or environment, not in the world store.

## UI notes

- Show the allocation string players need (`host:port`) on the list and detail pages.
- Show "asleep — join the game to start" rather than a red error when replicas are 0.
- Manual stop while players are online requires a confirm.
- Do not hide `failed`; show the last error from the gateway.

## Out of scope

- Per-world RBAC.
- Open registration.
- Impersonating the Kubernetes dashboard.
- Editing files inside the PVC from the browser (later).
