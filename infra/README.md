# Cluster infrastructure

Terraform for the v1 EKS Fargate cluster. Spec: [docs/specs/operations.md](../docs/specs/operations.md), [ADR-0002](../docs/ADRs/ADR-0002-reject-wings-use-eks-fargate.md).

This stack creates:

- One VPC with two public subnets (NAT, later ALB/NLB) and two private subnets (pods)
- One NAT gateway (operations alternative A)
- One EKS cluster with API access for the caller who applies
- Fargate profiles for `kube-system`, `games-system`, and `games-worlds`
- The `games-system` and `games-worlds` namespaces
- `vpc-cni` and `coredns` (CoreDNS on Fargate)
- One encrypted EFS file system with mount targets in the private subnets
- Optional per-world access points when `world_ids` is set
- ACM certificate for `games.bradfordly.com` and `*.games.bradfordly.com`
- AWS Load Balancer Controller (Fargate Deployment) and an internet-facing panel Ingress (IP targets, `/healthz`)
- Internet-facing NLB on TCP 25565 (IP targets, health checks on admin `/healthz`)
- Panel and gateway ServiceAccounts plus namespaced Roles in `games-worlds`

It does **not** create game workloads, EC2 node groups, DaemonSets, or HostPort bindings. Label the panel and gateway Deployments `app.kubernetes.io/name=panel` and `app.kubernetes.io/name=gateway`.

Panel and gateway share `games-system`. Worlds use `games-worlds` so they cannot read panel secrets.

## Apply

Requires AWS credentials and `aws` CLI v2 (the Kubernetes provider calls `aws eks get-token`).

```bash
cd infra
cp terraform.tfvars.example terraform.tfvars   # optional
terraform init
terraform plan
terraform apply
aws eks update-kubeconfig --name bradfordly-games --region us-east-1
```

Lock `cluster_endpoint_public_access_cidrs` after the first apply.

## World saves (EFS)

One file system. One access point per world:

| Field | Value |
| --- | --- |
| Path | `/worlds/<world_id>` |
| POSIX | uid/gid `1000` |
| PV handle | `<file-system-id>::<access-point-id>` |

Creating access points by hand is fine for the first worlds. To have Terraform create them, set `world_ids` in `terraform.tfvars`. Then copy `k8s/examples/world-save.yaml`, replace the placeholders, and `kubectl apply` it in `games-worlds`. Mount only the save directory, not game binaries.

Fargate static PVs use the in-platform EFS CSI node. The CSI controller add-on is not required until something needs dynamic provisioning.

## Panel ALB

The Load Balancer Controller creates an internet-facing ALB from the `panel` Ingress in `games-system`:

| Field | Value |
| --- | --- |
| Host | `games.bradfordly.com` |
| Listen | HTTPS 443 with the ACM certificate |
| Targets | IP (Fargate) |
| Health check | HTTP `/healthz` on port 8080 |

Set `route53_zone_id` to write ACM validation records. Otherwise apply the `panel_certificate_validation` output in DNS.

## Gateway NLB

The Load Balancer Controller creates an internet-facing NLB from the `gateway` Service in `games-system`:

| Field | Value |
| --- | --- |
| Player listener | TCP 25565 |
| Targets | IP (Fargate) |
| Health check | HTTP `/healthz` on admin port 8080 |
| Not used for health | 25565 |

Label the gateway Deployment `app.kubernetes.io/name=gateway`. The NLB does not target game world pods.

## RBAC

ServiceAccounts `panel` and `gateway` are in `games-system`. Namespaced Roles in `games-worlds` bind them:

| Account | Worlds / Services / StatefulSets / PVCs | Replicas |
| --- | --- | --- |
| panel | get/list/watch/create/update/patch/delete | same verbs on workloads |
| gateway | get/list/watch | patch workloads and scale |

Neither Role is cluster-admin. Point the panel and gateway Deployments at these ServiceAccounts.

## Check without credentials

```bash
terraform fmt -check
terraform init -backend=false -input=false
terraform validate
```
