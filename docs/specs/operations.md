# Operations

## Purpose

How the system is deployed on AWS, what it costs when asleep, and how worlds shut down without losing saves. Product behavior is in the other specs. Decisions: [ADR-0002](../ADRs/ADR-0002-reject-wings-use-eks-fargate.md), [ADR-0004](../ADRs/ADR-0004-persistence-and-networking.md).

## Topology

- One EKS cluster. Fargate profiles for the panel namespace, the gateway namespace (or the same namespace), and the worlds namespace.
- Private subnets only for pods. Public subnets hold the ALB, NLB, and NAT.
- AWS Load Balancer Controller creates the ALB and NLB with IP target types.
- One EFS file system. Per-world access points. Static PVs.
- DNS: `games.bradfordly.com` → ALB. Minecraft world hostnames → NLB. UDP allocations use the NLB hostname plus a port.

v1 may share one namespace if RBAC stays tight. Split namespaces if worlds should not read panel secrets.

## Always-on versus scale-to-zero

Always on (billed whenever the product exists):

- EKS control plane
- Control-plane Deployment (small)
- Edge-gateway Deployment (small, single replica)
- NAT gateway, ALB, NLB
- EFS (storage + a little throughput)

Scale to zero:

- Each game world Fargate pod

## Cost envelope

Figures are order-of-magnitude, not a quote.

| Item | When | Rough magnitude |
| --- | --- | --- |
| EKS cluster | Always | ~$0.10/hour (~$73/month) |
| Panel + gateway Fargate | Always | Tenths of a vCPU each; a few dollars to low tens per month |
| ALB + NLB + NAT | Always | Tens per month depending on traffic and AZs |
| EFS | Always | Cents to dollars for small worlds |
| Game pod | Only while `starting` / `online` / `stopping` | Dominates the bill if a Valheim or Palworld world is left up |

Scale-to-zero pays off when worlds are idle most hours. If a world runs 24/7, Fargate is usually more expensive than a small EC2 VM. The panel should make "this world has been online for N hours" visible.

ECS Fargate would drop the EKS fee. That is a later cost-cut, not v1. See [ADR-0002](../ADRs/ADR-0002-reject-wings-use-eks-fargate.md).

## Capacity

Fargate profiles must allow the CPU/memory in the game profiles ([game-adapters.md](game-adapters.md)). A world that asks for more than the profile allows will sit in `starting` until `start_timeout`.

Do not pack two worlds in one pod.

## Graceful stop

1. Gateway enters `stopping` and calls the adapter `GracefulStop`.
2. Kubernetes sends SIGTERM (or the adapter's signal) and honors `terminationGracePeriodSeconds` ≥ `stop_timeout`.
3. The image flushes the world to EFS.
4. Only then are replicas set to 0.
5. If the process is still running at `stop_timeout`, it is killed. The panel shows that the last stop was forced.

Never set a world's `terminationGracePeriodSeconds` to a few seconds. Minecraft and Valheim need time to write.

## Backups (v1 minimum)

- EFS replication or AWS Backup on the file system.
- One on-demand backup before a world PVC is deleted.
- Restore is "create a new world pointed at a restored access point," not a button in v1.

Do not copy worlds through the panel API.

## Secrets

- OIDC client secret: Kubernetes Secret, not in world env.
- Game passwords and RCON: Kubernetes Secrets mounted into the world pod.
- Palworld REST basic auth: cluster-internal Secret; not an NLB listener.
- Allowlist: ConfigMap is fine (emails are not credentials).

## Health checks

- Panel: HTTP `/healthz` on the ALB target group.
- Gateway: HTTP `/healthz` on the NLB TCP health check port (not 25565 if that would look like a Minecraft client).
- Game pods: Kubernetes probes only. NLB must not target game pods.
- UDP game health is the adapter ready check after the pod is up.

## DNS and certificates

- ACM certificate for `games.bradfordly.com` and `*.games.bradfordly.com` on the ALB.
- Game TCP/UDP on the NLB is not TLS (game protocols).
- Minecraft SRV records are optional. If used, they must still land on the NLB:25565.

## Deploy

Implementation issues choose Terraform versus CDK versus raw manifests. Requirements:

- Fargate-only node compute for these workloads.
- Load Balancer Controller installed (cannot run as a Fargate DaemonSet; it runs as a Deployment).
- EFS CSI available to Fargate (static PVs).
- Images pulled from a registry Fargate can reach.

## Operations the panel must not hide

- Last wake cause and time
- Last forced stop
- Current replica count versus gateway state
- Hours online in the last day (cost signal)

## Open operational questions

- Single AZ versus multi-AZ NLB (cost versus a world that must stay on one EFS mount target).
- Whether the first cluster is in `us-east-1` or closer to the usual player set.
- Who receives alerts when a world is `failed` (email, Slack, GitHub issue: later).
