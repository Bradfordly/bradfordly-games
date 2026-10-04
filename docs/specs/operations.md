# Operations

## Purpose

How the system is deployed on AWS, what it costs when asleep, and how worlds shut down without losing saves. Product behavior is in the other specs. Decision: [ADR-0006](../ADRs/ADR-0006-pack-on-public-ec2.md).

## Topology

- One public EC2 instance (`t3.medium` for Minecraft v1) with an Elastic IP.
- Docker Engine on the host. Panel, Caddy, gateway, and game containers share that daemon (panel/gateway may instead be systemd units).
- World saves are bind mounts on EBS. No EFS, no EKS, no ECS Fargate, no NAT, no ALB, no NLB.
- DNS: `games.bradfordly.com` and `*.games.bradfordly.com` A / ALIAS to the Elastic IP. UDP allocations use the same IP plus a host port.
- Admin access is SSM Session Manager. Do not open SSH to the world.

## Always-on versus scale-to-zero

Always on (billed whenever the product exists):

- The EC2 instance, EBS volume, and Elastic IP

Scale to zero (frees RAM, does **not** lower the AWS bill):

- Each game world container

## Cost model

Estimates use **us-east-1 Linux On-Demand list prices**, **730 hours per month**, and the game sizes in [game-adapters.md](game-adapters.md). They are a budget model, not a quote. Recheck the [EKS](https://aws.amazon.com/eks/pricing/), [Fargate](https://aws.amazon.com/fargate/pricing/), [EC2](https://aws.amazon.com/ec2/pricing/on-demand/), [ELB](https://aws.amazon.com/elasticloadbalancing/pricing/), [VPC](https://aws.amazon.com/vpc/pricing/), and [EFS](https://aws.amazon.com/efs/pricing/) pages before spending. Rates below were captured in October 2026 from public list prices (Fargate/EKS/ELB/NAT/EFS as of mid-2026 reviews; EC2 as of September 2026).

Traffic (NLB LCU, NAT GB, internet out) is modeled as a small allowance. A public server with community listing or many downloads will exceed it.

### Unit rates

| Resource | Rate | Monthly if always on |
| --- | --- | --- |
| EKS control plane | $0.10 / hour | $73.00 |
| Fargate vCPU (x86) | $0.04048 / hour | — |
| Fargate memory (x86) | $0.004445 / GB-hour | — |
| Fargate vCPU (Graviton, ECS only) | $0.03238 / hour | ~20% below x86 |
| NAT Gateway | $0.045 / hour + $0.045 / GB | $32.85 + data |
| ALB | $0.0225 / hour + $0.008 / LCU-hour | $16.43 + usage |
| NLB | $0.0225 / hour + $0.006 / NLCU-hour | $16.43 + usage |
| Public IPv4 | $0.005 / hour | $3.65 each |
| EFS Regional Standard | $0.30 / GB-month | $6.00 for 20 GB |
| EBS gp3 | $0.08 / GB-month | $4.00 for 50 GB |
| t3.medium (2 vCPU, 4 GiB) | $0.0416 / hour | $30.37 |
| t3.large (2 vCPU, 8 GiB) | $0.0832 / hour | $60.74 |
| t3.xlarge (4 vCPU, 16 GiB) | $0.1664 / hour | $121.47 |
| m6i.large (2 vCPU, 8 GiB) | $0.0960 / hour | $70.08 |
| m6i.xlarge (4 vCPU, 16 GiB) | $0.1920 / hour | $140.16 |
| m6i.2xlarge (8 vCPU, 32 GiB) | $0.3840 / hour | $280.32 |

Fargate bills the requested size from image pull until the pod or task stops, one-minute minimum. EKS Fargate has **no Spot**. ECS Fargate Spot exists but is interruptible, so it is not a game-world option.

### Fargate task sizes

Hourly = `vCPU × 0.04048 + GiB × 0.004445`.

| Role | Size | Hourly | 10 h | 40 h | 120 h | 730 h (24/7) |
| --- | --- | --- | --- | --- | --- | --- |
| Panel | 0.25 vCPU / 0.5 GiB | $0.0123 | — | — | — | $9.01 |
| Gateway (quiet) | 0.25 vCPU / 0.5 GiB | $0.0123 | — | — | — | $9.01 |
| Gateway (busier) | 0.5 vCPU / 1 GiB | $0.0247 | — | — | — | $18.02 |
| Minecraft Java | 1 vCPU / 2 GiB | $0.0494 | $0.49 | $1.97 | $5.92 | $36.04 |
| Valheim | 2 vCPU / 4 GiB | $0.0987 | $0.99 | $3.95 | $11.85 | $72.08 |
| Palworld | 2 vCPU / 8 GiB | $0.1165 | $1.17 | $4.66 | $13.98 | $85.06 |
| Three worlds together | sum of the three | $0.2646 | $2.65 | $10.58 | $31.75 | $193.18 |

Panel and gateway are always on in every serverless alternative. Game rows apply only while a world is `starting`, `online`, or `stopping`.

### Alternatives measured

Each alternative is the same product idea (panel + player listener + one or more worlds) on a different substrate. **v1 is G** ([ADR-0006](../ADRs/ADR-0006-pack-on-public-ec2.md)). The other rows stay so the rejected fees stay visible.

| ID | Alternative | What stays billed when worlds sleep | What is dropped vs EKS Fargate |
| --- | --- | --- | --- |
| A | **EKS Fargate, 1-AZ NAT** | Cluster, panel, gateway, NAT, ALB, NLB, EFS | — |
| B | **EKS Fargate, 2-AZ NAT** | A plus a second NAT | Nothing; higher HA |
| C | **ECS Fargate, private + NAT** | Same as A minus the EKS cluster | $73 cluster |
| D | **ECS Fargate, public subnets** | Panel, gateway, ALB, NLB, EFS | Cluster and NAT. EKS Fargate cannot do this (no public-subnet pods). |
| E | **EKS + one public EC2 node** | Cluster + node. Panel and gateway run on the node. Worlds are Docker/K8s on the same box. | Fargate task fees, NAT, optional LBs |
| F | **Public EC2 + Pelican/Pterodactyl** | One VM, EBS, one IPv4. Games stay up unless you add extra idle tooling. | Cluster, Fargate, NAT, ALB, NLB, EFS |
| G | **Public EC2 + custom gateway** (chosen v1, `t3.medium`) | One VM. Containers stop when idle; the gateway process stays on the VM. | Same as F, but worlds can sleep *inside* the still-paid VM |

Shared assumptions for A–D: quiet gateway (0.25 / 0.5), 20 GB EFS, 1 ALB (2 IPv4), 1 NLB (1 IPv4), light LCU $4, NAT data 10 GB ($0.45) when a NAT exists. A/C use one NAT + its IPv4.

### Always-on platform (worlds asleep)

| Line | A EKS Fargate 1-AZ | B EKS Fargate 2-AZ | C ECS Fargate + NAT | D ECS Fargate public | E EKS + m6i.xlarge public | F Pelican t3.xlarge | G Custom gateway t3.xlarge |
| --- | --- | --- | --- | --- | --- | --- | --- |
| EKS cluster | 73.00 | 73.00 | 0 | 0 | 73.00 | 0 | 0 |
| Panel compute | 9.01 | 9.01 | 9.01 | 9.01 | (on node) | (on VM) | (on VM) |
| Gateway compute | 9.01 | 9.01 | 9.01 | 9.01 | (on node) | n/a | (on VM) |
| NAT Gateway | 32.85 | 65.70 | 32.85 | 0 | 0 | 0 | 0 |
| NAT IPv4 | 3.65 | 7.30 | 3.65 | 0 | 0 | 0 | 0 |
| NAT data (10 GB) | 0.45 | 0.45 | 0.45 | 0 | 0 | 0 | 0 |
| ALB + 2 IPv4 | 23.73 | 23.73 | 23.73 | 23.73 | 0 | 0 | 0 |
| NLB + 1 IPv4 | 20.08 | 20.08 | 20.08 | 20.08 | 0 | 0 | 0 |
| LCU allowance | 4.00 | 4.00 | 4.00 | 4.00 | 0 | 0 | 0 |
| EFS 20 GB | 6.00 | 6.00 | 6.00 | 6.00 | 0 | 0 | 0 |
| EC2 | 0 | 0 | 0 | 0 | 140.16 | 121.47 | 121.47 |
| EBS | 0 | 0 | 0 | 0 | 8.00 | 6.40 | 6.40 |
| Node / VM IPv4 | 0 | 0 | 0 | 0 | 3.65 | 3.65 | 3.65 |
| **Monthly, worlds asleep** | **$182** | **$218** | **$109** | **$72** | **$225** | **$131** | **$131** |

E and F/G pack panel and games on one machine. They do not give per-world Fargate isolation. F has no wake-on-connect unless you add a proxy. G can stop containers, but you still pay the VM.

### Plus game hours

Add the Fargate game-size table to A–D. E–G do not add Fargate game hours; the VM is already paid (until you outgrow it).

| Workload on top of the platform | Add to A–D | A total | C total | D total | E / F / G total |
| --- | --- | --- | --- | --- | --- |
| Minecraft 10 h | $0.49 | $182 | $109 | $72 | $225 / $131 / $131 |
| Minecraft 40 h (weekends) | $1.97 | $184 | $111 | $74 | same (VM already on) |
| Minecraft 120 h (evenings) | $5.92 | $188 | $115 | $78 | same |
| Minecraft 24/7 | $36.04 | $218 | $145 | $108 | same, if the VM still fits |
| Valheim 40 h | $3.95 | $186 | $113 | $76 | same |
| Valheim 24/7 | $72.08 | $254 | $181 | $144 | F/G t3.xlarge is tight; prefer m6i.xlarge **$152** |
| Palworld 40 h | $4.66 | $187 | $113 | $77 | same |
| Palworld 24/7 | $85.06 | $267 | $194 | $157 | needs ≥8 GiB; t3.large **$68** if panel is tiny, t3.xlarge **$131** if packed |
| All three, 40 h each | $10.58 | $192 | $119 | $82 | t3.xlarge **$131** (packed, always on) |
| All three, 120 h each | $31.75 | $214 | $141 | $104 | t3.xlarge **$131** (may OOM if all three overlap) |
| All three, 24/7 | $193.18 | $375 | $302 | $265 | m6i.2xlarge **$294** |

A totals use the 1-AZ NAT baseline ($182). Add **$36** to any A column for alternative B (second NAT).

### How to read this

- **G `t3.medium` is the chosen v1 host (~$38).** Idle does not lower that number. The G `t3.xlarge` column below is the old packed-host row (~$131), not the chosen size.
- **EKS Fargate (A) is the expensive serverless option.** The cluster ($73) plus NAT ($33+) dominate. Scale-to-zero only saves the *game* Fargate row. A sleeping Minecraft world on A still costs ~$182/month.
- **ECS Fargate without NAT (D) is the cheap serverless option.** Same wake/idle story, about **$110/month less** than A when worlds sleep, because there is no cluster fee and EKS cannot put Fargate pods in public subnets. This is the cost-cut in ADR-0002.
- **ECS Fargate + NAT (C)** is the middle: keep private pods, drop only the cluster fee (**−$73** vs A).
- **A single public EC2 (F/G) wins if something must stay up 24/7 and you accept packing.** ~$131 for a t3.xlarge holds a panel plus one or two worlds with no ALB/NLB/NAT. You lose per-world isolation and, in F, idle shutdown unless you build G's gateway on the box.
- **EKS + EC2 (E) is the worst of both for this size:** you pay the cluster *and* a node. Skip it unless you already need Kubernetes for other workloads on that node.
- **Scale-to-zero pays off on A–D when games are large and mostly idle.** Valheim 24/7 on Fargate is $72 of compute alone; 40 h is $4. Palworld is the same pattern. Minecraft compute is cheap either way; it will not justify EKS by itself.
- **Break-even, one Minecraft world:** D ($72 + hours) stays below F/G ($131) even if Minecraft is 24/7 ($108). A ($182 + hours) never beats a t3.xlarge for a single packed host; you are paying for Kubernetes and isolation, not for Minecraft hours.
- **Break-even, all three worlds 24/7:** D ($265) and a dedicated m6i.2xlarge ($294) are close. A ($375) loses. If the three worlds overlap only on weekends (40 h), D ($82) beats any always-on VM that can host all three.

### Trade-offs of alternative D (ECS Fargate, public subnets)

D is the cheapest measured option that keeps the product shape: always-on panel and gateway, per-world Fargate isolation, wake-on-connect, idle scale-to-zero. At **40 hours of world on-time per month** the totals are about **$74** (Minecraft only), **$76** (Valheim), **$77** (Palworld), or **$82** (all three at 40 h each). Those add only a few dollars on top of the **$72** asleep platform. Alternative A is ~$184 / $192 for the same play.

40 h here means hours the *game task* is billed, not hours the platform exists. The panel, gateway, ALB, and NLB still run all month.

D is not the cheapest thing we measured in every slice. A packed public `t3.large` running a custom gateway (G, smaller than the $131 `t3.xlarge` row) is about **$68** always on. That beats D for **Minecraft-only**. It loses as soon as you want Valheim or Palworld isolation, or three worlds that must not share one box.

#### What you gain versus A

- **−$110/month** asleep: no EKS control plane (−$73) and no NAT (−$37 including its IPv4 and a little data).
- Same Fargate unit rates and the same idle math. `desiredCount` 0 on an ECS service is valid. The gateway still scales 0/1.
- ECS has had UDP NLB to Fargate longer than EKS. Graviton Fargate exists on ECS if an image is ARM.
- No Kubernetes upgrades, no Load Balancer Controller, no Fargate profile. The AWS surface is a cluster, task definitions, and services.

#### What you give up versus A

- **The control plane is rewritten.** Specs that patch StatefulSet replicas and store worlds as Kubernetes objects become ECS `UpdateService` / `RunTask` plus another store (SSM, DynamoDB, or files on EFS). [ADR-0002](../ADRs/ADR-0002-reject-wings-use-eks-fargate.md) chose EKS so the panel could be a Kubernetes client and so `mc-router` could scale StatefulSets. That path does not transfer.
- **`mc-router` in-cluster mode is gone.** Its Docker mode needs a Docker socket, which Fargate does not have. The Minecraft adapter must be a standalone proxy (custom, or mc-router with a static map the control plane rewrites) talking to Cloud Map or the task's private IP after each wake.
- **No ClusterIP.** Every wake gives the world a new ENI and IP. The gateway has to rediscover the backend. Player-facing hostnames stay on the NLB in front of the *gateway*, not the world. Do not hand players the task public IP; it changes and skips idle detection.
- **Public subnets are how D avoids NAT.** A Fargate task in a public subnet with `assignPublicIp` disabled cannot pull images or reach ECR/Docker Hub. So D implies `assignPublicIp=ENABLED` on tasks that need egress, *or* a set of VPC endpoints (which starts to look like C). Panel and gateway each then have an extra public IPv4 (~$3.65/month each). Game tasks add $0.005/hour only while running (~$0.20 at 40 h). Those IPs were not in the $72 row; add ~$7 if both always-on tasks get public IPs (**~$79** asleep). Inbound must be security-group locked to the ALB/NLB (and to the gateway SG for world ports). RCON and Palworld REST stay closed. A mistaken `0.0.0.0/0` on a world task publishes that game on a public IP. A and C fail closed for internet-to-pod.
- **You still pay for the ALB and NLB (~$44).** Dropping them to save money also drops HTTPS on `games.bradfordly.com` and a stable player allocation. That is no longer D; it is a different product.

#### What stays the same as A

- Fargate cold start (30–90 s) plus world boot (1–5+ min). UDP clients still retry. Minecraft can still kick/hold.
- EFS for saves. One access point per world. Same graceful SIGTERM story.
- Invite-only panel. Game passwords stay on the world.
- One replica per world. No Agones.

#### Compared with C and G

| | D | C (ECS + NAT, private) | G (packed EC2 + gateway) |
| --- | --- | --- | --- |
| 40 h, three worlds | ~$82 (~$79+$11 if you count task IPv4s) | ~$119 | ~$131 on `t3.xlarge`; ~$68 on `t3.large` if Minecraft-only |
| World isolation | Yes | Yes | No |
| Internet to world ENI | Possible if SG is wrong | No | Yes, one static host |
| Orchestrator change vs current spec | ECS API | ECS API | Docker on one VM |
| NAT / cluster fee | Neither | NAT only | Neither |

C is the safer ECS option: same rewrite as D, keep private tasks, pay $33 for NAT. Pick C if the public-IP blast radius is unacceptable and $119 at 40 h is fine. Pick G if one machine and a static IP are enough and you do not need per-world Fargate. Pick D if $70–85/month for isolated, sleeping worlds is the goal and you will treat security groups as part of the product.

D and D-min were rejected when isolation was dropped. Do not implement them while [ADR-0006](../ADRs/ADR-0006-pack-on-public-ec2.md) says G.

### Choices that move the bill

| Choice | Monthly effect vs A |
| --- | --- |
| Move to ECS Fargate, keep NAT (C) | −$73 |
| Move to ECS Fargate, public subnets (D) | −$110 |
| Second NAT AZ (B) | +$36 |
| Size gateway 0.5 / 1 GiB | +$9 |
| Graviton Fargate (ECS images only) | about −20% of Fargate compute rows |
| Leave Palworld up 24/7 | +$85 on A–D |
| VPC endpoints instead of NAT (ECR, S3, logs, EFS) | can remove most of the $33 NAT if image pulls stop using NAT; extra endpoint hourly fees apply |
| Fargate Spot for worlds | not used; interruptions corrupt saves |
| Savings Plans | lowers Fargate/EC2 unit rates after a 1-year commitment; not assumed here |

The panel must show hours online in the last day so a forgotten Palworld or Valheim world is visible before it adds tens of dollars.

### Cost envelope (short)

| Item | When | Monthly |
| --- | --- | --- |
| **G `t3.medium` (chosen)** | Always | **~$38** |
| G with weekly EBS snapshots | Always | ~$41 |
| D-min ECS (rejected) | Worlds asleep | ~$53 |
| A EKS Fargate (rejected) | Worlds asleep | ~$182 |
| Resize to `t3.large` (before Valheim/Palworld) | Always | ~$68 |
| Extra per Minecraft / Valheim / Palworld hour | On G | $0 (host already paid) |

## Capacity

The instance type must fit the OS, panel, gateway, and **one** running world ([game-adapters.md](game-adapters.md)). A world that asks for more RAM than is free will sit in `starting` until `start_timeout`.

v1 assumes one world running at a time. Do not start a second world on `t3.medium`.

## Graceful stop

1. Gateway enters `stopping` and calls the adapter `GracefulStop`.
2. Docker sends SIGTERM (or the adapter's signal) and honors `stop_timeout`.
3. The image flushes the world to the EBS bind mount.
4. Only then is the container left stopped.
5. If the process is still running at `stop_timeout`, it is killed. The panel shows that the last stop was forced.

Never set a world's Docker stop timeout to a few seconds. Minecraft and Valheim need time to write.

## Backups (v1 minimum)

- AWS Backup or scheduled EBS snapshots on the data volume (or the root volume if there is no data volume).
- One on-demand snapshot before a world's save directory is deleted.
- Restore is "create a new world pointed at a restored directory," not a button in v1.

Do not copy worlds through the panel API.

## Secrets

- OIDC client secret: SSM Parameter Store SecureString, not in world env.
- Game passwords and RCON: files or env on the host, bound into the container. Do not publish RCON on the security group.
- Palworld REST basic auth: localhost / Docker network only.
- Allowlist: SSM or a file on disk (emails are not credentials).

## Health checks

- Panel: HTTP `/healthz` behind Caddy.
- Gateway: HTTP `/healthz` on a host-only or localhost admin port (not 25565).
- Game containers: Docker health or the adapter ready check. Do not expose that check on the Elastic IP.
- UDP game health is the adapter ready check after the container is up.

## DNS and certificates

- Caddy + Let's Encrypt for `games.bradfordly.com`. Wildcard Minecraft names do not need HTTPS (game protocol is not TLS).
- Game TCP/UDP on the Elastic IP is not TLS.
- Minecraft SRV records are optional. If used, they must still land on the Elastic IP:25565.

## Deploy

Implementation issues choose Terraform versus a small cloud-init. Requirements:

- One public instance, Elastic IP, security group, IAM instance profile (SSM).
- Docker Engine and Caddy on the host.
- Images pulled from a registry the instance can reach (Docker Hub or ECR).
- Do not apply the EKS stack in `infra/`.

## Operations the panel must not hide

- Last wake cause and time
- Last forced stop
- Current container running state versus gateway state
- Hours online in the last day (cost signal)

## Open operational questions

- Whether the instance is in `us-east-1` or closer to the usual player set. The cost model is us-east-1; other regions are usually a few percent higher.
- Who receives alerts when a world is `failed` (email, Slack, GitHub issue: later).
- When to resize from `t3.medium` to `t3.large` (before Valheim/Palworld, or when a modpack does not fit).
