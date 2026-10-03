# Game adapters

## Purpose

Each title talks a different protocol and exposes player counts differently. The edge gateway calls a per-game adapter. This spec defines those adapters. v1 implements Minecraft Java only. Valheim and Palworld are specified so later work does not invent a generic L4 wake.

Interface and lifecycle: [edge-gateway.md](edge-gateway.md).

## Minecraft Java (v1)

| Item | Value |
| --- | --- |
| Image | `itzg/minecraft-server` (or a pinned tag). `EULA=TRUE` required. |
| Protocol | TCP. Default container port 25565. |
| Public allocation | Shared NLB TCP 25565. Route on handshake hostname. |
| Status versus login | Handshake `intent` 1 = status, 2 = login. Only 2 may wake. |
| Status when asleep | JSON MOTD from `asleep_motd`. Do not proxy. |
| Status when starting | `starting_motd`. Do not fail the server list. |
| Occupy | Default `kick` with "Server is starting, join again." Optional `hold` for short boots. |
| Activity | Number of proxied play connections. Status pings do not count. |
| Ready | Backend accepts TCP and completes a status handshake, or mc-router's existing ready path. |
| Graceful stop | SIGTERM to the container. Image should flush the world. `stop_timeout` default 2m. |
| Wake whitelist | If set, login name must match or the gateway does not scale up. |
| Persistence | Mount EFS on the image save path (typically `/data` or a tighter world directory if hitching appears). |
| Cold start UX | Server list stays reachable. First join may kick. Second join plays. Modded images need a larger `start_timeout`. |

Reuse `mc-router` hostname routing, asleep MOTD, and StatefulSet `0/1` scaling. Do not write a new handshake parser unless `mc-router` cannot run on Fargate (it is a userspace TCP proxy; it should).

Minecraft Bedrock is a different protocol (RakNet/UDP) and is not this adapter.

## Valheim (follow-on)

| Item | Value |
| --- | --- |
| Image | Community dedicated-server image (for example `lloesche/valheim-server`). |
| Protocol | UDP game port (default 2456). Query port is game + 1 (2457). |
| Public allocation | Dedicated NLB UDP listeners for game and, if public, query. No hostname routing. |
| Status versus login | A2S / `TSource Engine Query` on the query port is status. It must never wake. |
| Wake | Confirmed join handshake as documented when the adapter is built. If that cannot be distinguished reliably, do not auto-wake; use panel start only. Prefer missing wake over false wake. |
| Occupy | `retry` only. Steam clients time out before Fargate + world load finishes. |
| Activity (public server) | A2S player count on the query port. Zero players starts `idle_wait`. |
| Activity (private server) | A2S is unavailable. Last resort: datagram count in a short window, threshold high enough to ignore query noise (the lloesche defaults are a starting point, not a wake signal). Use a longer `idle_timeout` than Minecraft. |
| Ready | A2S responds, or the image's HTTP `status.json` if enabled. |
| Graceful stop | The image's documented signal (`SIGINT` on many Valheim units). Do not default to `SIGKILL`. |
| Persistence | EFS on the world save directory only, not the whole Steam install if it can stay in the image. |
| Cold start UX | Panel shows `starting`. Players retry. Optional asleep query response if we can speak A2S from the gateway without the game process. |

Do not enable community listing until false-wake is proven closed. A listed sleeping server will be queried constantly.

## Palworld (follow-on)

| Item | Value |
| --- | --- |
| Image | Community dedicated-server image (for example `thijsvanloef/palworld-server-docker`). |
| Protocol | UDP 8211 game. Optional UDP 27015 query, TCP 25575 RCON, TCP 8212 REST. |
| Public allocation | Dedicated NLB UDP listener for 8211 (or the chosen game port). Do not publish REST or RCON on the NLB. |
| Status versus login | Steam query and REST `GET` are not logins. REST stays on the ClusterIP. |
| Wake | Confirmed join on the game port, same conservatism as Valheim. If classification is weak, panel start only. |
| Occupy | `retry` only. |
| Activity | Palworld REST `GET /v1/api/players` (basic auth, `RESTAPIEnabled=True`) from the gateway to the ClusterIP. Zero players starts `idle_wait`. |
| Ready | REST health or players endpoint answers. |
| Graceful stop | Image `stop_grace_period` / SIGTERM so the world saves. |
| Persistence | EFS on the documented save directory. |
| Cold start UX | Same as Valheim: start on a confirmed join if possible, player retries. |

The REST API is not public. The gateway reaches it over the cluster network.

## Shared game profiles

The control plane ships a profile per `game` value:

| Profile | CPU / memory starting point | Notes |
| --- | --- | --- |
| `minecraft-java` | 1 vCPU / 2 GiB vanilla; raise for modpacks | Fargate size is per-world and editable. |
| `valheim` | 2 vCPU / 4 GiB | Idle RSS is already large; scale-to-zero is the cost win. |
| `palworld` | 2 vCPU / 8 GiB | Confirm against the image docs when implementing. |

These numbers are defaults, not guarantees. Operators can raise them on the world.

## Adding a game later

A new adapter is allowed when it can answer:

1. How do we tell a query from a join?
2. What does the player see during a 1–5 minute boot?
3. How do we count players without sniffing every packet?
4. What signal flushes the save?
5. Which path on EFS is the save, not the install?

If (1) has no answer, the game can still run with panel-only start and adapter idle-stop. That is better than wake-on-datagram.

## Out of scope

- Crossplay / Geyser as part of the Minecraft Java adapter.
- Windows containers.
- GPU titles.
- Multiple shards of one world.
