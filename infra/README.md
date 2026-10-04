# Cluster infrastructure

Terraform for the **rejected** EKS Fargate cluster. v1 runtime is one public EC2 host ([ADR-0006](../docs/ADRs/ADR-0006-pack-on-public-ec2.md)). Do not apply this stack for the product.

This stack creates:

- One VPC with two public subnets (NAT, later ALB/NLB) and two private subnets (pods)
- One NAT gateway (operations alternative A)
- One EKS cluster with API access for the caller who applies
- Fargate profiles for `kube-system`, `games-system`, and `games-worlds`
- The `games-system` and `games-worlds` namespaces
- `vpc-cni` and `coredns` (CoreDNS on Fargate)

It does **not** create game workloads, EC2 node groups, DaemonSets, HostPort bindings, the load balancer controller, or an ALB/NLB.

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

## Check without credentials

```bash
terraform fmt -check
terraform init -backend=false -input=false
terraform validate
```
