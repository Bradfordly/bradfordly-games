# Data volume backups

Additive Terraform for [issue #63](https://github.com/Bradfordly/bradfordly-games/issues/63). It does **not** replace the EKS stack and does **not** create the public EC2 host or the 50 GB data volume ([issue #57](https://github.com/Bradfordly/bradfordly-games/issues/57)).

Apply this root (or copy the resources into the option G stack) and wire:

1. Tag the data volume `Role = data-volume` (or set `volume_role_tag`).
2. Attach `on_demand_snapshot_policy_arn` to the instance profile.
3. Set `GAMES_DATA_VOLUME_ID` on the panel to that volume's id.

Scheduled snapshots are weekly DLM policies. They add about $2–3/month and are not in the $38 headline ([ADR-0006](../../docs/ADRs/ADR-0006-pack-on-public-ec2.md)).

Restore is an operator step: restore the snapshot, then create a new world pointed at the restored directory. Do not copy worlds through the panel API.

```bash
cd infra/backups
terraform init
terraform plan
```
