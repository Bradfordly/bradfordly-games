# ADR-0002: Reject Wings; run on EKS Fargate

## Status

Accepted

## Date

2026-10-03

## Context

[ADR-0001](ADR-0001-prior-art-and-viability.md) found that Pterodactyl and Pelican need a per-node Wings daemon with a Docker socket, host ports, and local disk. The product requirement is a serverless control plane that also schedules game servers on a serverless runtime, specifically AWS EKS with Fargate.

Two other orchestrators were considered:

- **Pelican/Pterodactyl on EC2:** fastest path to a familiar panel. It is not serverless, cannot scale a world to zero without extra work, and still has no wake-on-connect path.
- **ECS Fargate:** same Fargate compute without the EKS cluster fee (about $0.10/hour, ~$73/month). Less natural for a custom Kubernetes reconcile loop and for wrapping `mc-router`'s StatefulSet scaler.
- **EKS Fargate:** matches the stated target. Pods cannot use host networking or EBS. The cluster fee is always on.

## Decision

1. **Do not deploy Pterodactyl, Pelican, or Wings.** Build a custom control plane that talks to the Kubernetes API.
2. **Run the control plane, the edge gateway, and each game world on EKS Fargate.**
3. **Treat each world as a `0/1` replica workload** (StatefulSet or Deployment plus Service), not an Agones Fleet.
4. **Reuse published dedicated-server images** (`itzg/minecraft-server`, community Valheim and Palworld images). Do not maintain game server binaries in this repo.
5. **Keep ECS Fargate as a documented cost-cut**, not as the v1 orchestrator. Revisit only if the EKS fee dominates the bill after worlds actually sleep.

## Rationale

Wings cannot run on Fargate: there is no Docker socket, no `HostPort`, no `HostNetwork`, and no public-subnet pod. Porting Wings onto Kubernetes would be a larger project than writing a small reconciler that sets `replicas` and mounts EFS.

EKS is kept because the requirement named it, because `mc-router` already scales Kubernetes StatefulSets, and because a single cluster can host the panel, the gateway, and the worlds with ordinary Deployments and Services.

A world is not a match session. Scaling `replicas` between 0 and 1 is the whole lifecycle. Agones allocation, ready buffers, and FleetAutoscalers add machinery we will not use.

## Consequences

- The control plane is a Kubernetes client, not a Wings client.
- Game pods stay in private subnets. Ingress is an ALB (panel) and an NLB (player traffic) with IP targets. See [ADR-0004](ADR-0004-persistence-and-networking.md).
- Baseline cost includes the EKS control plane plus always-on Fargate for the panel and gateway, even when every world is scaled to zero.
- Fargate cold start (typically 30–90 seconds) plus game boot (1–5+ minutes) is visible to players. The gateway must handle that. See [ADR-0003](ADR-0003-edge-gateway-first.md).
- Privileged node features (DaemonSets, hostPath backups, node-local SSDs) are unavailable. Persistence is EFS.
- Operators will not get a drop-in Pelican UI. File management, consoles, and eggs have to be designed in the control-plane spec if we want them later. v1 only needs what [docs/specs/control-plane.md](../specs/control-plane.md) lists.
