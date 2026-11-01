# 0002. Game server hosting model

- Status: Proposed
- Date: 2026-10-12
- Amended by: [0003](0003-control-plane-server-lifecycle-and-idle-shutdown.md#amends-adr-0002) (A1–A10: host-agent signals; instance IAM, including a Deny-only flood-guard policy the control plane can attach at runtime; boot sequence with agent verification and conntrack accounting; admin-reason file; agent power-off rules and hold; interim `readyPattern`; lock and attempt rules, with the control plane stopping failed starts; extended grants incl. read-before-delete DNS; AWS-side backstops, an 80 h/month run cap and a flood-guard latch; revisit trigger met); [0004](0004-game-packages-and-per-server-configuration.md#amends-adr-0002) (C1–C11: JSON package manifests published as SSM projections, plus at most one server record per package, replace the catalog; `/srv/secrets` retired for write-only SecureStrings and a tmpfs env-file; `game-agent prepare` before `docker run`; fail-closed reasons `server-invalid`, `secret-missing`, `volume-mismatch` and `prepare-failed`, with world directories created only on the pinned data volume; new instance SSM paths; tag values and record names generated from `catalog/` package ids; onboarding checklist replaced; one server per game, never two running at once; pinned `ssm-user` uid; Never-granted list extended for SSM writes on packages and secret reads); [0005](0005-wake-on-connect.md#amends-adr-0002) (E1–E6: Route 53 query logging on the zone to a 3-day log group with its resource policy; SOA TTL and negative-caching minimum of 60 s, set once outside CDK and checked by the deploy preflight; record rule and Stop's DELETE kept, so NXDOMAIN while stopped is the wake signal; lookup-started runs as unrequested runs, bounded by cooldown, daily and 60 h limits; query log group in Configuration; `logs:FilterLogEvents` for `events` only; Never-granted list extended: no query-logging changes, no `logs:*` on the query log group except that read, and no `AWSLambdaBasicExecutionRole`)

## Context

Issue #3. Decide how the game server process runs on AWS (the "server plane"): compute, start/stop, player networking, where world data lives across scale-to-zero, AMI/patching and instance IAM, plus the contract the control plane must honour. Designing the control plane (lifecycle API, idle detection, auth, DNS updates) is out of scope: that is ADR-0003, `docs/adr/0003-control-plane-server-lifecycle-and-idle-shutdown.md` ("0003. Control plane: server lifecycle API and idle shutdown"). ADR-0001 fixes TypeScript CDK and a Go control plane on Lambda.

Owner constraints (authoritative; they override the issue's criteria weighting):

- **Cost first.** Total idle cost (no players) stays **under $5/month**, hosted zone included. Long cold starts are acceptable; cold-start time is the lowest-weighted criterion.
- Compute **scales to zero at zero players**. **On-demand only, no Spot.**
- **Several game servers/worlds, one running at a time**; at most 10 players; an 8 GiB `m6a.large` is acceptable.
- **Persistent worlds managed hands-on** (config, mods, admin console). Mods are downloaded on the instance; there is no upload path.
- Games are **Linux dedicated servers shipped as container images**; no specific game is chosen. CPU architecture and save-on-SIGTERM are per-game properties.
- Players connect via a **Route 53 hostname the control plane sets on start**. CDK in this repo creates a dedicated `games.bradfordly.com` hosted zone; the owner adds the NS delegation at the parent domain by hand.
- Region **us-east-1**; UDP must work.

Confirmed assumptions (owner, 2026-10-12): game ports are open to the internet and access control is the game's own allowlist or password; admin access is SSM Session Manager only (no SSH, no key pair); one AWS account and one environment; daily snapshots kept 7; baseline container hardening without user-namespace remapping; egress limited to TCP 443 plus declared per-game ports (port 80 dropped: nothing in the base design needs it); server-side power-off backstops (4 h admin mode, 12 h uptime); one reboot after a boot-time kernel update.

Verified facts (as of 2026-10-12, us-east-1 unless noted; "(unverified)" marks the rest):

- Stopping an EBS-backed instance ends instance charges while EBS keeps billing; the auto-assigned public IPv4 is released and replaced at the next start; each start bills at least 1 minute. Stop is a graceful ACPI shutdown with "no configurable OS shutdown time", and EC2 hard-stops the instance if the OS has not shut down "within a few minutes". Auto Scaling terminates rather than stops on scale-in [1].
- Public IPv4 costs $0.005/hour in use or idle (an Elastic IP is about $3.65/month even when stopped); a NAT gateway is $0.045/hour plus $0.045/GB (about $32.85/month idle); an NLB is $0.0225/hour plus NLCU-hours (about $16.43/month idle) [2].
- EBS gp3 is $0.08/GB-month and snapshots $0.05/GB-month (page examples; region unnamed) [3]. DLM costs nothing beyond its snapshots; one policy holds up to four schedules with their own retention, and schedules that fire together create one snapshot kept for the longest retention [4]. Snapshot Block Public Access and EBS encryption by default are free per-Region account settings; AWS managed keys such as `aws/ebs` have no monthly fee [5].
- On-demand Linux (Vantage, third-party, 2026-10-12): m6a.large (2 vCPU / 8 GiB) $0.0864/hour, r6a.large (16 GiB) $0.113, t3a.large $0.0752 [6]. t3a.large has a 30% per-vCPU baseline and defaults to Unlimited (T3 surplus $0.05/vCPU-hour; T3a not listed separately). The first 100 GB/month of internet egress is free across services; the $0.09/GB rate beyond that is third-party sourced (unverified) [7].
- Price List API (2026-09-11): Fargate x86 $0.04048/vCPU-hour and $0.004445/GB-hour; EFS Standard $0.30 and One Zone $0.16/GB-month, Elastic Throughput $0.03/GB read and $0.06/GB write [8]. ECS-managed EBS volumes on service tasks "are always deleted upon task termination" [9]. GameLift Linux m6a.large on-demand is $0.111/hour (2026-09-23), about 28% above EC2; fleets can scale to and from zero [10].
- Route 53: a hosted zone is $0.50/month and queries $0.40/million; `ChangeResourceRecordSets` supports the multi-valued keys `route53:ChangeResourceRecordSetsNormalizedRecordNames` (lowercase, no trailing dot), `…RecordTypes` and `…Actions` (`CREATE`/`UPSERT`/`DELETE`) with `ForAllValues:StringEquals` [11].
- Parameter Store standard parameters and Session Manager on EC2 are free. The documented minimal Session Manager instance policy is `ssm:UpdateInstanceInformation` plus `ssmmessages:CreateControlChannel`, `CreateDataChannel`, `OpenControlChannel`, `OpenDataChannel` on `*`; `AmazonSSMManagedInstanceCore` additionally allows `ssm:GetParameter(s)` (and more) on `*`. SSM Agent needs outbound HTTPS 443 to the `ssm` and `ssmmessages` endpoints [12]. AWS Budgets without actions are free [13].
- CloudFormation: on `AWS::EC2::Instance`, changing `ImageId` or `LaunchTemplate` replaces the instance, `UserData` restarts it, root block-device changes other than `DeleteOnTermination` replace it, and `MetadataOptions`/`Volumes` update in place. Replacement normally creates the new resource before deleting the old. Every `AWS::EC2::VolumeAttachment` property requires replacement. `AWS::EC2::Volume` `Size` grows in place, and EBS cannot shrink [14].
- CDK: `MachineImage.latestAmazonLinux2023()` replaces instances whenever a new AMI is published; `cachedInContext` pins the AMI. In aws-cdk-lib 2.273.0 (installed in `infra/`), `requireImdsv2` works by adding a launch template and cannot be combined with the individual `httpTokens`/`httpPutResponseHopLimit`/`instanceMetadataTags` props; `userDataCausesReplacement` defaults to true when `initOptions` is set [15].
- AL2023 doesn't update itself; `dnf upgrade --releasever=…` moves releases. Repositories are served over HTTPS from S3, DNF ships the `needs-restarting` plugin, and kernel updates need a reboot. `gpgcheck=1` is the default (`repo_gpgcheck` is off). End of support is 2029-06-30 [16]. Docker is packaged; SSM Agent and AWS CLI v2 are preinstalled [17].
- Instance tags in IMDS must be enabled and update live; instance tag keys are then limited to letters, digits and `+ - = . , _ : @` (no `/`). The IMDSv2 PUT hop limit defaults to 1; AWS recommends 2 for containers, and `ImdsSupport: v2.0` AMIs default to 2 [18]. AWS documents `iptables --insert DOCKER-USER 1 --in-interface docker+ --destination 169.254.169.254/32 --jump DROP` to keep bridge-mode containers off IMDS; it doesn't cover host networking [19].
- Security groups are stateful and don't filter traffic to Amazon DNS, DHCP, instance metadata or the Time Sync Service [20]. `ec2:CreateTags` and `ec2:DeleteTags` honour `aws:RequestTag/<key>` and `aws:TagKeys` [21]. `docker run --mount type=bind` fails if the source is missing, while `-v` creates it; dockerd's default `shutdown-timeout` is 15 s [22].
- Warm pools can't hold Spot in mixed-instance groups [23]. Lightsail bills stopped instances (8 GB plan $44/month; re:Post); EKS is $0.10 per cluster-hour [24].

## Options considered

All options are on-demand in us-east-1, and any of them can serve UDP from a public IP without a load balancer. Idle figures include the $0.50 zone.

### A. Simplest viable option: one shared EC2 instance, root volume only, containers started by hand

One `m6a.large` holds every game; a 30 GiB root volume holds the OS, images and worlds; the owner writes a `docker run` unit per game on the box and types the ports into CDK separately. Idle about $2.90/month.
Rejected (narrowly), because three gaps each have a cheap fix (see B): routine CDK changes (AMI, launch template, root volume size) replace the instance and bring it up on an empty disk [14][15]; each game's ports are maintained by hand in three places (security group, container, control plane); there is no undo for a bad mod install.

### B. A plus a retained data volume, a game catalog and snapshots (recommended)

Same instance and lifecycle, with worlds and the image cache on a retained data volume, game definitions in one TypeScript catalog that feeds the security group, the instance and the control plane, and DLM snapshots. Idle about $3.24–3.74/month. Details under Decision.

### C. One EC2 instance (and volumes) per game server

Roughly $1.74–1.94/month per server (8 GiB root, 10 GiB data, snapshots), so three servers plus the zone cost about $5.72 or more: **the $5 ceiling breaks at the third server**. It also enables concurrent games nobody needs. Rejected.

### D. One shared instance with one EBS volume per game, attached at start

Isolates worlds, but every volume carries its own unused headroom (EBS bills provisioned GiB), every start needs attach/detach steps and IAM, and all volumes stay in one AZ. Rejected: more parts and more idle cost for isolation nobody asked for.

### E. ECS on the shared EC2 instance

Typed task definitions would replace the catalog, but at the cost of an agent, a cluster, `iam:PassRole` and a two-phase lifecycle (start instance, wait for the agent, run task; stop task, stop instance). With one container at a time, EC2 start/stop plus systemd is the whole orchestrator. Rejected on KISS.

### F. ECS on Fargate (on-demand) with EFS (runner-up)

No OS to patch; `desiredCount` 0/1 per game; worlds on EFS. **The deciding factor is hands-on admin, not cost:** files can only be edited through ECS Exec into a running task, nothing is editable while the server is stopped, and there is no persistent host. On cost it is close: with EFS One Zone it is cheaper in total below roughly 40–55 active hours/month (before EFS I/O and backups), and above that the lower EC2 hourly rate wins ($0.0914 vs $0.1215) [8]. ECS-managed EBS volumes can't carry a world between tasks [9]. Rejected for v0; see "Revisit if".

### G. EC2 Auto Scaling group with desired capacity 0/1

Scale-in terminates [1], so state must leave the instance or sit in a warm pool with reuse [23], which is A plus an ASG, a warm pool and lifecycle hooks. Self-healing and Spot diversification don't apply to one on-demand instance. Rejected as over-engineering.

### H. Amazon GameLift Servers (managed EC2 or containers)

Native scale to and from zero [10], but about 28% more per hour, built around builds, fleets and match-style sessions, persistence left to S3, and immutable builds that conflict with hands-on mods. Anywhere supplies no compute. Rejected.

### I. Rejected on fixed cost or fit

NLB ($16.43/month), NAT gateway ($32.85/month), Elastic IP ($3.65/month) [2], EKS ($73/month) and Lightsail (billed while stopped) [24] all break "zero cost at zero players". Spot is excluded by the owner. Lambda and App Runner can't host a long-running UDP server (not re-verified in this session).

## Decision

**Option B: one shared on-demand `m6a.large`, stopped at rest, running one game container at a time under Docker and systemd; worlds on one retained gp3 data volume; game definitions from a TypeScript catalog published to SSM Parameter Store.**

### Compute, storage and network

- **Instance:** one `m6a.large` (x86_64, because the shared instance must suit every game's image), one AZ, `instanceInitiatedShutdownBehavior: STOP`, `disableApiTermination: true` (the rebuild runbook turns it off first). Instance-type changes are a manual owner action (stop, modify).
- **Root volume:** OS only, gp3, AMI default size, `encrypted: true` set on the first deploy (changing it later replaces the instance [14]).
- **Data volume:** one `AWS::EC2::Volume`, gp3, 20 GiB to start, `DeletionPolicy`/`UpdateReplacePolicy: Retain`, logical ID pinned with `overrideLogicalId`. It is mounted at `/srv` by label with `nofail` and formatted only if blank. Layout: `/srv/games/<id>/` (world), `/srv/secrets/` (root, 0700), `/srv/docker/` (Docker data-root). It grows in place [14]. Root, data volume and snapshots use the AWS managed `aws/ebs` key [5].
- **Attachment:** `CfnVolumeAttachment`, not `CfnInstance.volumes`. Every attachment property forces replacement and replacement creates the new attachment first, so replacing either the instance or the volume fails while the old attachment still holds the volume, and the deploy rolls back (expected; tested in the first infra PR). `Volumes` on the instance updates in place [14], which would swap a new empty volume onto the live instance: it fails open. Termination protection is a second stop: CloudFormation can't delete the old instance.
- **Backups:** one DLM policy on the data volume: daily at 09:00 UTC keeping 7, and weekly on Mondays at 09:00 UTC keeping 4 (coinciding runs make one snapshot [4]). Snapshots taken while a game runs are only crash-consistent.
- **Network:** a dedicated VPC (`natGateways: 0`, public subnets only, one AZ) with an auto-assigned public IPv4; no NAT, load balancer, Elastic IP or VPC endpoints. Security group ingress is each catalog port with `exposure: 'public'`, from `0.0.0.0/0`; no SSH. Egress is TCP 443 plus each game's `egressPorts`; a game that needs port 80 declares it there. SSM [12], the AL2023 repositories [16] and container registries (HTTPS; confirmed per registry at onboarding) use 443; Amazon DNS, IMDS and Time Sync aren't filtered, and replies to player traffic pass statefully [20].
- **DNS:** CDK creates the public zone `games.bradfordly.com` (retained, logical ID pinned), outputs its NS records for the manual delegation, and publishes CAA `0 issue ";"` and `0 issuewild ";"` because no certificates are issued for it. Only the control plane writes the per-game A records `<id>.games.bradfordly.com` (TTL 60).
- **One-time account settings** (us-east-1, outside the stack): EBS encryption by default, snapshot Block Public Access (`block-all-sharing`) [5], and an AWS Budgets cost alert, which is free [13].

### Game catalog

A TypeScript list in `infra/`, validated at synth and published as one standard SSM parameter per game at `/bradfordly-games/games/<id>` (free [12]). Its three consumers are the security group, the launcher and the control plane. Value shape (this is the schema the control plane receives):

```
id                  ^[a-z0-9][a-z0-9-]{0,31}$  (asserted at synth; also the DNS label)
image               repo:tag@sha256:<digest>  (digest pin asserted at synth)
ports[]             { port, protocol: udp|tcp, exposure: public|admin }
query?              { port, protocol }        (must be one of the public ports)
egressPorts[]?      { port, protocol }
env                 { KEY: non-secret value }
dataMountPath       absolute container path for /srv/games/<id>
stopTimeoutSeconds  1-120, default 60
capAdd[]?           subset of a fixed capability allowlist in CDK
requiresSecrets     boolean
```

### Server-plane invariants

These are the security and data-safety rules. Everything else is implementation detail for the first infra PR.

1. **Boot sequence (every boot, cloud-init `scripts-user` set to `always` [23]):** mount `/srv`; run `timeout 10m dnf -y upgrade --releasever=latest`, which is non-fatal (log and continue) and runs only with `gpgcheck=1` [16]; if `dnf needs-restarting -r` reports a reboot and the newest installed kernel differs from the one recorded in a marker file, record it and reboot once; start Docker; insert the IMDS drop rule; run the launcher.
2. **Fail closed to admin mode** (host up and reachable over SSM, no game) if any of these is true: `/srv` is not a mountpoint (`mountpoint -q`); the `game` tag is absent or fails `^[a-z0-9][a-z0-9-]{0,31}$`; `GetParameter` for that id fails; `/srv/games/<id>` doesn't already exist (the launcher never creates it); `requiresSecrets` is set and `/srv/secrets/<id>.env` isn't root:root 0600 inside a root-owned 0700 directory; the IMDS drop rule can't be inserted. `docker.service` and `game@.service` both carry `RequiresMountsFor=/srv`.
3. **Launcher hygiene:** bash with `set -euo pipefail` and no `set -x`; the tag is validated before any use; catalog JSON is parsed with `jq`; docker arguments are built as a bash array; no `eval`, no `sh -c`.
4. **Container template** (below). `game@.service` runs it in the foreground, ordered after `docker.service`, so it stops before dockerd and the 15 s daemon timeout [22] never applies. Never `--privileged`, `--network host`, `--pid host`, extra mounts, devices, or the Docker socket. `--memory` is host RAM minus 1 GiB. Admin ports are reached through Session Manager port forwarding.
5. **IMDS, two layers:** hop limit 1 set through the CDK props `httpTokens: REQUIRED`, `httpPutResponseHopLimit: 1`, `instanceMetadataTags: true` (never `requireImdsv2`, whose launch template would replace the instance when changed [14][15]), plus the `DOCKER-USER` drop rule [19]. Host networking defeats both and is forbidden. First-deploy test: an IMDSv2 token PUT from inside a container must time out.
6. **Graceful stop:** OS shutdown stops `game@<id>`, which runs `docker stop --time=<stopTimeoutSeconds>` (SIGTERM or the image's `STOPSIGNAL`, then SIGKILL) under `TimeoutStopSec` = timeout + 30 s. The 120 s cap keeps this within EC2's "few minutes" [1]. The server plane guarantees the signal and the grace period, not a save: saving is checked per game at onboarding. A pre-stop command (for example an RCON `save`) is out of scope until a game needs one.
7. **Backstop timers** (instance-initiated shutdown is `stop`, so no IAM is needed): power off when admin mode has lasted 4 h with no game unit active, and after 12 h of uptime in any mode. These bound unrequested runs, forgotten admin sessions and control-plane failures.
8. **CDK rules:** the AMI is pinned (`cachedInContext`, committed in `cdk.context.json`) [15]; no CloudFormationInit (`init`/`initOptions`), which makes user-data changes replace the instance [15]; user-data changes still restart the instance [14], which is why the catalog lives in SSM and the deploy runbook ends with "stop the instance".

```
docker run --rm --name game-<id> --network bridge \
  --security-opt no-new-privileges --cap-drop ALL [--cap-add <allowlisted> ...] \
  --memory <host RAM - 1 GiB> --pids-limit 2048 --log-driver journald \
  --mount type=bind,src=/srv/games/<id>,dst=<dataMountPath> \
  [--env-file /srv/secrets/<id>.env] [-e KEY=value ...] \
  -p <port>:<port>/<proto> ...            # exposure: public
  -p 127.0.0.1:<port>:<port>/<proto> ...  # exposure: admin
  <repo:tag@sha256:digest>
```

### Instance IAM

The instance role has the minimal Session Manager policy (`ssm:UpdateInstanceInformation`; `ssmmessages:CreateControlChannel`, `CreateDataChannel`, `OpenControlChannel`, `OpenDataChannel`; all on `*`) [12] plus `ssm:GetParameter` on `arn:aws:ssm:us-east-1:<account>:parameter/bradfordly-games/games/*`. It does not use `AmazonSSMManagedInstanceCore`, which would allow reading every parameter in the account. Nothing else: no EC2, Route 53, S3 or CloudWatch Logs permissions. Run Command and other Systems Manager features are therefore unavailable on this instance, by design.

### Contract with ADR-0003

The server plane provides the items below; ADR-0003 designs the control plane around them.

- **Configuration** (environment variables at deploy time): instance ID and ARN, hosted zone ID, catalog parameter path, the per-game record names `<id>.games.bradfordly.com`, and the catalog value shape above as the schema.
- **Single-flight lifecycle.** Start(id): when `stopped`, call `CreateTags game=<id>` then `StartInstances`. When `pending`/`running` with the same game, return the current state. When `stopping`, or running a different game or admin mode, reject (or retry later). If `StartInstances` fails, call `DeleteTags game`. `running` is not "ready": boot, patching, a possible reboot and game load take minutes (not yet measured).
- **Stop:** delete the game's A record, then `DeleteTags game`, then `StopInstances` (default). Never force-stop, skip OS shutdown, hibernate or terminate.
- **Endpoint:** the public IPv4 from `DescribeInstances` once running, plus ports from the catalog. The A record exists only while the instance runs with that IP. Requirement: an EventBridge rule on EC2 `stopping`/`stopped` for this instance deletes the record and the `game` tag, covering stops that bypass the control plane (backstop timers, console). The handler takes the same single-flight lock as Start/Stop and re-checks that the instance is still `stopping`/`stopped` before acting, so a late event can't strip the tag from a new Start.
- **Unrequested runs happen.** CloudFormation restarts the instance on user-data changes [14], and the owner starts admin sessions; the backstops end them within 4 h or 12 h. Any running instance is billable.
- **Signals:** no on-host player-count signal in v0. The catalog's optional `query` port and EC2 basic metrics are available; an on-host signal would amend this contract.
- **Grants (made by ADR-0003):**
  - `ec2:StartInstances` and `ec2:StopInstances` on the instance ARN; `ec2:DescribeInstances`.
  - `ec2:CreateTags` on the instance ARN with `StringEquals aws:RequestTag/game` set to the catalog ids (generated by CDK), `ForAllValues:StringEquals aws:TagKeys ["game"]` and `Null aws:TagKeys "false"`. `ec2:DeleteTags` with the same two `aws:TagKeys` conditions [21].
  - `ssm:GetParameter` and `ssm:GetParametersByPath` on the catalog path.
  - `route53:ChangeResourceRecordSets` on the zone ARN with `ForAllValues:StringEquals` on `…NormalizedRecordNames` (the game record names), `…RecordTypes` `["A"]` and `…Actions` `["UPSERT","DELETE"]` [11].
- **Never granted to the control plane:** `ec2:ModifyInstanceAttribute`, `ec2:ModifyInstanceMetadataOptions`, `ec2:RunInstances`, `ec2:TerminateInstances`, `ec2:DetachVolume`, `ec2:DeleteVolume`, `ec2:DeleteSnapshot`, `ec2:AssociateIamInstanceProfile`/`ReplaceIamInstanceProfileAssociation`, `iam:PassRole`, `ssm:SendCommand`, `ssm:StartSession`. Instance-type changes stay a manual owner action.

### Cost

us-east-1, 730 hours/month, prices from the facts above.

| Item | Chosen design (B) | Runner-up (F: Fargate + EFS) |
|---|---|---|
| **Idle, per month** | | |
| Compute | $0 (stopped) | $0 (`desiredCount` 0) |
| Root gp3 8 GiB (AMI default size, unverified) | $0.64 | n/a |
| World storage | data gp3 20 GiB: $1.60 | EFS 10 GB: $3.00 Standard, $1.60 One Zone |
| Backups | DLM daily ×7 + weekly ×4: **$0.50 (≈10 GB used) up to $1.00 (whole volume referenced), plus $0.05 per GB of retained churn** (estimate) | AWS Backup: **not priced (unverified)** |
| Public IPv4 | $0 (released on stop) | $0 |
| Route 53 zone (CDK-created) | $0.50 | $0.50 |
| SSM, Session Manager, VPC, SG, IAM, `aws/ebs`, Budgets alert | $0 | $0 |
| **Idle total** | **about $3.24–3.74 + churn** | **about $2.10–3.50 + backups** |
| **Active, per running hour** | | |
| Compute | m6a.large $0.0864 | 2 vCPU / 8 GB x86 $0.1165 |
| Public IPv4 | $0.005 | $0.005 |
| Storage I/O | included in gp3 | EFS $0.03/GB read, $0.06/GB write: **volume unknown** |
| Data transfer out | 100 GB/month free; then about $0.09/GB (unverified) | same |
| **Active total** | **about $0.091/hour**: 40 h ≈ $3.66, 100 h ≈ $9.14 | **≥ $0.1215/hour**: 40 h ≥ $4.86 |

Budget arithmetic: each data-volume GiB costs up to $0.13/month (storage plus a fully referenced snapshot) on top of $1.14 fixed (root plus zone). At 20 GiB the worst case leaves about $1.26–1.76 under $5 for ADR-0003, minus snapshot churn. Staying at or below about 18 GiB guarantees $1.50 of headroom, and just under 30 GiB breaks $5 on its own.

## Consequences

Positive:
- Idle about $3.24–3.74/month with no fixed-charge traps (NAT, NLB, Elastic IP, EKS), and the cheapest on-demand active rate considered.
- Few moving parts: one instance, one volume, one systemd unit. The control plane only tags, starts, reads the IP, writes DNS and stops.
- Hands-on admin through Session Manager (shell, `docker exec`, port forwarding to admin ports), plus an admin mode for edits while no game runs.
- Data fails safe: worlds live on a retained volume, replacement attempts fail the deploy, termination protection blocks deletion, and snapshots give daily and weekly undo.
- Defence in depth for third-party images: no capabilities by default, no new privileges, memory and PID limits, no IMDS, a minimal instance role, restricted egress, digest-pinned images and validated tag input.
- Server-side backstops bound runaway cost even if the control plane fails.

Negative:
- A pet-ish host patched in place. Boot-time upgrades and a possible reboot lengthen cold starts, and because `dnf` failures are tolerated, a broken mirror silently delays patches until someone notices.
- One AZ and one instance type: an impaired AZ or a capacity shortage blocks starts until the type changes or the volume moves via a snapshot.
- All games share one size and architecture (x86_64).
- Baseline hardening can break images that need extra capabilities, setuid binaries, host networking or many threads. Onboarding catches these: extend the allowlist with justification, or reject the image.
- Restricted egress breaks games or mods that need undeclared ports until `egressPorts` is updated.
- Saving on stop is checked at onboarding, not enforced, and the 12 h cap can end a long session.
- Idle headroom for ADR-0003 is about $1.26–1.76 and shrinks as the data volume or snapshot churn grows.

Revisit if:
- Two games must run at once: add a second instance, or upsize and run two containers.
- Idle nears $5: archive inactive worlds to S3 ($0.023/GB-month [25]), move to a smaller volume, or trim retention.
- A game needs host networking, arm64, a pre-stop save command, more than about 7 GiB of container memory (move to `r6a.large`, $0.113/hour [6]), or a capability outside the allowlist.
- Measured CPU stays under the 30% baseline: try `t3a.large` with `creditSpecification: STANDARD` to rule out surplus charges [7].
- Third-party images warrant stronger isolation: adopt userns-remap or rootless Docker.
- Active hours become high and steady, Spot is reconsidered, or hands-on admin is no longer needed: re-evaluate F.
- ADR-0003 needs an on-host player-count or readiness signal: amend the contract.
- AL2023 nears end of support (2029-06-30).

## Follow-ups (not part of this ADR)

1. First infra PR: build the stack as decided here. First-deploy tests: the AMI root size; an attempted instance replacement rolls back; the admin-mode boot; an IMDSv2 token PUT from a container times out; `dnf`, SSM and registry pulls work under the restricted egress; the first-boot journal shows no secret values.
2. CI: #9 (cdk-nag `AwsSolutionsChecks`) and #10 (synth plus IaC scan). Expected justified suppressions: AwsSolutions-EC23 (public game ports only), EC28 (no detailed monitoring), VPC7 (no flow logs), IAM5 (`*` resources of the Session Manager actions, via `appliesTo`), and Checkov CKV_AWS_88 (public IP). Add a `trivy image` job over the catalog digests.
3. Per-game onboarding checklist:
   - amd64 image pinned by digest;
   - in-image auto-update disabled, or the game version pinned via env (SteamCMD-style images);
   - bridge networking, with capabilities from the allowlist;
   - autosave on, and the SIGTERM save test passed (change the world, stop, start, check);
   - `/srv/games/<id>` created, and the secrets file (if required) has the correct ownership and mode;
   - mod checksums recorded.
4. Runbooks:
   - Rebuild: disable termination protection, stop, detach the data volume, deploy, stop the instance.
   - Routine deploy: deploy, then stop the instance.
   - Restore a snapshot.
   - Grow the data volume.
5. Measure cold start and CPU in the first sessions.
6. Owner actions: the one-time account settings above, and the NS delegation at the parent domain.

## Sources

1. EC2 stop/start — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/how-ec2-instance-stop-start-works.html ; https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Stop_Start.html
2. Amazon VPC pricing — https://aws.amazon.com/vpc/pricing/ ; Elastic Load Balancing pricing — https://aws.amazon.com/elasticloadbalancing/pricing/
3. Amazon EBS pricing — https://aws.amazon.com/ebs/pricing/
4. DLM at no charge — https://aws.amazon.com/blogs/aws/new-lifecycle-management-for-amazon-ebs-snapshots/ ; DLM schedules — https://docs.aws.amazon.com/ebs/latest/userguide/dlm-elements.html
5. Snapshot Block Public Access — https://docs.aws.amazon.com/ebs/latest/userguide/block-public-access-snapshots.html ; EBS encryption by default — https://docs.aws.amazon.com/ebs/latest/userguide/encryption-by-default.html ; AWS KMS pricing — https://aws.amazon.com/kms/pricing/
6. Vantage EC2 pages (third-party, 2026-10-12) — https://instances.vantage.sh/aws/ec2/m6a.large , https://instances.vantage.sh/aws/ec2/r6a.large , https://instances.vantage.sh/aws/ec2/t3a.large
7. Burstable baselines — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/burstable-credits-baseline-concepts.html ; EC2 On-Demand pricing — https://aws.amazon.com/ec2/pricing/on-demand/
8. Price List API, us-east-1 (2026-09-11): AmazonECS — https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonECS/current/us-east-1/index.json ; AmazonEFS — https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEFS/current/us-east-1/index.json
9. EBS volumes with ECS — https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ebs-volumes.html
10. Price List API AmazonGameLift (2026-09-23) — https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonGameLift/current/us-east-1/index.json ; scale to/from zero — https://docs.aws.amazon.com/gameliftservers/latest/developerguide/fleets_scale-to-from-zero.html
11. Route 53 pricing — https://aws.amazon.com/route53/pricing/ ; Route 53 IAM conditions — https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/specifying-rrset-conditions.html
12. Systems Manager pricing — https://aws.amazon.com/systems-manager/pricing/ ; minimal Session Manager role — https://docs.aws.amazon.com/systems-manager/latest/userguide/getting-started-create-iam-instance-profile.html ; AmazonSSMManagedInstanceCore — https://docs.aws.amazon.com/aws-managed-policy/latest/reference/AmazonSSMManagedInstanceCore.html ; SSM Agent endpoints — https://docs.aws.amazon.com/systems-manager/latest/userguide/troubleshooting-ssm-agent.html
13. AWS Budgets pricing — https://aws.amazon.com/aws-cost-management/aws-budgets/pricing/
14. CloudFormation — https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ec2-instance.html ; https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/using-cfn-updating-stacks-update-behaviors.html ; https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ec2-volumeattachment.html ; https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ec2-volume.html
15. CDK `MachineImage` — https://docs.aws.amazon.com/cdk/api/v2/docs/aws-cdk-lib.aws_ec2.MachineImage.html ; aws-cdk-lib 2.273.0 `aws-ec2/lib/instance.d.ts` and `instance.js` (installed in `infra/node_modules`)
16. AL2023 — https://docs.aws.amazon.com/linux/al2023/ug/deterministic-upgrades.html ; https://docs.aws.amazon.com/linux/al2023/ug/managing-repos-os-updates.html ; https://docs.aws.amazon.com/linux/al2023/ug/repo-metadata-signing.html ; https://docs.aws.amazon.com/linux/al2023/release-notes/support-information.html
17. AL2023 components — https://docs.aws.amazon.com/linux/al2023/ug/install-docker.html ; https://docs.aws.amazon.com/systems-manager/latest/userguide/ami-preinstalled-agent.html ; https://docs.aws.amazon.com/linux/al2023/ug/awscli2.html
18. IMDS — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/work-with-tags-in-IMDS.html ; https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-metadata-v2-how-it-works.html ; https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-IMDS-new-instances.html
19. Blocking IMDS for bridge-mode containers — https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html
20. VPC security groups — https://docs.aws.amazon.com/vpc/latest/userguide/vpc-security-groups.html
21. EC2 tagging conditions — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/supported-iam-actions-tagging.html
22. Docker bind mounts — https://docs.docker.com/engine/storage/bind-mounts/ ; dockerd reference — https://docs.docker.com/reference/cli/dockerd/
23. EC2 Auto Scaling warm pools (also the per-boot user-data tip) — https://docs.aws.amazon.com/autoscaling/ec2/userguide/ec2-auto-scaling-warm-pools.html
24. Lightsail pricing — https://aws.amazon.com/lightsail/pricing/ ; billed while stopped (re:Post) — https://repost.aws/questions/QUXTceGm-iTe6ewBHIN6NrBQ/why-am-i-being-charged-for-a-stopped-amazon-lightsail-instance ; EKS pricing — https://aws.amazon.com/eks/pricing/
25. Price List API AmazonS3 (2026-09-28; S3 Standard $0.023/GB-month) — https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonS3/current/us-east-1/index.json
