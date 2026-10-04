# ADR-0001: Prior art and viability

## Status

Accepted

## Date

2026-10-03

## Context

Bradfordly Games needs a control plane at `games.bradfordly.com` that orchestrates dedicated game servers (Minecraft, Valheim, Palworld, and later titles) as containers on a serverless runtime. Servers must stop after a configurable idle period and start again when a player tries to join.

This ADR records the research that shaped the rest of the design. It does not choose a runtime or identity provider; those decisions are [ADR-0002](ADR-0002-reject-wings-use-eks-fargate.md) through [ADR-0005](ADR-0005-identity-for-games-bradfordly.md).

## Research

### Pterodactyl and Pelican

[Pterodactyl](https://pterodactyl.io) and [Pelican](https://github.com/pelican-dev/panel) are the standard open-source game-server panels.

Both split the product in two:

- **Panel:** Laravel (Pelican also uses Filament) plus a React or Filament UI. Users, nodes, allocations, server templates ("Eggs"), file managers, databases, and public APIs live here.
- **Wings:** a Go daemon on each game-server host. It talks to the Docker Engine, streams consoles over WebSockets, serves SFTP, and reports resource use. The Panel calls Wings over HTTP with a per-node bearer token. Wings calls the Panel back on `/api/remote/*`.

Useful ideas:

- Eggs / templates for "this is how you run Minecraft" versus "this is how you run Valheim"
- Allocations as a stable public endpoint for a world
- Power actions with an exclusive lock (`start`, `stop`, `restart`, `kill`)
- A file manager and console that operators expect

What they do not do:

- Scale a world to zero
- Sit on the player path and wake a stopped server
- Run without a Docker socket, host ports, or local disk

Wings is a node agent, not a serverless control plane. It cannot run on EKS Fargate. See [ADR-0002](ADR-0002-reject-wings-use-eks-fargate.md).

### Minecraft wake/sleep proxies

[lazymc](https://github.com/timvisee/lazymc) is a protocol-aware Minecraft Java proxy. It answers status pings while the backend is asleep, starts the server on a login handshake, can hold the client until the backend is ready, and stops the process after idle time. [lazymc-docker-proxy](https://github.com/joesturge/lazymc-docker-proxy) applies the same idea to Docker Compose.

[itzg/mc-router](https://github.com/itzg/mc-router) is the closest Kubernetes-native version. It routes on the hostname in the Java handshake, serves an asleep MOTD, and can patch a StatefulSet from `replicas: 0` to `1` and back after `--auto-scale-down-after`. Many worlds can share TCP 25565.

Minecraft Java is the only popular title researched here where a proxy can both distinguish a server-list ping from a login and hold the client across a multi-minute boot.

### Valheim idle patterns

Valheim listens on UDP (default 2456) with a Steam A2S query port at game port + 1 (2457). [lloesche/valheim-server-docker](https://github.com/lloesche/valheim-server-docker) treats a public server as idle when A2S reports zero players. Private servers do not answer A2S, so that image falls back to counting UDP datagrams in a short window (`IDLE_DATAGRAM_WINDOW`, `IDLE_DATAGRAM_MAX_COUNT`). That heuristic is noisy: query pings and scanners look like traffic.

Steam clients typically time out in 10–30 seconds. A Fargate pod plus world load is usually 1–5 minutes. A UDP proxy cannot hold a Valheim client the way lazymc holds Minecraft. First join starts the server; the player retries.

### Palworld idle patterns

Palworld game traffic is UDP 8211. Optional ports include Steam query (27015/udp), RCON (25575/tcp), and a REST API (8212/tcp). With `RESTAPIEnabled=True`, `GET /v1/api/players` returns the connected player list. That is a reliable idle signal once the process is up. The REST API is not designed for the public internet.

Wake-on-connect has the same UDP cold-start problem as Valheim.

### Agones

[Agones](https://agones.dev) allocates GameServers from Fleets for live-service matchmaking. Buffer autoscalers keep warm ready servers. Webhook autoscalers can scale a Fleet to zero, but the intended consumer is a matchmaker that allocates *before* a player connects, not a player hitting a sleeping world.

Agones is the wrong problem. Bradfordly Games runs one persistent world per server, not a pool of anonymous match sessions.

### Other Fargate game-server work

[raykrueger/cdk-game-server](https://github.com/raykrueger/cdk-game-server) runs one ECS Fargate task per game, mounts EFS for saves, and scales to zero from CloudWatch CPU. Players start the server with a Discord slash command. There is no wake-on-connect path.

AWS samples for UDP on EKS use an NLB with IP targets and a TCP sidecar for health checks, because NLB cannot health-check UDP.

### EKS Fargate limits that matter

From the [EKS Fargate considerations](https://docs.aws.amazon.com/eks/latest/userguide/fargate.html):

- No `HostPort` or `HostNetwork`
- No public subnets for pods
- No DaemonSets
- No EBS; EFS works, with static persistent volumes only
- NLB and ALB work with IP targets
- No Docker socket, no privileged node agent

An always-on proxy inside the game pod is impossible: the pod is gone when the world is scaled to zero. The middleware must be a separate always-on service. See [ADR-0003](ADR-0003-edge-gateway-first.md).

## Decision

The proposed system is **viable for a personal or small-community panel**, with these research conclusions treated as facts for later ADRs:

1. Steal panel UX and the egg/template idea from Pterodactyl and Pelican. Do not run Wings.
2. The login/idle middleware is the product. It must stay up when game pods are down.
3. Minecraft Java is the only first-class "hold the client" protocol. UDP titles wake on a confirmed join attempt and expect the client to retry.
4. Do not treat the first raw packet as a login. Scanners and query pings will false-wake UDP worlds.
5. Idle detection is game-specific: proxied play connections and player lists for Minecraft, A2S or conservative datagram thresholds for Valheim, REST player lists for Palworld.
6. One world is one scale `0/1` workload. Agones fleets are out of scope.
7. Reuse existing dedicated-server images rather than writing game containers.

## Consequences

- Later ADRs can assume "custom control plane plus always-on gateway" rather than "install Pelican."
- Implementation specs must describe per-game adapters, not a single L4 proxy.
- Cold start is a product feature. Specs must say what the player sees while a world is booting.
- Cost on the chosen host ([ADR-0006](ADR-0006-pack-on-public-ec2.md)) is the EC2 instance, billed whether worlds sleep or not. Idle is for RAM and UX, not the AWS bill.
