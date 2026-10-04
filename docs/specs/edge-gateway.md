# Edge gateway

## Purpose

The edge gateway is the always-on middleware between player clients and game worlds. It is the only public listener for game traffic. It detects a real login, starts a stopped world, proxies while players are present, and stops the container after the configured idle timeout.

This is the primary implementation spec. Related documents:

- [ADR-0003](../ADRs/ADR-0003-edge-gateway-first.md)
- [game-adapters.md](game-adapters.md)
- [control-plane.md](control-plane.md)
- [operations.md](operations.md)

## Process shape

- One process (or container) on the public EC2 host, always running.
- Listens on host ports: TCP 25565 for Minecraft Java, plus one UDP port per allocated UDP world (later).
- Exposes a localhost HTTP admin port for health checks and for the control plane to query state.
- Uses the local Docker API to `docker start` / `docker stop` the world's container.
- Loads world config from the control plane API or the on-disk world store. It does not own idle-timeout as a hidden constant.

Do not put this process inside the game container. On G it can share the host with the panel.

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
- `GracefulStop` uses the game's documented signal or API, then the gateway waits up to `stop_timeout` before a hard `docker kill`.

v1 implements the Minecraft Java adapter. Valheim and Palworld are specified in [game-adapters.md](game-adapters.md) and return `not implemented` if selected.

## World state machine

The gateway keeps this state per world. The panel reads it.

| State | Meaning |
| --- | --- |
| `asleep` | Container stopped. Listener still answers status. |
| `starting` | Container starting, backend not ready. |
| `online` | Backend accepted a proxied connection or passed the adapter ready check. |
| `idle_wait` | Online, zero players, timer running. |
| `stopping` | Graceful stop in progress. |
| `failed` | Start or stop timed out. Manual intervention or next login may retry. |

Transitions:

1. `asleep` + status → stay `asleep`, serve asleep MOTD/status.
2. `asleep` + confirmed login → `docker start`, enter `starting`.
3. `starting` + backend ready → `online`, apply Occupy (hold/kick/retry), then proxy.
4. `starting` + `start_timeout` → `failed`, default `docker stop` so a wedged boot does not pin RAM (the AWS bill does not change).
5. `online` + zero players → `idle_wait`, start timer at `idle_timeout`.
6. `idle_wait` + a player → cancel timer, `online`.
7. `idle_wait` + timer fires → `stopping`, `GracefulStop`, wait, container stopped, `asleep`.
8. Control-plane manual stop → same as (7) even if players are present, after a panel warning.
9. Control-plane manual start → same as (2) without a client to occupy.

A power lock prevents concurrent start/stop on one world, the same idea as Wings' exclusive power action.

## Minecraft Java behavior (v1)

Reuse [itzg/mc-router](https://github.com/itzg/mc-router) in **Docker mode** rather than inventing a handshake parser.

- Route on the hostname in the handshake.
- Status (`next state = 1`): answer immediately. If the backend is down, serve `asleep_motd`. If starting, serve `starting_motd`. If online, proxy to the backend or synthesize a live MOTD.
- Login (`next state = 2`): `ShouldWake = true`. `docker start` the backend container when needed.
- Occupy default: **kick with a starting message** on the first login if the backend is not ready within a short hold window; a second login proxies. Optional **hold** if the world is expected to boot inside the client timeout (vanilla, not heavy modpacks).
- Activity: count proxied play connections. A status ping is not activity.
- Scale down: after `idle_timeout` with zero play connections, SIGTERM the backend (`docker stop`). The image must trap SIGTERM so it can flush to the bind mount.

Do not wake on a status ping. Server-list refresh must not start containers.

## UDP behavior (follow-on)

Until a UDP adapter is implemented:

- Bind allocated UDP ports on the host, but do not forward to a missing backend.
- Do not scale up because a datagram arrived.

When an adapter exists:

- Parse enough of the first packets to classify query versus join. Steam A2S (`TSource Engine Query` and related) is never a wake.
- If classification is uncertain, drop. Do not wake.
- Occupy is **retry only**. Do not buffer UDP for minutes.
- Activity comes from the adapter (A2S, REST players), not from datagram counts alone. Datagram counts are a last resort for private Valheim and must use a high threshold plus a longer idle timeout. See [game-adapters.md](game-adapters.md).

## Scaling

The gateway is the only writer of a world's container running state during normal play.

```
scale_up(world):
  acquire power lock
  if container stopped: docker start
  wait until ready or start_timeout
  release lock

scale_down(world):
  acquire power lock
  GracefulStop
  wait until process exited or stop_timeout
  docker stop
  release lock
```

Ready means: the container accepts the game port on the Docker network (or localhost published port) and the adapter's ready check passes (Minecraft handshake or TCP connect; Palworld REST; Valheim A2S).

Docker events are allowed so the gateway notices containers disappearing. The gateway must not fight the control plane: if the control plane deletes a world, the gateway drops the allocation.

On G, stopping a container frees RAM. It does not change the AWS bill.

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
| `allocation` | Public hostname and/or port, backend container, protocol. |

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

Caddy or systemd may hit `/healthz` on localhost. They must not target a game port.

## Failure handling

| Failure | Gateway action |
| --- | --- |
| Docker start/stop fails | Stay in current state, return kick/retry to the client, surface `failed` to the panel. |
| Backend ready too slow | `failed`, default `docker stop`, MOTD explains the failure. |
| Backend crash while online | Optional one restart (`wake_on_crash` default true). Then `failed` if it crashes again within a window. |
| Gateway process restart | Rebuild state from Docker (running containers) and control-plane config. Assume `idle_wait` if the backend is up and activity is unknown; do not immediately stop. |
| Two gateway processes | v1 runs one. A second process would steal ports. |

## Cost comparison

The gateway shares the G host. It does not add an AWS line. Buying an NLB so the gateway could live off-box would add ~$20/month and was rejected in [ADR-0006](../ADRs/ADR-0006-pack-on-public-ec2.md). Idle-stop does not reduce the bill.

## Out of scope

- TLS termination for game protocols.
- Deep packet inspection beyond adapter classification.
- Replacing the control plane UI.
- Exposing the Docker socket on the Elastic IP.
- A second gateway replica.
