# ADR-0006: Pack panel, gateway, and worlds on one public EC2

## Status

Accepted

## Date

2026-10-04

## Context

[ADR-0002](ADR-0002-reject-wings-use-eks-fargate.md) chose EKS Fargate. Isolation is not a requirement. EKS is not a requirement. The cost model in [operations.md](../specs/operations.md) showed:

| Option | Asleep monthly | What you pay extra for |
| --- | --- | --- |
| A · EKS Fargate + NAT | ~$182 | Kubernetes + private pods |
| D-min · ECS Fargate, one NLB, no NAT | ~$53 | Per-world Fargate isolation that actually leaves the bill |
| G · one public EC2 + custom gateway | ~$38 on `t3.medium` | Nothing. The VM is billed whether worlds sleep or not. |

@Bradfordly chose G. Idle shutdown remains a product feature (RAM, MOTD, leaving room for the next world). It is **not** a cost feature on G: the instance stays up.

This ADR supersedes the EKS/Fargate runtime in ADR-0002 and the EFS/ALB/NLB/ClusterIP decisions in [ADR-0004](ADR-0004-persistence-and-networking.md). [ADR-0003](ADR-0003-edge-gateway-first.md) still owns the gateway product; only its runtime changes. [ADR-0005](ADR-0005-identity-for-games-bradfordly.md) is unchanged.

## Decision

1. **Run v1 on one public EC2 instance** with an Elastic IP. Panel, gateway, and game containers share that host. Do not use EKS, ECS Fargate, ALB, NLB, NAT, or EFS for this product.
2. **Do not deploy Pelican, Pterodactyl, or Wings.** Wings would own the Docker daemon and fight the gateway for start/stop. The panel stays a thin custom app. Buy [itzg/mc-router](https://github.com/itzg/mc-router) in Docker mode (the socket exists again) and [Caddy](https://caddyserver.com) for HTTPS.
3. **v1 instance is `t3.medium`** (2 vCPU, 4 GiB) in `us-east-1`, Amazon Linux 2023, a small root volume plus a **50 GB gp3 data volume** (saves, SQLite, snapshots). Resize the type before shipping Valheim or Palworld; do not pay for `t3.large` until those adapters exist.
4. **Worlds, the panel, and the gateway are Docker containers** on the host. The gateway `docker start` / `docker stop`s worlds. One world running at a time is the v1 assumption. Saves are bind mounts on the data volume.
5. **World records are SQLite on the data volume.** OIDC secret and allowlist live in SSM Parameter Store. First issuer is **GitHub**. No RDS, Aurora, DynamoDB, Secrets Manager, or JSON-file store.
6. **Idle stop does not change the AWS bill.** Do not add instance-stop-on-idle in v1 (that would add a 1–2 minute EC2 cold start in front of game boot).

## Cost comparison

Estimates use **us-east-1 Linux On-Demand list prices**, **730 hours per month**. Budget model, not a quote. Rates from [operations.md](../specs/operations.md) (October 2026 snapshot). AWS only.

### Unit rates used

| Resource | Rate | Monthly if always on |
| --- | --- | --- |
| t3.medium | $0.0416 / hour | $30.37 |
| t3.large | $0.0832 / hour | $60.74 |
| t3.xlarge | $0.1664 / hour | $121.47 |
| EBS gp3 | $0.08 / GB-month | $4.00 for 50 GB |
| Public IPv4 (EIP) | $0.005 / hour | $3.65 |
| EBS snapshot | ~$0.05 / GB-month | ~$2.50 if 50 GB full-equivalent |
| D-min platform (from prior review) | — | ~$53 asleep |
| A · EKS Fargate + NAT | — | ~$182 asleep |

### Chosen G (`t3.medium`) versus rejected hosts

| Line | G t3.medium (chosen) | G t3.large | G t3.xlarge | D-min | A EKS |
| --- | --- | --- | --- | --- | --- |
| Compute | 30.37 | 60.74 | 121.47 | 18.02 (two Fargate tasks) | 91.02 (cluster + tasks) |
| Disk / EFS | 4.00 (50 GB gp3) | 4.00 | 4.00 | 3.20 (EFS One Zone 20 GB) | 6.00 (EFS Regional 20 GB) |
| Network | 3.65 (EIP) | 3.65 | 3.65 | 31.38 (NLB + IPv4s + LCU) | 84.76 (NAT + ALB + NLB) |
| **Monthly, worlds asleep** | **$38** | **$68** | **$129** | **$53** | **$182** |
| Plus Minecraft 40 h | $38 (already paid) | $68 | $129 | $55 | $184 |
| Plus Palworld 40 h | does not fit | $68 | $129 | $58 | $187 |
| Plus three worlds 40 h each | does not fit | tight | $129 | $64 | $192 |

Snapshots add about **$2–3** if enabled. Not in the $38 headline.

### Build vs buy on this box

| Piece | Build or buy | Monthly | Why |
| --- | --- | --- | --- |
| EC2 + EBS + EIP | Buy AWS | $38 | The whole platform. |
| Docker Engine | Buy | $0 | Needed for published game images. |
| mc-router (Docker mode) | Buy | $0 | Handshake, MOTD, start/stop via Docker socket. Writing this is wasted. |
| Caddy + Let's Encrypt | Buy | $0 | Replaces ALB/ACM. |
| Pelican + Wings | Reject | $0 license, same $38 host | Wings owns containers. We would still build a gateway and fight it. |
| Custom panel | Build | $0 extra | Invite-only list/detail/OIDC. Pelican's extra UI is out of scope. |
| Wake / idle / adapters | Build (thin) | $0 extra | Product. On G this is `docker start` / `docker stop` plus mc-router. |
| RDS / Aurora | Reject | $12–$43+ | Larger than the disk line. |
| DynamoDB / Secrets Manager | Reject | ~$0–$1 | SSM + a file on EBS already paid. |
| ALB / NLB / NAT / EKS / EFS | Reject | $44–$182 | Isolation and Kubernetes we are not buying. |

### What extra money would buy

- **+$30** (`t3.large`): Valheim or Palworld on this host, still one world at a time. **D-min is cheaper than this** (~$53 vs $68) if isolation ever becomes interesting again.
- **+$91** (`t3.xlarge`): three worlds overlapping. D-min at 40 h each is ~$64. Do not buy this size to "have room."
- **+$15** (stay on D-min instead of G): per-world Fargate isolation and a bill that drops when worlds sleep. Rejected; isolation is not required.
- **+$144** (stay on A): EKS. Rejected.

Break-even: G `t3.medium` is the cheapest measured AWS host that still matches the product for **Minecraft Java v1**. It loses to D-min the moment the box must be `t3.large` or larger.

## Rationale

Without isolation, paying Fargate + NLB to sleep containers is paying ~$15 extra versus a `t3.medium` that is already on. The Docker socket is the other win: mc-router's Docker mode was impossible on Fargate and is the Minecraft adapter.

One box is a single point of failure. That is acceptable for an invite-only panel.

## Consequences

- The control plane is a **Docker API client**, not a Kubernetes client. Specs that create StatefulSets, Services, CRs, or Fargate profiles are wrong.
- Player and panel traffic land on **one Elastic IP**. `games.bradfordly.com` and `*.games.bradfordly.com` are A records (or ALIAS) to that IP. Caddy terminates HTTPS for the panel. Game ports are not TLS.
- Game boot is container start + world load, not Fargate cold start + world load. First join can still take minutes. Occupy stays kick/retry.
- Minecraft I/O hits **EBS**, not EFS. That is the cheaper and faster save path.
- The EKS Terraform already on `main` must not be applied for this product. Replace it with a small instance + EIP + security group + IAM (SSM) stack.
- Security group is the isolation we have: 443 and 25565 public; RCON and SSH closed (SSM Session Manager for admin). A published world port is on the host IP, not a per-task ENI.
- `t3.medium` (4 GiB) fits vanilla Minecraft + panel + gateway. It does **not** fit Palworld (8 GiB profile) or a heavy modpack. Resize before those ship.
- Stacked feature PRs that targeted Kubernetes never landed on `main` and should not be rebased onto G. New issues, new branches.
- Implementation issues #57–#60 locked the former open questions: separate data volume, panel and gateway as containers, SQLite, GitHub OIDC with the allowlist in SSM.
