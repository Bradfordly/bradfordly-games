# Edge gateway

## Purpose

The edge gateway is the always-on middleware between player clients and game worlds. It is the only public listener for game traffic. It detects a real login, starts a stopped world, proxies while players are present, and scales the world to zero after the configured idle timeout.

This is the primary implementation spec. Related documents:

- [ADR-0003](../ADRs/ADR-0003-edge-gateway-first.md)
- [game-adapters.md](game-adapters.md)
- [control-plane.md](control-plane.md)
- [operations.md](operations.md)

## Process shape

- One Fargate Deployment, always at least one replica.
- Listens on the ports the NLB forwards: TCP 25565 for Minecraft Java, plus one UDP port per allocated UDP world (later).
- Exposes a loopback or cluster-only HTTP admin port for NLB/Kubernetes health checks and for the control plane to query state.
- Uses an in-cluster Kubernetes client to scale the world's workload between 0 and 1.
- Loads world config from the control plane API or from Kubernetes objects the control plane owns. It does not own idle-timeout as a hidden constant.

Do not put this process in the game pod.

## Adapter interface

Each game implements this interface. Names are illustrative; the implementation language is not fixed.

```
Adapter
  Match(allocation, first_bytes_or_connection) -> world | none
  Classify(connection) -> Status | Login | Other
  ShouldWake(event) -> bool
  ServeStatus(connection, world_state) -> void
  Occupy(connection, world_state) -> Hold | Kick | Retry
  Activity(world) -> player_count | unknown
  GracefulStop(world) -> void
```

Rules that apply to every adapter:

- `ShouldWake` is false for status/query and for unclassified noise.
- `ShouldWake` is true only for a confirmed login or the adapter's documented game handshake.
- `Activity` returning `unknown` must not start the idle timer. Prefer a longer timeout over a false shutdown.
- `GracefulStop` uses the game's documented signal or API, then the gateway waits up to `stop_timeout` before a hard scale-down.

v1 implements the Minecraft Java adapter. Valheim and Palworld are specified in [game-adapters.md](game-adapters.md) and return `not implemented` if selected.

## World state machine

The gateway keeps this state per world. The panel reads it.

| State | Meaning |
| --- | --- |
| `asleep` | Workload replicas = 0. Listener still answers status. |
| `starting` | Replicas = 1, backend not ready. |
| `online` | Backend accepted a proxied connection or passed the adapter ready check. |
| `idle_wait` | Online, zero players, timer running. |
| `stopping` | Graceful stop in progress. |
| `failed` | Start or stop timed out. Manual intervention or next login may retry. |

Transitions:

1. `asleep` + status → stay `asleep`, serve asleep MOTD/status.
2. `asleep` + confirmed login → set replicas to 1, enter `starting`.
3. `starting` + backend ready → `online`, apply Occupy (hold/kick/retry), then proxy.
4. `starting` + `start_timeout` → `failed`, leave replicas at 1 or scale back to 0 per world setting (default: scale to 0 to avoid a stuck bill).
5. `online` + zero players → `idle_wait`, start timer at `idle_timeout`.
6. `idle_wait` + a player → cancel timer, `online`.
7. `idle_wait` + timer fires → `stopping`, `GracefulStop`, wait, set replicas to 0, `asleep`.
8. Control-plane manual stop → same as (7) even if players are present, after a panel warning.
9. Control-plane manual start → same as (2) without a client to occupy.

A power lock prevents concurrent start/stop on one world, the same idea as Wings' exclusive power action.

## Minecraft Java behavior (v1)

Reuse [itzg/mc-router](https://github.com/itzg/mc-router) behavior rather than inventing a handshake parser.

- Route on the hostname in the handshake.
- Status (`next state = 1`): answer immediately. If the backend is down, serve `asleep_motd`. If starting, serve `starting_motd`. If online, proxy to the backend or synthesize a live MOTD.
- Login (`next state = 2`): `ShouldWake = true`. Scale the backend StatefulSet from 0 to 1 when needed.
- Occupy default: **kick with a starting message** on the first login if the backend is not ready within a short hold window; a second login proxies. Optional **hold** if the world is expected to boot inside the client timeout (vanilla, not heavy modpacks).
- Activity: count proxied play connections. A status ping is not activity.
- Scale down: after `idle_timeout` with zero play connections, SIGTERM the backend (mc-router's Kubernetes path sets replicas to 0; the pod must trap SIGTERM so the image can flush).

`mc-router` requires the workload `metadata.name` and `spec.serviceName` to match when it scales StatefulSets. The control plane must create worlds that way, or the gateway must expose an equivalent scale API and not call mc-router's scaler.

Do not wake on a status ping. Server-list refresh must not start Fargate tasks.

## UDP behavior (follow-on)

Until a UDP adapter is implemented:

- Bind allocated UDP ports so NLB health is not the only consumer, but do not forward to a missing backend.
- Do not scale up because a datagram arrived.

When an adapter exists:

- Parse enough of the first packets to classify query versus join. Steam A2S (`TSource Engine Query` and related) is never a wake.
- If classification is uncertain, drop. Do not wake.
- Occupy is **retry only**. Do not buffer UDP for minutes.
- Activity comes from the adapter (A2S, REST players), not from datagram counts alone. Datagram counts are a last resort for private Valheim and must use a high threshold plus a longer idle timeout. See [game-adapters.md](game-adapters.md).

## Scaling

The gateway is the only writer of a world's replica count during normal play.

```
scale_up(world):
  acquire power lock
  if replicas == 0: patch replicas = 1
  wait until ready or start_timeout
  release lock

scale_down(world):
  acquire power lock
  GracefulStop
  wait until process exited or stop_timeout
  patch replicas = 0
  release lock
```

Ready means: the world's ClusterIP accepts the game port and the adapter's ready check passes (Minecraft handshake or TCP connect; Palworld REST; Valheim A2S or a TCP sidecar).

Kubernetes watches are allowed so the gateway notices pods disappearing. The gateway must not fight the control plane: if the control plane deletes a world, the gateway drops the allocation.

## Settings the gateway reads

Per world, from the control plane:

| Setting | Role |
| --- | --- |
| `idle_timeout` | Time in `idle_wait` before stop. Minimum 1 minute. Default 15 minutes. |
| `start_timeout` | Give up on boot. Default 10 minutes. |
| `stop_timeout` | Give up on graceful stop. Default 2 minutes. |
| `asleep_motd` / `starting_motd` | Minecraft status text. |
| `occupy_mode` | `hold`, `kick`, `retry`. Minecraft default `kick`. UDP forced `retry`. |
| `wake_whitelist` | Optional Minecraft names that may wake the world. |
| `allocation` | Public hostname and/or port, backend Service, protocol. |

Global:

| Setting | Role |
| --- | --- |
| Admin bind address | Health and control-plane queries. Not public. |
| Default idle timeout | Used when a world omits one. |

## Anti-false-wake

- Never wake on TCP connect alone. Read the Minecraft handshake intent.
- Never wake on UDP length or rate alone in v1 UDP adapters.
- Rate-limit wake attempts per world (for example one scale-up per 30 seconds) so a crashing boot cannot loop.
- Log every wake with cause: `minecraft_login:<name>` or `manual`.
- Banned or non-whitelisted Minecraft logins (when `wake_whitelist` is on) are not wakes.

## Health and observability

- `GET /healthz` on the admin port: process is serving.
- `GET /worlds` or equivalent: in-memory state for the panel.
- Metrics: wakes, false-wake rejects, scale errors, proxy connections, idle stops, start timeouts.
- Structured logs: world id, state transition, player identifier when the protocol has one.

NLB health checks hit `/healthz` over TCP/HTTP. They must not target a game port.

## Failure handling

| Failure | Gateway action |
| --- | --- |
| Kubernetes patch fails | Stay in current state, return kick/retry to the client, surface `failed` to the panel. |
| Backend ready too slow | `failed`, default scale to 0, MOTD explains the failure. |
| Backend crash while online | Optional one restart (`wake_on_crash` default true). Then `failed` if it crashes again within a window. |
| Gateway replica restart | Rebuild state from Kubernetes (replica counts) and control-plane config. Assume `idle_wait` if the backend is up and activity is unknown; do not immediately stop. |
| Split brain (two gateway replicas) | v1 runs a single replica (`Recreate`). Horizontal scale needs a lock in a later revision. |

## Out of scope

- TLS termination for game protocols.
- Deep packet inspection beyond adapter classification.
- Replacing the control plane UI.
- Docker socket access.
