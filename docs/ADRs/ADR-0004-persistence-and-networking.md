# ADR-0004: Persistence and networking

## Status

Superseded by [ADR-0006](ADR-0006-pack-on-public-ec2.md)

Saves are EBS bind mounts. Ingress is one Elastic IP (Caddy for the panel, host ports for games). There is no EFS, ALB, NLB, or ClusterIP.

## Date

2026-10-03

## Context

Game worlds are stateful. A scale-to-zero pod loses everything that is not on a volume. EKS Fargate cannot mount EBS, cannot use `hostPath`, and does not support dynamic EFS provisioning. It can mount Amazon EFS through static PersistentVolumes.

Fargate pods cannot run in public subnets and cannot set `HostPort` or `HostNetwork`. Players therefore cannot dial a pod IP the way they would a VPS. Ingress must be a load balancer with IP targets.

Minecraft Java puts the target hostname in the handshake, so one TCP listener can route many worlds. Steam UDP games do not, so each Valheim or Palworld world needs its own port or address.

## Decision

### Persistence

1. **World saves live on Amazon EFS.** Create the file system and access points out of band. Bind them with static PersistentVolumes and PersistentVolumeClaims. One access point (or directory) per world.
2. **Mount only save data on EFS.** Game binaries and caches stay in the container image or ephemeral disk. EFS is billed and is slower than local NVMe; Minecraft chunk I/O in particular should not put the whole `/data` tree on NFS if a smaller save path is enough. Follow the image's documented save directory.
3. **Scale-down happens only after a graceful stop.** The gateway (or adapter) sends the game's clean shutdown (`SIGTERM`, RCON stop, or the image's documented signal) and waits for the process to flush. A hard kill after the stop timeout is allowed; data loss is then possible and must be visible in the panel.
4. **Backups are EFS snapshots or a copy job off the access point**, not node-local tar from Wings. The operations spec names the v1 backup minimum.

### Networking

1. **Panel traffic** (`games.bradfordly.com`) uses an internet-facing Application Load Balancer, HTTPS, IP targets, to the control-plane Deployment.
2. **Player traffic** uses an internet-facing Network Load Balancer, IP targets, to the edge-gateway Deployment. TCP and UDP listeners are both in scope. NLB health checks are TCP or HTTP on a gateway admin port; they are never UDP.
3. **Minecraft Java** shares one NLB TCP listener (25565). The gateway routes on handshake hostname, for example `survival.games.bradfordly.com` or `mc.games.bradfordly.com` plus a world name. DNS CNAMEs for those names point at the NLB.
4. **UDP worlds** get a dedicated NLB listener port (or a dedicated address if ports are exhausted). The gateway binds that port and proxies only to that world's Service. Hostname routing is not available.
5. **Game pods are ClusterIP Services** in private subnets. They are never type LoadBalancer. Only the gateway talks to them.
6. **Allocations** (the Pelican idea of a reserved endpoint) are first-class: each world has a stable public hostname and/or port that does not change when the pod IP changes.

## Consequences

- Creating a world means creating an EFS access point (or directory), a static PV/PVC, a Service, and a `0/1` workload before anyone can join.
- EFS latency may show up as Minecraft hitching. If that becomes a problem, the next ADR would be "EC2 node group plus EBS for that world," not a silent switch.
- Mixed TCP+UDP on one Kubernetes Service is awkward. Prefer one Service per protocol or separate NLB listeners rather than fighting mixed-protocol Services.
- Port allocation for UDP is a control-plane concern. The panel must show the public address players actually type into the client.
- One NLB can serve many listeners. Do not create an NLB per world in v1.
