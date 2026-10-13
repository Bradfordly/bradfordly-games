# 0003. Control plane: server lifecycle API and idle shutdown

- Status: Proposed
- Date: 2026-10-12

## Context

Issue #16. ADR-0002 (merged, Proposed) defines the server plane:

- one stopped-at-rest `m6a.large` running one game container at a time, selected by the instance's `game` tag;
- a launcher that fails closed to admin mode;
- host backstops (4 h admin mode, 12 h uptime);
- a "Contract with ADR-0003" (single-flight lifecycle, DNS, Grants, "Never granted") that this ADR honours.

ADR-0001 fixes Go 1.27 on Lambda (`provided.al2023`, arm64) for code we write.

This ADR decides the control-plane core:

- the lifecycle state machine and API;
- the lock and state store;
- how a start is followed to "ready";
- idle shutdown and cost guards;
- a host agent that reports readiness and activity;
- the domain events and Discord notifications.

It defines the interfaces that three later ADRs plug into, and decides none of their content:

- **0004 (#17) Game packages and per-server configuration:** package manifest, runtime servers, forms schema, secrets, world provisioning, the Hytale package, and the catalog and Grants amendments.
- **0005 (#18) Wake on connect:** DNS query-log trigger, placeholder records, abuse limits, and the DNS contract amendment.
- **0006 (#19) Web frontend and authentication:** hosting, Cognito, sessions, domain and CAA.

Owner decisions (authoritative):

- (2026-10-12) Total idle cost stays under $5/month. ADR-0002 leaves about $1.26–1.76/month for the control plane and everything after it. Long cold starts are acceptable.
- (2026-10-12) Worlds are created in the UI from existing packages. New game types come only by reviewed PR (0004).
- (2026-10-12) Login is invite-only Cognito. Group `admin` creates servers and edits config and secrets. Every other invited user (a "member") views status and starts or stops servers. 0006 decides how sessions carry the group.
- (2026-10-12) Wake on connect is a Route 53 query-log trigger (0005). Messages say "lookup detected", never "login attempted".
- (2026-10-12) A Go host agent provides readiness and idle detection:
  - It watches the game container's journal for the package's ready pattern.
  - It counts conntrack flows on the package's public ports.
  - It powers the host off locally when idle.
  - It sends events with `events:PutEvents`, scoped to one custom bus by `events:source` and `events:detail-type` conditions plus Null checks.
- (2026-10-12) Idle defaults: stop 15 min after the last player leaves, or 20 min after "ready" if nobody joins. Keep the 4 h and 12 h host backstops. Per-server overrides may come later (0004).
- (2026-10-12) Discord gets an incoming webhook only. Messages: starting, lookup detected, ready, stopped (idle), stopped, auth required, start failed. Slash commands must be addable later without reworking the events.
- (2026-10-13) **O1:** the control plane mirrors both host backstops from AWS. It stops a running instance more than 12 h 15 min after its run began, or more than 4 h 15 min after in `ADMIN`.
- (2026-10-13) **O2:** a cap of 80 run-hours per month. Above it, Start is refused unless an admin overrides.
- (2026-10-13) **O3:** an admin hold on the host, `game-agent hold [--minutes N]` (default 60), released early with `game-agent release`. It pauses the agent's power-off rules and stays inside the backstops.
- (2026-10-13, revised the same day) **O4:** a flood guard.
  - When a CloudWatch alarm on the host-event rate fires, the control plane immediately attaches the Deny-only policy `DenyHostPutEvents` to the instance role, stops the host, and the owner is emailed.
  - There is no Budgets action. ADR-0002's free budget alert remains the slow cost signal.

Verified facts (2026-10-12/13, us-east-1 unless noted; "(unverified)" marks the rest):

- **PutEvents authorization:**
  - `events:PutEvents` accepts `events:source` and `events:detail-type` (type ArrayOfString) on event-bus resources [1].
  - Authorization is per entry: denied entries fail with `NotAuthorizedForSourceException` or `NotAuthorizedForDetailTypeException` in their result entry [2].
  - So `ForAllValues:StringEquals` plus a `Null` check is the correct form; without the `Null` check, `ForAllValues` passes when the key is absent [3].
  - An explicit Deny, including one in a bus resource policy, overrides a same-account identity Allow [23].
- **EventBridge pricing and delivery:**
  - Classic custom-bus events cost $1.00 per million, billed per 64 KB chunk [4].
  - "AWS management events" are free to ingest [4]. That EC2 state-change events are also free is assumed (unverified).
  - Scheduler includes 14 million invocations per month free [4].
  - Rule targets are retried for 24 h and up to 185 times only when the invoke itself fails [5].
  - Creating or updating a Scheduler schedule requires `iam:PassRole` [6].
  - EventBridge publishes `AWS/Events` metrics every minute on a best-effort basis. `MatchedEvents` with `EventBusName` and `RuleName` counts the events one rule matched on a custom bus [7].
- **Step Functions** Standard costs $0.000025 per transition, with 4,000 free per month [8].
- **DynamoDB** on-demand costs $0.625 per million writes, $0.125 per million reads and $0.25/GB-month, with 25 GB of storage free (always-free status not stated) [9].
- **Lambda:**
  - 1 million requests and 400,000 GB-s per month are free (always-free status not stated). arm64 costs $0.0000133334/GB-s and $0.20 per million requests [10].
  - For an event Lambda has accepted asynchronously, its own settings apply: function errors are retried twice, throttles are retried for up to 6 h by default, and duplicates can occur [11].
  - Reserved concurrency can be at most the unreserved account concurrency minus 100 [12].
- **CloudWatch:**
  - 5 GB of logs per month and 10 alarm metrics are free; standard alarms cost $0.10 per alarm metric per month [13].
  - EC2 stop actions work only on alarms that watch an EC2 per-instance metric, or a custom metric with an `InstanceId` dimension [14].
  - Metric alarms can notify SNS, and an alarm invokes its actions only when it changes state [15].
  - `cloudwatch:DescribeAlarms` can be scoped to one alarm ARN [16].
  - EC2 `NetworkPacketsIn` exists only at 5-minute resolution and counts all interfaces [17].
- **SSM and KMS:**
  - Parameter Store standard parameters are free [18].
  - Standard SecureStrings are encrypted under `aws/ssm` by default. Decrypting "needs kms:Decrypt permission", but "you cannot establish access control policies for the default aws/ssm KMS key", and `kms:EncryptionContext:PARAMETER_ARN` is available as a condition [19]. So whether a role needs an IAM `kms:Decrypt` statement for `aws/ssm` is unclear (unverified).
  - CloudFormation can't create SecureStrings [20].
  - KMS requests are billed beyond 20,000 free per month [21]. Secrets Manager costs $0.40 per secret-month [22].
- **IAM and SNS:**
  - `iam:AttachRolePolicy` supports the `iam:PolicyARN` condition key [24].
  - Instance-profile credentials have no published fixed lifetime. They rotate automatically, with new credentials available at least 5 minutes before the old ones expire [25].
  - IAM "Revoke active sessions" works by attaching `AWSRevokeOlderSessions` [26].
  - An SNS topic's default policy lets principals in the account publish [27].
- **EC2:**
  - `StopInstances` documents no behaviour for a `pending` instance. AWS documents a CLI force stop as possible in `pending` [28]. So whether a normal stop of a `pending` instance fails with `IncorrectInstanceState` is unverified.
  - `LaunchTime` is the time of the most recent start (not re-verified).
- **Linux:**
  - `nf_conntrack_acct` (default 0) adds per-flow packet and byte counters. Default timeouts are 432,000 s for an established TCP flow, and 30 s or 120 s for UDP (single packet or stream) [29].
  - `CAP_NET_ADMIN` is enough to read conntrack over nfnetlink [30]. That AL2023 provides `nf_conntrack_netlink` is a first-deploy test.
  - `systemctl poweroff` as root works through logind without `CAP_SYS_BOOT` (security review; first-deploy test).
- **Discord:**
  - A webhook is executed at `/webhooks/{id}/{token}` with no other authentication.
  - Its `flags` may set `SUPPRESS_EMBEDS`, and `allowed_mentions` controls mentions [31]. (`parse: []` suppresses mentions but not masked links.)
  - Rate limits are per route. 10,000 invalid requests in 10 minutes trigger a temporary ban, and a webhook that returns 404 should not be used again [32].
  - Interactions endpoints must verify Ed25519 signatures and answer within 3 s [33].
- **Logs:** Docker's journald driver tags each message with `CONTAINER_NAME` [34].
- **Hytale and prior art:**
  - Hytale documents no query protocol and rewrites its config while running [35].
  - Pelican's community egg treats `Hytale Server Booted!` as readiness [36] (not checked against real output).
  - Prior art for idle stop on the game host: itzg's AUTOSTOP and doctorray117's watchdog [37][38].
- **Route 53:** `…NormalizedRecordNames` accepts `ForAllValues:StringLike` [39] (for 0004).
- **This repository** is public on GitHub [40].

## Options considered

All options use Lambda, DynamoDB and EventBridge in the same way. They differ in how a start is followed to "ready" and how failures are detected.

### A. Simplest viable option: purely event-driven, no timers in AWS

The control plane reacts only to API calls, EC2 state-change events and host events. One DynamoDB item holds the state and a lease lock, and every timer lives on the host. Idle cost is $0.

Rejected, because some failures can only be seen from AWS:

- a dead agent;
- an event lost after Lambda's retries [11];
- a crash between the state write and `StartInstances`;
- a stop that never completes.

In each case the item goes stale, members are blocked, and the instance bills until a host backstop fires. That assumes the host's own timers still work, which O1 does not.

### B. A plus one recurring reconcile (recommended)

Same as A, plus one CDK-created EventBridge Scheduler schedule that runs a reconcile every 5 minutes. The reconcile compares the item with `DescribeInstances`, the zone's A records and the last host event, then:

- repairs drift;
- enforces the control-plane timeouts;
- applies O1.

8,640 runs a month fit in Scheduler's free tier [4] and cost fractions of a cent (Cost, below).

### C. A Step Functions Standard execution per start

Rejected as over-engineering for one host:

- it duplicates state between the execution and DynamoDB;
- it needs task-token plumbing;
- it still can't see a dead agent after "ready", so the reconcile is needed anyway.

### D. Per-start EventBridge Scheduler timeouts

Rejected. `CreateSchedule` requires `iam:PassRole` [6], which is on ADR-0002's "Never granted" list. A schedule created by CDK has its role passed by CloudFormation at deploy time, not by the control plane.

### Other alternatives (rejected)

- **Readiness from Lambda protocol probes:** Hytale has no query protocol [35], and probes need per-game code.
- **`NetworkPacketsIn` alarms:** 5-minute resolution, and the count includes SSM, `dnf` and scanner traffic [17].
- **Host transports other than EventBridge:**
  - SQS has no fan-out to the notifier.
  - `PutMetricData` sends metrics, not events.
  - Direct DynamoDB writes would make the host a writer of authoritative state.
- **State in EC2 tags or SSM:** neither offers compare-and-swap, and the control plane may write only the `game` tag key.
- **Reserved concurrency of 1 as the lock:** synchronous API calls would throttle instead of queueing.
- **An EC2 stop action on the flood alarm:** not valid. The host-event metric has no `InstanceId` dimension [14].
- **An AWS Budgets action as the flood deny:** the owner rejected it (O4) as hours too slow.
- **A repeat email while the flood alarm stays in ALARM:**
  - It would need a second topic, because only CloudWatch may publish to the alert topic.
  - Once the deny is attached, the flood's entries fail authorization and the alarm returns to OK.
- **Restricting IMDS to uid 0 on the host with an iptables owner match:** the processes that use the role (agent, SSM Agent, cloud-init, dockerd) run as root, and containers are already blocked (ADR-0002 invariant 5). The gain is marginal.
- **Shipping the agent:**
  - ADR-0001's default, a Go container image from CDK assets, would need registry or S3 read on the instance. It would also need host conntrack, journal and power-off access, which ADR-0002 forbids for containers.
  - An S3 asset needs `s3:GetObject`, a second instance permission.
  - A custom AMI conflicts with ADR-0002's pinned stock AL2023.

## Decision

**Option B: an event-driven lifecycle with one DynamoDB item as state and lease lock, a host agent that reports and powers off on idle, one custom EventBridge bus, a 5-minute reconcile that also mirrors the host backstops, a monthly run-hours cap, a flood guard with an instant deny, and a separate Discord notifier.**

### Components

All Lambdas are Go, arm64, 128 MB with a 30 s timeout, built from one module (`backend/`).

| Component | Invoked by | Responsibility |
|---|---|---|
| `api` Lambda (`api` role) | Only 0006's front door (resource policy) | HTTP routes. Takes identity from the front door and denies by default |
| `events` Lambda (`events` role) | EventBridge rules (full envelopes), the Scheduler and the alert topic | EC2 state changes, commands, the reconcile, the flood guard |
| `hostevents` Lambda | The host-events rule only. Reserved concurrency 1 | Turns host events into state and domain events. **No EC2, Route 53 or IAM permissions** |
| `notifier` Lambda | The domain-events rule. Reserved concurrency 1 | Posts to the Discord webhook |
| `lifecycle` table | DynamoDB on-demand | One item per host (`host#<instanceId>`) |
| `bradfordly-games` bus | EventBridge, with a resource policy | Host events, commands, domain events |
| Reconcile schedule | Scheduler, `rate(5 minutes)` | Invokes `events` |
| Flood guard | CloudWatch alarm; SNS topic `bradfordly-games-alerts` with a locked topic policy; the customer-managed Deny-only policy `DenyHostPutEvents` | O4 |
| `game-agent` | systemd on the instance (linux/amd64) | Readiness, activity, power-off, hold |

`api` and `events` share the core package and the same lifecycle grant set, built once in CDK. Only the `events` role adds the two flood-guard statements, so the internet-facing `api` never holds an IAM permission. The event types are one shared package used by agent, `api`, `events`, `hostevents` and `notifier`, which meets ADR-0001's rule of three.

Configuration is passed as environment variables (12-factor):

- from ADR-0002's contract: instance ID and ARN, zone ID, catalog path, record names;
- added here: table, bus, topic, alarm, deny-policy and webhook-parameter names, `RUN_HOURS_CAP=80` and the timeouts.

**Deploy prerequisite: account concurrency.** The two reservations of 1 need an account concurrency quota of at least 102, because reservations can't take the account below 100 unreserved [12].

- The infra PR's preflight checks `aws lambda get-account-settings` for `AccountLimit.ConcurrentExecutions` ≥ 102.
- If it is lower, request an increase before the first deploy (Service Quotas "Concurrent executions", code L-B99A9384, unverified). New accounts can start with a reduced quota; the commonly quoted value of 10 is unverified.
- A CDK context flag may omit both reservations temporarily. The consequence: a host-event flood then competes for account concurrency and slows the flood guard, and Discord posts are no longer serial.

### State machine

| State | Meaning |
|---|---|
| `STOPPED` | Instance stopped; no `game` tag and no A record |
| `STARTING` | `game` tag set and `StartInstances` accepted; booting, patching or loading |
| `READY` | The game reported ready; the endpoint is published |
| `STOPPING` | A stop was issued or announced and the instance hasn't reached `stopped` |
| `ADMIN` | Instance running without a lifecycle start (console start, CloudFormation restart, admin session) |

**Attempts.** Every run has an `attemptAt`: the Start request time, or the instance's `LaunchTime` whenever a run is adopted (as `ADMIN`, or as `STARTING` by reconcile rule 2 or Start step 3). Entering `ADMIN` clears `serverId`, `hostname` and `ports`.

- EC2 `stopping` and `stopped` events earlier than `attemptAt` are dropped. They need no skew tolerance, because a real stop of this attempt comes minutes later.
- EC2 `pending` and `running` events get 5 s of tolerance.
- Host events whose `bootTime` is more than 60 s before `attemptAt` are dropped.

This stops a late `stopped` event from stripping a new start's tag, and stops the previous boot's host events from moving a new attempt.

| Trigger | From | To | Side effects |
|---|---|---|---|
| Start(id) | `STOPPED` (or no item), EC2 `stopped`, not `blocked` | `STARTING` | See "Start, below" |
| EC2 `running` | `STARTING` | `STARTING` | Record `launchTime` from `DescribeInstances`; UPSERT `<id>.games.bradfordly.com` (A, TTL 60) |
| EC2 `pending`/`running` without a `game` tag | `STOPPED` | `ADMIN` | Adopt: `attemptAt` = `launchTime` = `LaunchTime` |
| `Game Ready`, or a heartbeat with `ready: true` | `STARTING` | `READY` | `Server Ready` |
| `Host Admin Mode`, or a heartbeat with `mode: admin` | `STARTING` | `STOPPING` | `Server Start Failed` (`admin-mode`); `stopReason` `start-failed`. The reconcile issues the stop |
| `Host Shutting Down` | `STARTING`, `READY` | `STOPPING` | Record the reason. `ready-timeout` also emits `Server Start Failed` and sets `stopReason` `start-failed` |
| Stop | `STARTING`, `READY`, `STOPPING`, `ADMIN` | `STOPPING` | DELETE the A record, `DeleteTags game`, `StopInstances`. On an EC2 error the item stays `STOPPING`, the API returns 502, and the reconcile retries |
| EC2 `stopping` | any running state | `STOPPING` | DELETE the A record, `DeleteTags game` |
| EC2 `stopped`, instance still stopped | any but `STOPPED` | `STOPPED` | DELETE the A record, `DeleteTags game`, accrue run-hours, `Server Stopped` |
| EC2 `stopped`, instance already `pending`/`running` again (restarted outside the lifecycle, e.g. by CloudFormation) | any but `STOPPED` | `ADMIN` | As above, with reason `external`; adopt the new run |
| Flood alarm, confirmed `ALARM` | any | (Stop, if running) | See "Flood guard" |
| `Unblock Requested` (admin CLI) | any | unchanged | Clear `blocked` |

Only the `stopped` handler emits `Server Stopped`. Other rules emit `Server Start Failed` and record `stopReason`, which the `stopped` message then carries.

Where Stop comes from:

- `POST /servers/{id}/stop` and `Stop Requested`, for the active server;
- `POST /host/stop`, in any state;
- the reconcile;
- the flood guard.

### Item, lock and DNS

The item holds:

- the attempt: `serverId`, `hostname`, `ports`, `startId`, `attemptAt`, `launchTime`, `cause`, `requestedBy`. `hostname` and `ports` are copied from the server lookup at Start or adoption;
- status: `stateSince`, `readyAt`, `readyBasis`, `peers`, `idleDetection`, `hold`, `lastHostEventAt`, `lastHeartbeatAt`, `blocked`;
- outcome and usage: `stopReason`, `lastFailure`, `runMonth`, `runSeconds`;
- the lock: `lockOwner`, `lockExpiresAt`.

Every write is a conditional `UpdateItem` on named attributes. A missing item counts as `STOPPED` (`attribute_not_exists(#state)`), and the reconcile creates it.

**Lease lock** (shared by Start, Stop, the EC2 handlers, the flood guard's Stop and the reconcile, per ADR-0002):

- **Acquire:** a conditional `UpdateItem` with `ReturnValues=ALL_NEW` that sets `lockOwner` and `lockExpiresAt` = now + 60 s, on condition `attribute_not_exists(lockOwner) OR lockExpiresAt < :now`. The holder re-reads `DescribeInstances` and decides on the fresh item, never on its pre-lock read.
- **Release:** written together with the new state, on condition `lockOwner = :me`.
- **When the lock is busy:**

  | Caller | Result |
  |---|---|
  | API | 409 `busy` with `Retry-After` |
  | Command | `Request Rejected` (`busy`) |
  | EC2 event | Returns an error so Lambda retries; the reconcile repairs the rest |
  | Flood guard's Stop | Returns an error so Lambda retries; reconcile rule 9 backs it. The latch and the deny don't need the lock |
  | Reconcile | Skips this run |

**Host-event writes** (`hostevents`) take no lease.

- They use fixed update and condition expressions, with values only.
- Their condition is `#state IN (…) AND serverId = :id AND attemptAt = :a`. In `ADMIN`, where events carry no `serverId`, it is `#state = ADMIN AND attemptAt = :a`. Either way, the lock must also be free.
- On a failed condition the handler returns an error and Lambda retries.
- Heartbeats only update `peers`, `hold`, `idleDetection` and the timestamps, and are skipped when `lastHeartbeatAt` is less than 60 s old (a read, not a write).

**DNS.**

- The running handler and the reconcile UPSERT under the lock.
- Every DELETE first reads the live record with `ListResourceRecordSets`, then deletes exactly that value and TTL. A record that's already absent counts as done. This avoids treating Route 53's ambiguous `InvalidChangeBatch` as success.
- A host-event-driven stop changes only the item, because `hostevents` has no Route 53 rights. The record goes when the EC2 `stopping`/`stopped` handlers run, or when the reconcile issues the Stop.

### Start, run-hours cap and Stop

Start(id):

1. Validate the id through the server-lookup function (interim: the catalog), or return 404.
2. Acquire the lock. If `blocked` is set, return 409 `flood-guard`.
3. Check `DescribeInstances`:
   - `stopped`: continue.
   - `pending`/`running` without a tag: adopt as `ADMIN` and return 409 `admin-mode`.
   - `pending`/`running` with a valid tag: adopt as `STARTING` (as rule 2). Return 200 if the tag is this id, otherwise 409 `other-server-active`.
   - `stopping`: return 409 `stopping`.
4. Check the cap (O2):
   - If `runMonth` isn't the current UTC month, count `runSeconds` as 0.
   - At or above `RUN_HOURS_CAP` (80 h), refuse with 409 `budget-exhausted`. The exception is an admin override: `POST /servers/{id}/start?override=budget`, or a CLI command with `override: "budget"`. The override is per start and stores nothing.
5. Write `STARTING`, `attemptAt`, `startId`, `hostname` and `ports`. Clear `launchTime`, `readyAt` and `stopReason`.
6. `CreateTags game=<id>`, then `StartInstances`. If `StartInstances` fails, `DeleteTags game`, return to `STOPPED`, emit `Server Start Failed` (`ec2-error`) and return 502.
7. Release the lock and emit `Server Starting`.

**Run-hours accrual:**

- A run accrues from the item's own `launchTime` (recorded on EC2 `running`, at adoption, or by the reconcile when it first sees the instance running without one).
- It ends at the `stopped` event's time, or at the reconcile's observation time when the reconcile repairs a lost `stopped`.
- Nothing accrues if `launchTime` is missing or earlier than `attemptAt` (a start that died before `StartInstances`, or a stale value).
- Each run is clamped to 0–12 h 15 min.
- `runMonth` resets when the month changes, and a run counts in the month it stopped.
- Every billed run counts, including `ADMIN`.
- The cap is checked only at Start; the 12 h backstops bound a session that is already running.

**Stop:**

- A Stop while `STOPPING` re-issues `StopInstances` and returns 202.
- `StopInstances` on a `pending` instance may fail (unverified [28]). Either way the item stays `STOPPING` and reconcile rule 3 retries. The control plane never forces, hibernates or terminates.

### API

0006 chooses the front door. The `api` handler:

- accepts only that front door's payloads, carrying a verified `{username, groups}`;
- denies unknown routes (404) and requests with no identity (401) by default;
- enforces each route's level in code, matching group names exactly;
- never reads identity from anything else.

Anyone with `lambda:InvokeFunction` on `api` can claim any identity, so that permission is admin-equivalent. The resource policy grants it only to the front door. There are no request bodies and no idempotency keys: requests are idempotent by state.

| Route | Level | Behaviour |
|---|---|---|
| `GET /status` | member | Shows the item's fields: state, server, since, endpoint, approximate players, ready time, hold, blocked, last failure, run-hours this month and the cap |
| `GET /servers` | member | Known server ids (interim: the catalog) and which one is active |
| `POST /servers/{id}/start` | member | Start(id). `?override=budget` requires admin |
| `POST /servers/{id}/stop` | member | Stop the active server |
| `POST /host/stop` | admin | Stop in any running state, including `ADMIN` |
| Create, edit, config and secrets | admin | Defined by 0004 |

Responses:

- **202:** a transition started.
- **200:** already in the requested state (Start while `STARTING`/`READY` for the same server; Stop while `STOPPED`).
- **404:** unknown server.
- **409** with a reason: `other-server-active`, `admin-mode`, `stopping`, `busy`, `budget-exhausted` or `flood-guard`. `stopping` and `busy` include `Retry-After`.
- **502:** an EC2 error, with the error code.
- **401/403:** no identity, or the wrong group.

Until 0006 there is no front door. The owner uses `aws events put-events` (Commands, below) and reads status from Discord or the item.

### Commands

Asynchronous producers put commands on the bus. `Start Requested` and `Stop Requested` (detail `{v, serverId, override?}`) have the same semantics as the API.

- `cause.kind` comes from the envelope `source`, never from the detail:

  | Source | Cause kind | Level |
  |---|---|---|
  | `bradfordly-games.cli` | `cli` | Admin |
  | `bradfordly-games.wake` (0005) | `lookup` | Member; can't override |
  | `bradfordly-games.discord` (future) | `discord` | Member |

- `Unblock Requested` (detail `{v}`) is accepted only from `bradfordly-games.cli`.
- Outcomes come back as domain events: `Server Starting`, the eventual `Server Stopped`, or `Request Rejected` with the 409 reason.
- Adding a producer takes one rule-pattern entry, one bus-policy producer entry and its own source-scoped `PutEvents` grant.

### Reconcile (every 5 minutes)

The reconcile reads the item, `DescribeInstances` and the zone's A records. It takes the lock only when it must act, and re-evaluates on the returned item.

Rules 5, 6, 8 and 9 apply only while the instance is `pending` or `running`. Their clocks start at `t0`, the later of `LaunchTime` and `attemptAt`, so a stale `LaunchTime` can't fire them on a seconds-old start.

| # | Condition | Action |
|---|---|---|
| 0 | No item | Create it as `STOPPED`; rule 2 adopts a running instance in the same run |
| 1 | Item `STARTING`/`READY`/`STOPPING`/`ADMIN`, instance `stopped` | Run the `stopped` handler, ending the run at the observation time. For `STARTING`, only if `attemptAt` is more than 2 min old |
| 2 | Item `STOPPED`, instance `pending`/`running` | Adopt with `attemptAt` = `launchTime` = `LaunchTime`: as `ADMIN` if untagged, or as `STARTING` (cause `external`, `hostname`/`ports` from the lookup) with a valid tag. Any running item without `launchTime` gets it recorded |
| 3 | `STOPPING`, instance `pending`/`running` for more than 5 min since `stateSince` | Stop again |
| 4 | `STARTING`, instance not running 10 min after `attemptAt` | `Server Start Failed` (`ec2-timeout`); Stop |
| 5 | `STARTING`/`READY`, and no host event for 20 min since the later of `t0` and `lastHostEventAt` | Agent presumed lost. Stop; `Server Start Failed` (`agent-lost`) if `STARTING`; `stopReason` `agent-lost` |
| 6 | `STARTING` for longer than the package's ready timeout + 30 min after `t0` (default 60 min), unless the last heartbeat said `hold` | `Server Start Failed` (`start-timeout`); Stop |
| 7 | A game A record that isn't the active server's record at the instance's current public IP (while `STARTING`/`READY`), or a missing one | DELETE (read first) or UPSERT, so DNS matches the item |
| 8 | `t0` more than 12 h 15 min ago in any state, or more than 4 h 15 min ago in `ADMIN` (O1). Hold doesn't exempt | Stop, `stopReason` `backstop-control-plane` |
| 9 | `blocked` set | Stop, `stopReason` `flood-guard` |

`ADMIN` is stopped automatically only by rules 8 and 9 and the host's 4 h backstop. Otherwise only `POST /host/stop` stops it.

### Host agent

`game-agent` is a static Go binary run by `game-agent.service`.

**Lifetime and limits:**

- It starts in every boot (A3), but only after its hash is verified, and runs in game and admin mode. It doesn't need `/srv`.
- It never starts or stops containers, never touches the Docker socket, and opens no listening ports.
- Its only outward actions are `events:PutEvents` (one entry per call) and `systemctl poweroff`.
- It uses the instance role through IMDS. Containers can't, because of ADR-0002's hop limit of 1, its `DOCKER-USER` drop rule and its ban on host networking.

**Runtime directory.** `/etc/tmpfiles.d/bradfordly-games.conf` declares `d /run/bradfordly-games 0700 root root -`.

- User data applies it with `systemd-tmpfiles --create` on first boot. On later boots systemd creates the directory early, before either the agent or the launcher starts.
- Files in it are 0600, and each has one writer:
  - the daemon writes `agent-state.json`;
  - `game-agent hold`/`release` write `hold-until`;
  - the launcher writes `admin-reason`.

**Unit hardening (S8):**

- Filesystem and process: `NoNewPrivileges=yes`, `ProtectSystem=strict`, `ReadWritePaths=/run/bradfordly-games`, `ProtectHome=yes`, `PrivateTmp=yes`, `PrivateDevices=yes`.
- Kernel and system calls: `ProtectKernelTunables=yes`, `ProtectKernelModules=yes`, `ProtectControlGroups=yes`, `LockPersonality=yes`, `RestrictSUIDSGID=yes`, `SystemCallArchitectures=native`.
- Capabilities and sockets: `CapabilityBoundingSet=CAP_NET_ADMIN`, which is enough for conntrack over nfnetlink [30]; `RestrictAddressFamilies=AF_UNIX AF_NETLINK AF_INET AF_INET6`.
- Never `PrivateNetwork=`, which would hide the host's conntrack table.
- `ExecStartPre` re-checks the binary's SHA-256 against `/etc/bradfordly-games/game-agent.sha256`. `Restart=on-failure`.

It runs as root to read the journal and call `systemctl poweroff`, which goes through logind and needs no `CAP_SYS_BOOT`. If the first deploy shows otherwise, the fallback is `systemctl start poweroff.target --job-mode=replace-irreversibly`, never `--force`.

**Inputs:**

- The `game` tag from IMDS, validated with ADR-0002's regex.
- The catalog parameter (already readable by the instance).
- `systemctl is-active game@<id>`.
- `journalctl -b --no-tail -f -o json CONTAINER_NAME=game-<id>` [34]. `--no-tail` makes a restarted agent re-read the whole boot's log.
- conntrack over netlink, with `nf_conntrack_acct=1` (A3).
- `hold-until`.
- Unit environment variables: `IDLE_EMPTY_MINUTES=15`, `IDLE_NOJOIN_MINUTES=20`, `READY_TIMEOUT_MINUTES=30`, `PEER_MIN_PACKETS=60`.

The daemon persists `readyAt`, `firstPeerAt`, `lastPeerAt` and `gameActiveSince` in `agent-state.json`, so a restart resumes its timers.

**Peers.** A peer is a distinct remote address whose conntrack flows to any public port of the package grew by at least `PEER_MIN_PACKETS` packets **in each direction** during a 30 s sample. The default, 60 per 30 s (2 pps), is an estimate. Active play in the target games should run well above 2 pps in both directions, while pings, A2S queries and slow scanners run well below it.

- An idle TCP entry (kept for 5 days by default) never counts [29].
- The query port needs no special case. ADR-0002 requires `query` to be one of the public ports, and Minecraft answers queries on its game port.
- A player standing idle may fall below the threshold. Their session then ends 15 min after activity stops, which suits a cost-first owner.
- The first deploy measures real rates per game. 0004 may tune the threshold per package.
- Synth asserts that every catalog entry has at least one public port.
- Someone who keeps exchanging enough packets with a game port still counts as a peer. That is bounded by the 12 h backstops and the run-hours cap, not by idle detection.

**Required package fields** (requirements on 0004's schema):

| Field | Use | Interim (ADR-0002 catalog until 0004) |
|---|---|---|
| `ports[]` with `exposure: public` | Ports whose flows are counted | Existing field |
| `readyPattern` | Log text meaning "ready". 0004 must anchor it to the game's log-line prefix, because console lines echo chat and player names | New optional field (A6). It is a substring match, so a player could trigger an early "ready", which is low impact. If absent, the agent reports `unverified` 60 s after `game@<id>` becomes active |
| `authRequiredPattern?` | Log text meaning game-account auth is needed. Anchored like `readyPattern` | Not available until 0004 |
| `readyTimeoutSeconds?` | Overrides the 30-minute ready timeout (and reconcile rule 6) | Not available until 0004 |

| Situation | Agent action |
|---|---|
| From agent start | `Host Heartbeat` every 5 min, plus on a change between zero and non-zero peers (at most once a minute) |
| Ready pattern seen; or peers > 0 before any ready line; or the interim fallback | `Game Ready` (basis `pattern`, `peers` or `unverified`) |
| Auth pattern seen | `Game Auth Required` (no text), at most once per 10 min |
| Not ready 30 min after `game@<id>` became active, and no peers | `Host Shutting Down` (`ready-timeout`), power off |
| Ready, and no peer for 20 min | `Host Shutting Down` (`no-join`), power off |
| No peers for 15 min after at least one peer | `Host Shutting Down` (`idle`), power off |
| `game@<id>` inactive for 2 min after having been active this boot (not during shutdown) | `Host Shutting Down` (`game-exited`), power off |
| Fail-closed admin mode | `Host Admin Mode` once, with a reason enum read from `admin-reason` (A4). No power-off |
| Hold active (O3) | None of the four power-off rules apply; heartbeats say `mode: hold`. When the hold ends, all four windows (ready-timeout, no-join, idle, game-exited) restart from that moment |
| conntrack or accounting unavailable | `peers: null`, `idleDetection: false`. The no-join and idle rules are off, because the agent never powers off on unknown activity. Ready-timeout and game-exited still apply |
| `PutEvents` fails | Log and retry up to 3 times. Never delays a power-off |

**Hold (O3):**

- `game-agent hold [--minutes N]` (default 60) writes `hold-until`; `game-agent release` deletes it. Both are run as root over Session Manager.
- A hold pauses the agent's power-off rules and reconcile rule 6. It doesn't pause ADR-0002's 4 h or 12 h backstops, or reconcile rules 8 and 9.
- This replaces stopping the agent during admin work, and covers stopping `game@<id>` to edit files, which would otherwise trigger `game-exited`.

**Power-off and failures:**

- Power-off is ADR-0002's graceful path. The resulting EC2 events drive DNS and tag cleanup.
- ADR-0002's backstop units call `game-agent notify --reason backstop-admin|backstop-uptime` first (best effort).
- If the agent crashes, systemd restarts it and it resumes from its persisted state.
- If it stays dead, never passed verification, or was never installed, reconcile rule 5 stops the host within about 20–25 min.

**Host input is untrusted.** It reaches only `hostevents`, which can move the item to `READY` or `STOPPING` and emit domain events, but has no EC2, Route 53 or IAM permissions. The reconcile then stops a `STOPPING` host that keeps running; a compromised host could just as well stop itself. Host events can't start servers, tag the instance or point DNS anywhere.

### Events

All events go on the `bradfordly-games` bus, except EC2's, which arrive on the default bus.

- Every detail carries `v: 1`, `instanceId`, `at` and, where it applies, `serverId`.
- Host events add `bootTime` (from `/proc/stat`).
- Domain events add `startId`.
- No detail carries free text from logs, secrets or player IPs.
- Consumers validate reason enums, drop unknown ones, and ignore unknown fields.

| Source | Detail-type | Type-specific detail | Producer | Consumer |
|---|---|---|---|---|
| `aws.ec2` (default bus) | `EC2 Instance State-change Notification` | AWS-defined | EC2 | `events` |
| `bradfordly-games.host` | `Host Heartbeat` | `mode` (`booting`/`game`/`admin`/`hold`), `peers` (int or null), `ready`, `idleDetection` | agent | `hostevents` |
| | `Game Ready` | `basis` (`pattern`/`peers`/`unverified`), `bootSeconds`, `idleDetection` | agent | `hostevents` |
| | `Game Auth Required` | none | agent | `hostevents` |
| | `Host Admin Mode` | `reason` (the failed launcher check) | agent | `hostevents` |
| | `Host Shutting Down` | `reason` (`idle`, `no-join`, `ready-timeout`, `game-exited`, `backstop-admin`, `backstop-uptime`) | agent | `hostevents` |
| `bradfordly-games.cli`, later `.wake` and `.discord` | `Start Requested`, `Stop Requested` | `override?` | owner, 0005, future | `events` |
| `bradfordly-games.cli` only | `Unblock Requested` | none | owner | `events` |
| `bradfordly-games.lifecycle` | `Server Starting` | `cause` `{kind: user\|cli\|lookup\|discord\|external, user?}` | `api`, `events` | `notifier` |
| | `Server Ready` | `endpoint` `{hostname, ports[]}`, copied from the item and never from host input; `basis`; `idleDetection` | `hostevents` | `notifier` |
| | `Server Auth Required` | none | `hostevents` | `notifier` |
| | `Server Start Failed` | `reason` (`ec2-error`, `ec2-timeout`, `admin-mode`, `ready-timeout`, `start-timeout`, `agent-lost`) | all three | `notifier` |
| | `Server Stopped` | `reason` (`requested`, `idle`, `no-join`, `game-exited`, `start-failed`, `agent-lost`, `backstop-admin`, `backstop-uptime`, `backstop-control-plane`, `flood-guard`, `external`), `uptimeSeconds` | `events` | `notifier` |
| | `Request Rejected` | `command`, `reason`, `cause` | `api`, `events` | none now |

How host events are accepted:

- `Game Ready` and a heartbeat with `ready: true` both count, but only in `STARTING`.
- `Server Auth Required` is accepted only in `STARTING` or `READY`, at most once per 10 min.
- Host events without a `serverId` (backstop shutdowns, admin heartbeats) are accepted only in `ADMIN`.

Rules:

| Bus | Pattern | Target |
|---|---|---|
| default | `aws.ec2` state changes for this instance | `events` |
| custom | source `bradfordly-games.host` | `hostevents` |
| custom | command detail-types from the allowed sources (`Unblock Requested` from `.cli` only) | `events` |
| custom | source `bradfordly-games.lifecycle` | `notifier` |

Targets get full envelopes (no `InputPath` or transformer), and handlers dispatch on (source, detail-type).

Asynchronous invoke settings (the layer that applies once Lambda has accepted an event [11]):

| Function | Maximum event age | Retries |
|---|---|---|
| `events` | 1 h | 2 |
| `hostevents` | 10 min | 1 |
| `notifier` | 1 h | 2 |

There are no DLQs: the reconcile repairs state, and notifications are best-effort. The UI (0006) polls `GET /status`.

### Discord notifier

**Webhook URL storage:**

- The URL is a SecureString, `/bradfordly-games/control/discord-webhook-url` (standard tier, `aws/ssm`).
- The owner creates it with the CLI, because CloudFormation can't [20].
- `DISCORD_WEBHOOK_PARAM` names it, and the notifier caches the value for 15 min. If it's missing, notifications are disabled with a warning.
- Secrets Manager would cost $0.40/month [22].
- To rotate: create a new webhook, overwrite the parameter, delete the old one.

**Message hardening (S4, S7):**

- Messages are built only from validated event fields.
- Every free-text field is escaped for Discord markdown.
- Posts set `allowed_mentions: {"parse": []}` and `flags: 4` (`SUPPRESS_EMBEDS`) [31].
- The auth-required message is fixed: "X needs game-account authentication; read the console via Session Manager". No URL or code from a log is ever forwarded.
- Errors are logged with the webhook URL redacted, because Go's `*url.Error` prints the full URL, and the token alone can post to, edit or delete the webhook.

| Owner's message | Event |
|---|---|
| starting | `Server Starting` with cause `user`, `cli`, `discord` or `external` ("Starting X, requested by …") |
| lookup detected | `Server Starting` with cause `lookup` ("Lookup detected for X.games.bradfordly.com; cold start underway") |
| ready | `Server Ready` (hostname and ports; "readiness unverified" or "idle detection unavailable" when flagged) |
| stopped (idle) | `Server Stopped` with reason `idle` or `no-join` |
| stopped | `Server Stopped` with any other reason |
| auth required | `Server Auth Required` |
| start failed | `Server Start Failed` |

**Failure isolation:**

- Reserved concurrency of 1 keeps posts serial.
- **429:** honour `retry_after`, retrying in-function if the wait is 5 s or less; otherwise return an error.
- **5xx or network errors:** return an error.
- **401, 403 or 404:** log and don't retry [32].
- Lifecycle functions publish domain events after their state write and only log a `PutEvents` failure. A notification failure can never block or roll back a lifecycle action.

### Flood guard (O4)

`PutEvents` is the host's only permission that is billed per request.

**Alarm:**

- It watches `AWS/Events` `MatchedEvents` for the host-events rule (`EventBusName`, `RuleName`) [7]: statistic Sum over 60 s, threshold greater than 30, 1 of 1 datapoints, missing data not breaching. Normal traffic is at most about 3 events a minute.
- An EC2 stop action isn't valid for this metric [14], so the alarm notifies the topic `bradfordly-games-alerts` [15]. Its subscribers are the owner's email (confirmed once) and `events`.

**Topic policy:** it replaces the default, which lets principals in the account publish [27].

- Allow `sns:Publish` to `cloudwatch.amazonaws.com` with `aws:SourceArn` = the alarm ARN and `aws:SourceAccount`.
- Deny `sns:Publish` to every principal with `ArnNotEquals aws:SourceArn` = the alarm ARN.

**When a notification arrives,** `events` ignores the state in the message body and calls `DescribeAlarms` on the alarm [16]. Only `ALARM` acts; `OK` and `INSUFFICIENT_DATA` do nothing. On `ALARM`:

1. Set `blocked = flood-guard` on the item with a lock-free conditional write, which works even while the lock is held.
2. `iam:AttachRolePolicy` `DenyHostPutEvents` (Deny `events:PutEvents`) to the instance role. This is idempotent. An explicit Deny applies to every session of the role [23], so it also cuts off credentials copied off the host, whatever their remaining lifetime [25].
3. Stop with reason `flood-guard`. If the lock is busy, return an error so Lambda retries [11]; reconcile rule 9 stops a blocked host that is still running.

**While blocked:** Start returns 409 `flood-guard`, and commands get `Request Rejected` (`flood-guard`).

**Clearing (admin runbook):**

1. Detach `DenyHostPutEvents` by hand; no runtime role can detach it.
2. Optionally, as a cleanup step, run IAM "Revoke active sessions" on the instance role. It attaches `AWSRevokeOlderSessions` [26] and is not the primary control.
3. Put `Unblock Requested` from `bradfordly-games.cli`, which clears `blocked`.

Clearing without detaching just means the next start ends at rule 5 (agent-lost), which fails safe.

**Exposure:**

- Metrics are best-effort [7] and alarms act only on a state change [15], so a flood runs for roughly 2–3 minutes before the deny lands: about $1–2 at 10,000 events/s (estimate).
- After the deny, the flood's entries fail authorization and aren't ingested. That denied entries are not billed is unverified. The alarm then returns to `OK`.
- ADR-0002's free account budget alert remains the slow cost signal.

**Synth assertion:** `DenyHostPutEvents` contains only `Deny` statements, so attaching it can only remove permissions.

### IAM

**`api` role:**

- ADR-0002's Grants, unchanged:
  - `ec2:StartInstances` and `ec2:StopInstances` on the instance ARN;
  - `ec2:DescribeInstances`;
  - `ec2:CreateTags` and `ec2:DeleteTags` with their conditions;
  - `ssm:GetParameter` and `ssm:GetParametersByPath` on the catalog path;
  - `route53:ChangeResourceRecordSets` with its name, type and action conditions.
- Additions:
  - `route53:ListResourceRecordSets` on the zone ARN (read before delete).
  - `dynamodb:GetItem` and `dynamodb:UpdateItem` on the table.
  - `events:PutEvents` on the bus, `ForAllValues:StringEquals` source `bradfordly-games.lifecycle` and the domain detail-types, with `Null` checks.
  - Its own log group (14-day retention).

**`events` role:** the same statements, plus:

- `iam:AttachRolePolicy` on the instance role's ARN, with `ArnEquals iam:PolicyARN` = `DenyHostPutEvents` [24]. There is no `iam:DetachRolePolicy`;
- `cloudwatch:DescribeAlarms` on the flood alarm's ARN [16].

**`hostevents` role:**

- `dynamodb:GetItem` on the table.
- `dynamodb:UpdateItem` with `ForAllValues:StringEquals dynamodb:Attributes`, limited to the key and the attributes it writes or tests, plus `Null dynamodb:Attributes = false`.
- `events:PutEvents` as above, limited to `Server Ready`, `Server Auth Required` and `Server Start Failed`.
- Its log group.
- Nothing else.

**`notifier` role:**

- `ssm:GetParameter` on the webhook parameter, plus its log group.
- If `aws/ssm` decryption fails without an IAM statement [19] (a first-deploy test), add `kms:Decrypt` on the `aws/ssm` key with `kms:ViaService` = `ssm.us-east-1.amazonaws.com` and `kms:EncryptionContext:PARAMETER_ARN` = that parameter.

**Invoke permissions:**

- The Scheduler role grants `lambda:InvokeFunction` on `events`. Its trust policy is `scheduler.amazonaws.com` with `aws:SourceAccount` and `aws:SourceArn` = the schedule's ARN.
- SNS (the alert topic) and EventBridge invoke through Lambda resource policies, each limited by `SourceArn`.
- `api` grants invoke only to 0006's front door.

**Bus resource policy:** an explicit Deny in it overrides any identity Allow in the account [23]. Each reserved source gets a Deny of `events:PutEvents` when `ForAnyValue:StringEquals events:source` is that source and `ArnNotLike aws:PrincipalArn` matches none of its producers:

| Source | Producers |
|---|---|
| `.host` | the instance role |
| `.lifecycle` | the `api`, `events` and `hostevents` roles |
| `.cli` | the owner's principal, as an ArnLike pattern from CDK context |
| `.wake` | 0005's role (added by 0005) |

- The owner's principal must be a pattern, because an IAM Identity Center role has an `aws-reserved/sso.amazonaws.com/` path and a generated suffix. An exact ARN would lock the CLI out.
- One catch-all Deny covers any other `bradfordly-games.*` source (`ForAnyValue:StringLike`) from principals outside the full producer set. So `.discord` needs no row until its producer exists.

**Instance role** (the one new permission, A2):

```
Effect: Allow
Action: events:PutEvents
Resource: arn:aws:events:us-east-1:<account>:event-bus/bradfordly-games
Condition:
  ForAllValues:StringEquals:
    events:source: ["bradfordly-games.host"]
    events:detail-type: ["Host Heartbeat", "Game Ready", "Game Auth Required",
                         "Host Admin Mode", "Host Shutting Down"]
  Null: { events:source: "false", events:detail-type: "false" }
```

The flood guard can attach `DenyHostPutEvents` to this role at runtime. Only an admin detaches it.

**Not needed:** nothing on ADR-0002's "Never granted" list.

- Timeouts run on a deploy-time schedule, which avoids `iam:PassRole` [6].
- Readiness is pushed by the agent, so the control plane never needs `ssm:SendCommand`.
- No control-plane runtime role has `iam:*`, `scheduler:*`, `lambda:*`, `events:PutRule` or `events:PutTargets`. The one stated exception is the `events` role's `iam:AttachRolePolicy`, limited to attaching the Deny-only `DenyHostPutEvents` to the instance role. That grant can only take permissions away.

**Left for 0004 to change:** ADR-0002's Grants enumerate catalog ids in `aws:RequestTag/game` and enumerate game record names in `…NormalizedRecordNames`. Runtime servers need both changed (for example `ForAllValues:StringLike` [39] with a Deny on reserved names). This ADR keeps them as written.

### Interfaces for later ADRs

- **0004:**
  - Supply the package fields above, with anchored patterns.
  - Replace the server-lookup function, which resolves an id to {exists, hostname, public ports}.
  - Keep ids matching `^[a-z0-9][a-z0-9-]{0,31}$`.
  - Define the admin routes and the per-server idle and peer-threshold overrides.
  - If parameters move, change the instance's `GetParameter` path. Keep `/bradfordly-games/control/*` outside every prefix granted to the instance.
- **0005:**
  - Emit `Start Requested` from `bradfordly-games.wake`, with its own source-scoped grant, one rule entry and one bus-policy producer entry. Wake starts can't override the cap.
  - Decide abuse limits. One input: sustained fake peers can hold every wake to the 12 h 15 min bound, so about 7 hostile wakes exhaust the 80 h cap. That is an availability denial of service.
  - Amend the DNS write points (the running handler and reconcile rule 7) for placeholder records.
  - Optionally subscribe to `Request Rejected`, for `budget-exhausted`, `flood-guard` and other rejections.
- **0006:**
  - Provide the front door that invokes `api` with a verified `{username, groups}`.
  - Don't use a Function URL with `AuthType NONE` on `api` unless `api` itself verifies the JWT: signature, `iss`, `aud`/`client_id`, `token_use` and `exp`.
  - Map groups exactly to the route levels above.
  - Poll `GET /status`.
- **Future slash commands:** another command producer (`bradfordly-games.discord`), plus a consumer of domain events. No lifecycle rework.

### Cost

Unit prices are verified [4][9][10][13][18][21]; the volumes are estimates.

| Item | Idle month | One 2-hour session |
|---|---|---|
| Lambda (8,640 reconcile runs ≈ 216 GB-s; about 50 invocations per session) | $0 in the free tier; about $0.005 without it | < $0.001 |
| DynamoDB (about 9,000 reads idle; about 60 writes per session) | about $0.001 | < $0.001 |
| EventBridge (about 40 events per session) and Scheduler (8,640, inside 14M free) | $0 | < $0.001 |
| CloudWatch Logs (about 5 MB, 14-day retention) and one alarm (inside 10 free) | $0 | negligible |
| SNS topic and Route 53 API reads | $0 (free tiers unverified) | $0 |
| SSM SecureString and KMS decrypts | $0 | inside 20,000 free |
| **Total** | **about $0.00 ($0.01 without free tiers)** | **< $0.01** |

- ADR-0002's $1.26–1.76 of idle headroom is effectively untouched.
- Active EC2 spend has a monthly ceiling: 80 h plus at most one session of up to 12 h 15 min that started under the cap, about 92 h × $0.091 ≈ $8.40/month, plus any admin overrides.
- Idle shutdown ends a forgotten session about 15 minutes after the last player leaves instead of at 12 h, saving up to about $1.07 (11.75 h × $0.091).
- A `PutEvents` flood, from the host or from copied credentials, costs a few dollars before the instant deny cuts it off.

## Amends ADR-0002

ADR-0002's status line records these amendments in this PR.

- **A1. Contract with ADR-0003 → Signals.**
  - Old: "no on-host player-count signal in v0… an on-host signal would amend this contract."
  - New: `game-agent` reports heartbeat (peers), readiness, auth required, admin mode and its own shutdowns as events, and powers the host off on idle. Host input is untrusted: it reaches only a function with no EC2, Route 53 or IAM permissions.
- **A2. Instance IAM.**
  - Old: Session Manager minimum plus `ssm:GetParameter` on the catalog path; "Nothing else".
  - New: also `events:PutEvents` on the `bradfordly-games` bus only, conditioned on source `bradfordly-games.host` and the five host detail-types with Null checks. The bus resource policy denies that source to every other principal.
  - The control plane's flood guard may attach the Deny-only `DenyHostPutEvents` to this role at runtime; only an admin detaches it.
- **A3. Server-plane invariants → 1. Boot sequence.**
  - Old: mount, patch, maybe reboot, Docker, IMDS rule, launcher.
  - New, after the mount attempt and before patching:
    - set `net.netfilter.nf_conntrack_acct=1`;
    - apply the tmpfiles.d entry for `/run/bradfordly-games` (0700);
    - check the on-disk `game-agent` against the SHA-256 that user data writes to `/etc/bradfordly-games/game-agent.sha256`. If it is missing or mismatched, delete it, download the pinned GitHub Release asset into a root-owned 0700 directory, verify it, then `install -m 0755` it to `/usr/local/lib/bradfordly-games/`;
    - start `game-agent.service` only after a successful verify. The unit's `ExecStartPre` re-checks the hash, and the unit doesn't require `/srv`.
  - Failures are non-fatal for the boot, which continues without the agent. An unverified binary never runs.
  - A pin change changes user data and restarts the instance [ADR-0002 14], so agent upgrades follow "deploy, then stop".
- **A4. Server-plane invariants → 2. Fail closed to admin mode.**
  - Old: the listed checks drop the host into admin mode.
  - New: unchanged, and the launcher also writes the failed check's name to `/run/bradfordly-games/admin-reason` (0600).
- **A5. Server-plane invariants → 7. Backstop timers.**
  - Old: power off after 4 h of admin mode and after 12 h of uptime.
  - New: both are unchanged and call `game-agent notify` first. The agent also powers off on its idle, no-join, ready-timeout and game-exited rules.
  - It never powers off on unknown activity, in admin mode, or during a hold (`game-agent hold`/`release`).
  - The control plane mirrors both backstops from AWS at 12 h 15 min and 4 h 15 min (O1).
- **A6. Game catalog → value shape.**
  - Old: the listed fields.
  - New: plus an optional, interim `readyPattern` (1–200 printable ASCII characters; substring; asserted at synth), and a synth assertion of at least one public port. 0004 supersedes both and must anchor patterns.
- **A7. Contract → Single-flight lifecycle.**
  - The lock is the DynamoDB lease, shared by every writer that calls EC2 or Route 53.
  - Start requires EC2 `stopped` to be observed under the lock.
  - Every adopted run takes its `LaunchTime` as `attemptAt`, and EC2 and host events older than the current attempt are dropped.
  - A failed requested start (launcher fails closed) is stopped by the control plane instead of waiting for the 4 h backstop.
- **A8. Contract → Grants.**
  - Old: the listed server-plane grants.
  - New: unchanged, extended with:
    - `route53:ListResourceRecordSets` on the zone (read before DELETE; the reconcile removes stray records);
    - the DynamoDB table;
    - source-scoped `PutEvents`;
    - the Scheduler role and the SNS topic;
    - the `events` role's `iam:AttachRolePolicy`, limited to attaching `DenyHostPutEvents` to the instance role. This action isn't on "Never granted".
  - Nothing from "Never granted" is used.
- **A9. Contract → Unrequested runs happen.**
  - Old: the host backstops end unrequested runs within 4 h or 12 h.
  - New: also ended from AWS (O1). Starts are refused above 80 run-hours a month unless an admin overrides (O2), and while the flood guard's latch is set.
- **A10. Consequences → Revisit if.** The trigger "ADR-0003 needs an on-host player-count or readiness signal" is fulfilled by A1–A5.

Left to later ADRs:

- **0004:** the enumerated tag values and record names in Grants, the secrets model, the launcher's directory rule.
- **0005:** "the A record exists only while the instance runs" and Stop's DELETE.

## Amends ADR-0001

- **B1. Decision → Interaction with ADR-0002.**
  - Old: "If ADR-0002 needs a long-running agent or sidecar, it ships as a Go container image built by CDK assets."
  - New: the on-host agent ships as a static Go binary (`CGO_ENABLED=0 go build -trimpath`, toolchain pinned in `go.mod`), attached to a GitHub Release of this repo, SHA-256-pinned in user data, with a build attestation.
  - Reason: a container would need host access that ADR-0002 forbids for containers, and a CDK asset would need registry or S3 read on the instance.
  - Control-plane Lambdas are unchanged.

## Consequences

Positive:

- Control-plane idle cost is about $0, so ADR-0002's headroom survives for 0004–0006.
- Idle shutdown runs on the host and keeps working when the control plane is down. The backstops now run on both sides, a monthly cap bounds active spend, and a `PutEvents` flood is cut off within minutes, copied credentials included.
- One lease and attempt-scoped events give a state machine where every non-terminal state has an exit, plus a reconcile that repairs lost events, failed stops, stale DNS, a dead agent and a missing item.
- It is game-agnostic: readiness comes from a log pattern, activity from bidirectional conntrack counters, and no game query protocol is needed.
- Trust boundaries are explicit:
  - host input never reaches EC2, Route 53 or IAM permissions;
  - API identity comes only from the front door, and `api` holds no IAM permission;
  - bus sources are bound to their producers, and only CloudWatch can trigger the flood guard;
  - the webhook token stays out of logs, and log text never reaches Discord.
- 0005, 0006 and future slash commands plug in as producers and consumers without lifecycle rework.

Negative:

- More moving parts than the first draft:
  - four functions, an alarm and a locked SNS topic;
  - a runtime IAM attach, and an account-concurrency prerequisite;
  - an on-host binary to build, attest, release and patch. Agent upgrades restart the instance.
- A flood still runs for the 2–3 minutes before the alarm fires.
- A false or forged alarm blocks all starts until an admin detaches the policy and unblocks. Forging is limited by the topic policy and the `DescribeAlarms` check.
- Player counts are approximate:
  - someone who keeps exchanging enough packets with a game port keeps the host up until the backstops;
  - an idle player may count as absent;
  - friends behind one NAT count as one;
  - the 2 pps default is an estimate to tune per game.
- The interim `readyPattern` is spoofable from chat (an early "ready" message only). Anchoring waits for 0004.
- Reconcile-detected failures surface within about 25 minutes. Admin-mode start failures stop within about 10.
- Until 0006, starts and stops go through the AWS CLI.
- If the item is lost, the run-hours counter resets and the latch clears. So the cap fails open for the rest of that month, and a still-attached deny is the only remaining block. Everything else is rebuilt by the reconcile, which is why point-in-time recovery is off.
- Unverified until the first deploy:
  - `nf_conntrack_netlink` and accounting on AL2023;
  - `systemctl poweroff` and journal reads under the hardening;
  - `aws/ssm` decryption without an IAM statement;
  - `StopInstances` behaviour on a `pending` instance;
  - that denied `PutEvents` entries are unbilled;
  - that a CloudFormation deploy leaves an out-of-band attachment of `DenyHostPutEvents` in place;
  - the account's concurrency quota.

Revisit if:

- conntrack is unusable on AL2023: add per-package join/leave log patterns (0004) or protocol probes.
- Credential theft becomes a real concern: add uid-0 IMDS filtering on the host.
- A second host or concurrent servers appear: the item is already per host; add a host id to the API.
- Members want commands, or Discord limits bite: write an interactions-endpoint ADR [33].
- The UI needs history: add an EventBridge archive or an event log table.
- 80 h/month proves wrong: change `RUN_HOURS_CAP`, or add a rest-of-month override.

## Follow-ups (not part of this ADR)

1. **Infra PR:**
   - the bus with its resource policy; the rules; the table; the four functions with their roles and async settings; the schedule; the alarm, the topic and its policy; `DenyHostPutEvents`;
   - the agent unit with its hardening, `ExecStartPre` and the tmpfiles.d entry;
   - a preflight that fails the deploy when `AccountLimit.ConcurrentExecutions` < 102, unless the context flag that omits reservations is set;
   - synth assertions: `DenyHostPutEvents` is Deny-only, every catalog entry has a public port, and `readyPattern` is well formed;
   - the release workflow:
     - runs on protected `agent-v*` tags only (no `pull_request_target`);
     - grants `contents: write` only to the release job and uses SHA-pinned actions;
     - builds with `CGO_ENABLED=0 go build -trimpath` and produces an artifact attestation [41];
   - a required check on every pin-bump PR that runs `gh attestation verify <asset> --repo Bradfordly/bradfordly-games --signer-workflow Bradfordly/bradfordly-games/.github/workflows/<release>.yml --source-ref refs/tags/agent-v<x> --deny-self-hosted-runners` and asserts that the asset's SHA-256 equals the pin in the PR;
   - expected cdk-nag suppressions: AwsSolutions-DDB3 (no point-in-time recovery; see Negative) and IAM5 for `ec2:DescribeInstances`.
2. **First-deploy tests:**
   - conntrack and accounting visible to the agent under its hardening, for UDP and TCP flows; `systemctl poweroff` and journal reads work; real per-game packet rates are recorded;
   - host `PutEvents`: an allowed event succeeds; a wrong source, a wrong detail-type, another bus and a mixed-entry batch fail per entry;
   - the bus policy denies `.lifecycle` and `.cli` from the instance role, `.host` from the owner, and an unlisted `bradfordly-games.*` source; the owner's Identity Center role passes the `.cli` pattern;
   - `hostevents` conditional writes pass under the `dynamodb:Attributes` condition;
   - a container still can't get an IMDSv2 token; read `Expiration` from IMDS to record the real credential lifetime;
   - the notifier reads the SecureString with `ssm:GetParameter` alone (otherwise add the scoped `kms:Decrypt`);
   - the flood guard:
     - a forced `ALARM` latches, attaches the deny, stops the instance and emails the owner;
     - an `OK` notification does nothing;
     - a hand-published SNS message is denied;
     - a deploy while the deny is attached leaves it attached;
   - idle, no-join, ready-timeout, game-exited, hold and agent-lost each end with the right DNS and tag cleanup and Discord message;
   - Stop on a `pending` instance; two concurrent Starts; a CloudFormation restart mid-session ends in `ADMIN` with sane run-hours.
3. **Runbooks:**
   - create and rotate the webhook parameter;
   - start and stop with `aws events put-events`, including the cap override;
   - `game-agent hold`/`release` during hands-on admin work;
   - upgrade the agent (deploy, then stop);
   - after a flood alarm: investigate; detach `DenyHostPutEvents`; optionally revoke the role's active sessions; then put `Unblock Requested`.
4. Measure boot-to-ready time, agent CPU and real per-game packet rates in the first sessions, then tune `READY_TIMEOUT_MINUTES` and `PEER_MIN_PACKETS`.

## Sources

Accessed 2026-10-12 or 2026-10-13. "Security review" marks facts that the 2026-10-13 security review checked against the cited page.

1. EventBridge actions and condition keys (China-partition Service Authorization Reference) — https://docs.amazonaws.cn/service-authorization/latest/reference/list_amazoneventbridge.html (ArrayOfString key type: security review)
2. `PutEvents` API reference — https://docs.aws.amazon.com/eventbridge/latest/APIReference/API_PutEvents.html (per-entry errors: security review)
3. EventBridge IAM conditions — https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-use-conditions.html
4. EventBridge pricing (including Scheduler) — https://aws.amazon.com/eventbridge/pricing/
5. EventBridge retry policy — https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-rule-retry-policy.html
6. EventBridge Scheduler permissions — https://docs.aws.amazon.com/scheduler/latest/UserGuide/security_iam_id-based-policy-examples.html
7. EventBridge metrics — https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-monitoring.html
8. Step Functions pricing — https://aws.amazon.com/step-functions/pricing/
9. DynamoDB on-demand pricing — https://aws.amazon.com/dynamodb/pricing/on-demand/
10. Lambda pricing — https://aws.amazon.com/lambda/pricing/
11. Lambda asynchronous error handling — https://docs.aws.amazon.com/lambda/latest/dg/invocation-async-error-handling.html
12. Lambda reserved concurrency — https://docs.aws.amazon.com/lambda/latest/dg/configuration-concurrency.html
13. CloudWatch pricing — https://aws.amazon.com/cloudwatch/pricing/
14. CloudWatch EC2 alarm actions — https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/UsingAlarmActions.html
15. CloudWatch alarms — https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/AlarmThatSendsEmail.html
16. CloudWatch actions and resources (China-partition Service Authorization Reference) — https://docs.amazonaws.cn/service-authorization/latest/reference/list_amazoncloudwatch.html
17. EC2 CloudWatch metrics — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/viewing_metrics_with_cloudwatch.html
18. Systems Manager pricing — https://aws.amazon.com/systems-manager/pricing/
19. KMS encryption for SecureString parameters — https://docs.aws.amazon.com/kms/latest/developerguide/services-parameter-store.html
20. `AWS::SSM::Parameter` — https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ssm-parameter.html
21. AWS KMS pricing — https://aws.amazon.com/kms/pricing/
22. Secrets Manager pricing — https://aws.amazon.com/secrets-manager/pricing/
23. IAM policy evaluation logic — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic.html (security review)
24. IAM actions and condition keys — https://docs.aws.amazon.com/service-authorization/latest/reference/list_awsidentityandaccessmanagementiam.html (`iam:PolicyARN` on Attach/DetachRolePolicy: security review)
25. IAM roles for Amazon EC2 — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html (security review)
26. Revoking IAM role sessions — https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_revoke-sessions.html (security review)
27. Amazon SNS access control — https://docs.aws.amazon.com/sns/latest/dg/sns-authentication-and-access-control.html (default topic policy: security review)
28. `StopInstances` — https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_StopInstances.html ; stop troubleshooting (force stop in `pending`) — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/TroubleshootingInstancesStopping.html
29. Netfilter conntrack sysctls — https://docs.kernel.org/networking/nf_conntrack-sysctl.html
30. Linux `nfnetlink` (`netlink_net_capable(CAP_NET_ADMIN)` in `nfnetlink_rcv`) — https://github.com/torvalds/linux/blob/master/net/netfilter/nfnetlink.c (security review)
31. Discord webhooks — https://docs.discord.com/developers/resources/webhook
32. Discord rate limits — https://docs.discord.com/developers/topics/rate-limits
33. Discord interactions — https://docs.discord.com/developers/interactions/overview
34. Docker journald logging driver — https://docs.docker.com/engine/logging/drivers/journald/
35. Hytale Server Manual — https://support.hytale.com/hc/en-us/articles/45326769420827 (read through the Help Center API; article updated 2026-06-15)
36. Pelican eggs — https://pelican.dev/docs/eggs/creating-a-custom-egg ; Hytale egg — https://raw.githubusercontent.com/pelican-eggs/games-standalone/main/hytale/egg-hytale.yaml
37. itzg/minecraft-server AUTOSTOP — https://docker-minecraft-server.readthedocs.io/en/latest/misc/autopause-autostop/autostop/
38. doctorray117/minecraft-ondemand — https://github.com/doctorray117/minecraft-ondemand
39. Route 53 IAM conditions — https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/specifying-rrset-conditions.html
40. Repository — https://github.com/Bradfordly/bradfordly-games
41. `gh attestation verify` — https://cli.github.com/manual/gh_attestation_verify (security review)
