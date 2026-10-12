# 0003. Control plane: server lifecycle API and idle shutdown

- Status: Proposed
- Date: 2026-10-12

## Context

Issue #16. ADR-0002 (merged, Proposed) defines the server plane: one stopped-at-rest `m6a.large` running one game container at a time, selected by the instance's `game` tag; a launcher that fails closed to admin mode; host backstops (4 h admin mode, 12 h uptime); and a "Contract with ADR-0003" (single-flight lifecycle, DNS, Grants, "Never granted") that this ADR honours. ADR-0001 fixes Go 1.27 on Lambda (`provided.al2023`, arm64) for code we write.

This ADR decides the control-plane core: the lifecycle state machine and API, the lock and state store, how a start is followed to "ready", idle shutdown, a host agent that reports readiness and activity, the domain events, and Discord notifications. It defines the interfaces that three later ADRs plug into, and decides none of their content:

- **0004 (#17) Game packages and per-server configuration:** package manifest, runtime servers, forms schema, secrets, world provisioning, the Hytale package, and the catalog and Grants amendments.
- **0005 (#18) Wake on connect:** DNS query-log trigger, placeholder records, abuse limits, and the DNS contract amendment.
- **0006 (#19) Web frontend and authentication:** hosting, Cognito, sessions, domain and CAA.

Owner decisions (2026-10-12; authoritative):

- Total idle cost stays under $5/month. ADR-0002 leaves about $1.26–1.76/month for the control plane and everything after it. Long cold starts are acceptable.
- Worlds are created in the UI from existing packages; new game types (image, ports, capabilities, schema) only by reviewed PR (0004).
- Login is invite-only Cognito. Group `admin` creates servers and edits config and secrets; every other invited user (a "member") views status and starts or stops servers. 0006 decides how sessions carry the group.
- Wake on connect is a Route 53 query-log trigger (0005). Messages say "lookup detected", never "login attempted".
- Readiness and idle detection come from a Go host agent. It watches the game container's journal for the package's ready pattern, counts conntrack flows on the package's public ports, powers the host off locally when idle (no IAM), and sends events with `events:PutEvents` scoped to one custom bus by `events:source` and `events:detail-type` conditions plus Null checks.
- Idle defaults: stop 15 min after the last player leaves, or 20 min after "ready" if nobody joins. The 4 h admin-mode and 12 h uptime backstops stay. Per-server overrides may come later (0004 owns per-server config).
- Discord gets an incoming webhook only, with messages for starting, lookup detected, ready, stopped (idle), stopped, auth required and start failed. No interactions endpoint or gateway bot now, but slash commands must be addable later without reworking the events.

Verified facts (as of 2026-10-12, us-east-1 unless noted; "(unverified)" marks the rest):

- `events:PutEvents` accepts the condition keys `events:source`, `events:detail-type`, `events:eventBusInvocation`, `events:SystemMetadata/EventGroupId` and `events:ResourceArn` on event-bus resources (read from the China-partition Service Authorization Reference, because the global page didn't render) [1]. A `ForAllValues` condition without a `Null` check evaluates true when the key is absent (documented for `PutRule`) [2]. Whether PutEvents presents these keys as single- or multi-valued isn't stated (unverified).
- EventBridge classic custom-bus events cost $1.00 per million, billed per 64 KB chunk, and "AWS management events are ingested by the event bus for free" (EC2 state-change events assumed to be free too; unverified). EventBridge Scheduler includes 14 million invocations per month free, then $1.00 per million [3]. Rule targets are retried for 24 h and up to 185 times by default, then dropped unless a DLQ is configured [4]. **Creating or updating a Scheduler schedule requires `iam:PassRole` on the schedule's execution role** [5].
- Step Functions Standard costs $0.000025 per state transition, with 4,000 free per month that don't expire. Express costs $1.00 per million requests plus duration [6].
- DynamoDB on-demand costs $0.625 per million writes, $0.125 per million reads and $0.25/GB-month, with 25 GB of storage in the free tier. The page doesn't say whether that tier is always free, and its free WCU/RCU apply to provisioned capacity only [7].
- Lambda includes 1M requests and 400,000 GB-s per month free (always-free status not stated). arm64 costs $0.0000133334/GB-s and $0.20 per million requests [8]. For asynchronous invocations Lambda retries function errors twice (after 1 min, then 2 min), retries throttles for up to 6 h, and may deliver an event more than once [9].
- CloudWatch includes 5 GB of logs data per month free (ingestion, archive storage and Insights scans); its examples use $0.50/GB for ingestion [10]. EC2 `NetworkPacketsIn` exists only at the 5-minute basic-monitoring resolution and counts all interfaces [11].
- Parameter Store standard parameters have no storage or standard-throughput API charge [12].
  - Standard SecureStrings (up to 4,096 bytes) are encrypted directly under a KMS key; the default is the AWS managed `aws/ssm` [13].
  - The same page says decrypting "needs kms:Decrypt permission" and that "you cannot establish access control policies for the default aws/ssm KMS key". So whether a role needs an IAM `kms:Decrypt` statement for `aws/ssm` is unclear (unverified).
  - CloudFormation can't create SecureString parameters [14].
  - KMS: AWS managed keys have no monthly fee, but requests are billed, with 20,000 free per month [15]. Secrets Manager costs $0.40 per secret-month [16].
- Discord:
  - A webhook is executed at `/webhooks/{id}/{token}` with no other authentication, and `allowed_mentions` restricts mentions [17].
  - Webhooks have per-route rate-limit buckets with no published figure. 10,000 invalid requests (401, 403 or 429) in 10 minutes trigger a temporary ban, and a webhook that returns 404 should no longer be used [18].
  - Interactions endpoints must verify Ed25519 signatures and send an initial response within 3 s; they support deferred responses and 15-minute follow-up tokens [19].
- Docker's journald log driver adds `CONTAINER_NAME` and other fields to every message; `journalctl CONTAINER_NAME=<name>` filters on it [20].
- Hytale documents no query or status protocol, and rewrites its config files while running [21]. Pelican's community egg treats `Hytale Server Booted!` as readiness: a substring ("done") that moves a server from STARTING to RUNNING [22] (not checked against real output). itzg's Minecraft image has a built-in AUTOSTOP, and doctorray117/minecraft-ondemand runs a watchdog sidecar: prior art for idle stop on the game host [23][24].
- Route 53's `…NormalizedRecordNames` condition key accepts `ForAllValues:StringLike` with IAM wildcards [25] (needed by 0004, not here).
- This repository is public on GitHub and has no releases yet [26]. Whether AL2023 exposes conntrack through `nf_conntrack_netlink` with Docker's NAT rules loaded is unverified.

## Options considered

All options use Lambda, DynamoDB and EventBridge in the same way. They differ in how a start is followed to "ready" and how failures are detected.

### A. Simplest viable option: purely event-driven, no timers in AWS

The control plane reacts only to three inputs: API calls, EC2 state-change events and host-agent events. One DynamoDB item holds the state and a lease lock. All timers live on the host: the agent's idle and ready-timeout rules, plus ADR-0002's backstops. Idle cost is $0.
Rejected (narrowly) because some failures can only be detected from AWS. Examples: an agent that never starts or crashes, a lost event after Lambda's two retries [9], or a Lambda that dies between the state write and `StartInstances`. In each case the item stays at STARTING or READY, the UI shows a stale state, and the instance bills until the 12 h backstop (about $1.09 per incident).

### B. A plus one recurring reconcile (recommended)

Same as A, plus one CDK-created EventBridge Scheduler schedule that invokes the lifecycle function every 5 minutes. It compares the item with `DescribeInstances` and the last agent heartbeat, then repairs drift and enforces control-plane timeouts. At 8,640 runs per month it sits inside Scheduler's free tier [3] and costs fractions of a cent in Lambda and DynamoDB (Cost, below). Details are under Decision.

### C. A Step Functions Standard execution per start

A state machine would wait for the agent's ready event with a task token (or poll in a Wait loop), with a timeout on the state. Transitions are cheap [6]. Rejected as over-engineering for one host:
- it duplicates state between the execution and DynamoDB;
- it needs task-token storage and `states:SendTaskSuccess` plumbing;
- it still can't detect a dead agent after "ready", so a reconcile is needed anyway.

### D. Per-start EventBridge Scheduler one-time timeouts

Rejected: the control plane would have to call `CreateSchedule`, which requires `iam:PassRole` [5], and that is on ADR-0002's "Never granted" list. Schedules created by CDK at deploy time (option B) pass the role through CloudFormation, not through the control plane.

### Signals, state and delivery alternatives (rejected)

- **Readiness from per-protocol probes in Lambda:** Hytale has no query protocol [21]. The probes would also need public query ports and per-game code.
- **`NetworkPacketsIn` alarms:** 5-minute resolution, and the count includes SSM, `dnf` and scanner traffic [11].
- **Host-to-control-plane transports other than EventBridge:**
  - SQS: a queue and a poller, with no fan-out to the notifier.
  - `PutMetricData` plus alarms: metrics rather than events, and no "ready" or "auth required" semantics.
  - Direct DynamoDB writes from the host: the host would write authoritative state.
  - EventBridge is chosen for fan-out, rule filtering and verified condition keys [1].
- **State in EC2 tags or SSM:** the control plane may write only the `game` tag key (ADR-0002 Grants), and neither EC2 tags nor SSM offers compare-and-swap.
- **Lambda reserved concurrency of 1 as the lock:** synchronous API calls would throttle instead of queueing, and status still needs storage.
- **Posting to Discord from the lifecycle function:** this ties lifecycle latency and failures to Discord.
- **Shipping the agent:**
  - a CDK S3 asset needs `s3:GetObject`, a second instance permission;
  - a public GHCR image plus `docker cp` adds steps and a registry dependency;
  - a custom AMI conflicts with ADR-0002's pinned stock AL2023.

## Decision

**Option B: event-driven lifecycle with one DynamoDB item as state and lease lock, a host agent that reports and powers off on idle, one custom EventBridge bus, a separate Discord notifier, and a 5-minute reconcile.**

### Components

| Component | What it is | Responsibility |
|---|---|---|
| `lifecycle` Lambda | Go, arm64, 128 MB, 30 s timeout | Only writer of lifecycle state and the only caller of EC2 and Route 53. Handles HTTP API requests (front door from 0006), EC2 state changes, host events, command events and the reconcile |
| `notifier` Lambda | Go, arm64, reserved concurrency 1 | Turns domain events into Discord webhook posts |
| `lifecycle` DynamoDB table | On-demand | One item per host (key `host#<instanceId>`) |
| `bradfordly-games` custom event bus | EventBridge, plus rules | Host events, command events and domain events |
| Reconcile schedule | EventBridge Scheduler, `rate(5 minutes)` | Invokes `lifecycle` with a reconcile command |
| `game-agent` | Go, linux/amd64, systemd unit on the instance | Readiness, activity and idle power-off |

The event types live in one shared Go package. It has three users (agent, lifecycle, notifier), which meets ADR-0001's rule of three. Configuration reaches every function through environment variables (12-factor), as ADR-0002's contract already lists: instance ID and ARN, zone ID, catalog path, record suffix, table and bus names, and the webhook parameter name.

### Lifecycle states

| State | Meaning |
|---|---|
| `STOPPED` | Instance stopped; no `game` tag and no A record |
| `STARTING` | `game` tag set and `StartInstances` accepted; booting, patching or loading |
| `READY` | The agent reported the game ready; the endpoint is published |
| `STOPPING` | A stop was requested, the host announced its own shutdown, or EC2 reported `stopping` |
| `ADMIN` | Instance running without a lifecycle start (console start, CloudFormation restart, admin session) |

Transitions:

- `STOPPED` → `STARTING`: Start.
- `STARTING` → `READY`: Game Ready.
- `STARTING`/`READY` → `STOPPING`: Stop, Host Shutting Down, or EC2 `stopping`.
- Any running state → `STOPPED`: EC2 `stopped`, after ADR-0002's cleanup.
- `STOPPED` → `ADMIN`: EC2 `running` with no `game` tag.
- On EC2 `running` during `STARTING`, the lifecycle reads the public IPv4 with `DescribeInstances` and UPSERTs `<id>.games.bradfordly.com` (A, TTL 60), as ADR-0002 requires. It records the IP it wrote.

The item also holds:

- the attempt: `serverId`, a ULID `startId`, `cause` and `requestedBy`;
- timing and status: `stateSince`, `readyAt`, `readyBasis`, `peers`, `idleDetection`, `lastHeartbeatAt`;
- DNS: the published IP and TTL;
- outcome: `stopReason` and `lastFailure`.

Every write is an `UpdateItem` on named attributes, never a whole-item `PutItem`, so a heartbeat can't overwrite a transition.

### Single-flight lock

ADR-0002 requires one lock shared by Start, Stop and the EC2 state-change handler. It is a lease on the host item:

- **Acquire:** a conditional `UpdateItem` that sets `lockOwner` (the request ID) and `lockExpiresAt` (now + 60 s), on condition `attribute_not_exists(lockOwner) OR lockExpiresAt < :now`. Start folds its state check into the same condition (`state = STOPPED`).
- **Release:** a conditional `UpdateItem` that writes the new state and removes the lock, on condition `lockOwner = :me`.
- The lease (60 s) outlives the function timeout (30 s), so a crashed holder blocks for at most a minute.
- Every side effect is idempotent: `CreateTags` with the same value, `StartInstances` on a running instance, Route 53 UPSERT, `DeleteTags` of an absent tag, and a DELETE of a record that no longer exists, which is treated as success.

Lock holders follow ADR-0002's order exactly:

- **Start:** `CreateTags game=<id>`, then `StartInstances`. If `StartInstances` fails, `DeleteTags game` and back to `STOPPED`.
- **Stop:** DELETE the A record, then `DeleteTags game`, then `StopInstances`. Never force, hibernate or terminate.
- **EC2 `stopping`/`stopped` handler:** takes the lock, re-checks with `DescribeInstances` that the instance is still stopping or stopped, then deletes the record and the tag.

Heartbeats update only `peers` and `lastHeartbeatAt`, with no lock, on condition `serverId = :id`.

### API

The routes below are transport-neutral. 0006 chooses the front door and passes a verified identity (`username`, `groups`). The handler enforces each route's level in code even if the front door also does. No route takes a request body, and there are no idempotency keys: requests are idempotent by state.

| Route | Level | Behaviour |
|---|---|---|
| `GET /status` | member | The host item: state, server, since, endpoint (hostname and public ports), approximate players, ready time, last failure |
| `GET /servers` | member | Known server ids (interim: the catalog) and which one is active |
| `POST /servers/{id}/start` | member | Start(id) |
| `POST /servers/{id}/stop` | member | Stop(id); stopping the active server stops the host |
| `POST /host/stop` | admin | Stop the host in any running state, including `ADMIN` |
| Create, edit, config and secrets routes | admin | Defined by 0004 |

Responses:

- **202** with the new status when a transition starts.
- **200** with the current status when the server is already in the requested state: Start while `STARTING`/`READY` for the same server, or Stop while `STOPPED`/`STOPPING`.
- **404** for an unknown server id.
- **409** with a reason: `other-server-active`, `admin-mode`, `stopping` or `busy` (lock held). `stopping` and `busy` include `Retry-After`.
- **502** with the EC2 error code when `StartInstances` fails (after the tag rollback).
- **403** when the caller lacks the route's group.

There is no HTTP front door until 0006. In the meantime the owner starts and stops servers with `aws events put-events` (commands, below), and reads status from Discord or the DynamoDB item.

### Commands

Asynchronous producers don't call the function; they put commands on the bus.

- **Detail-types:** `Start Requested` and `Stop Requested`, detail `{v, serverId, cause}`.
- **Sources:**
  - `bradfordly-games.cli` (now; only the owner's admin principal can put events on the bus, so this is admin level);
  - `bradfordly-games.wake` (0005);
  - `bradfordly-games.discord` (a possible future slash-command handler).
- **Semantics:** the same as the API. The outcome comes back as a domain event: `Server Starting`, `Server Stopped`, or `Request Rejected` with the 409 reason. Adding a producer takes one rule-pattern entry, one cause kind and the producer's own source-scoped `PutEvents` grant; nothing in the lifecycle changes.

### Reconcile (every 5 minutes)

The reconcile reads the item and `DescribeInstances`, and takes the lock only if it must act:

1. Item `STARTING`/`READY`/`STOPPING` but the instance is `stopped`: run the stopped handler. `Server Stopped` carries the recorded `stopReason`, or `external` if there is none (console, backstop timer, CloudFormation). A `STARTING` item is reset only once it is more than 2 minutes old, so it can't race a start that is still in flight.
2. Item `STOPPED` but the instance is running: `ADMIN` if there is no tag. With a valid `game` tag (for example a lost write), adopt the run as `STARTING` with cause `external`.
3. `STARTING` and the instance not running 10 minutes after Start: `Server Start Failed` (`ec2-timeout`), then Stop.
4. `STARTING`/`READY`, the instance running, and no host event for 20 minutes (counted from the later of EC2 `running` and the last heartbeat): the agent is presumed lost. Stop, then `Server Start Failed` or `Server Stopped` with reason `agent-lost`.
5. `STARTING` for more than 60 minutes: `Server Start Failed` (`start-timeout`), then Stop. This backs up the agent's 30-minute ready timeout and covers patching and a reboot.
6. `STARTING`/`READY` with a public IP different from the published one: UPSERT the record.

`ADMIN` without a `game` tag is never stopped by the control plane; ADR-0002's 4 h backstop owns it.

### Host agent

`game-agent` is a Go binary run by `game-agent.service`.

**Installation and lifetime:**

- It is installed and started early in every boot (amendment A3) and runs in both game and admin mode.
- It doesn't require `/srv`, so it can report a failed mount.
- It never starts or stops containers, never reads the Docker socket, and opens no listening ports.
- It runs as root because it needs conntrack netlink, the journal and `systemctl poweroff`. The first infra PR sets the systemd hardening options.
- Its only outward actions are `events:PutEvents` and `systemctl poweroff`.
- It uses the instance role through IMDS, which only host processes can reach. Containers are blocked from it by ADR-0002's hop limit of 1 and the `DOCKER-USER` drop rule, and host networking is forbidden, so no game process can use the new permission.

**Inputs:**

- The `game` tag from IMDS, validated with ADR-0002's regex.
- The catalog parameter, which the instance may already read.
- `systemctl is-active game@<id>`.
- `journalctl -b -f -o json CONTAINER_NAME=game-<id>` [20]. `-b` is enough because each boot serves one game.
- conntrack entries through netlink.
- Idle defaults from its unit's environment (12-factor): `IDLE_EMPTY_MINUTES=15`, `IDLE_NOJOIN_MINUTES=20`, `READY_TIMEOUT_MINUTES=30`. Per-server overrides are left to 0004.

**Required package fields** (requirements on 0004's schema):

| Field | Use | Interim (ADR-0002 catalog until 0004 lands) |
|---|---|---|
| `ports[]` with `exposure: public` | Ports whose flows are counted | Existing field |
| `query?` | Excluded from counting (server-list pings aren't players) | Existing field |
| `readyPattern` | Case-sensitive substring of a container log line meaning "ready" | New optional field (amendment A6). If absent, ready is reported as `unverified` 60 s after `game@<id>` becomes active |
| `authRequiredPattern?` | Log substring meaning game-account auth is needed | Not available until 0004 |
| `readyTimeoutSeconds?` | Overrides the 30-minute ready timeout | Not available until 0004 |

**Peers:** distinct remote addresses with a live conntrack entry whose original destination is a public, non-query port of the package: UDP entries that have seen a reply, and TCP entries that are `ESTABLISHED`. The agent samples every 30 s. This is an approximate player count. Friends behind one NAT count as one, and a scanner that gets replies counts until its entry expires.

| Situation | Agent action |
|---|---|
| Any mode, from agent start | `Host Heartbeat` every 5 min, plus immediately (at most once a minute) when peers change between zero and non-zero |
| `readyPattern` seen (or the interim fallback) | `Game Ready` |
| `authRequiredPattern` seen | `Game Auth Required`, at most once per 10 min |
| Not ready 30 min after `game@<id>` became active | `Host Shutting Down` (`ready-timeout`), then power off |
| Ready, and no peer within 20 min | `Host Shutting Down` (`no-join`), then power off |
| Peers zero for 15 min after at least one peer | `Host Shutting Down` (`idle`), then power off |
| `game@<id>` inactive for 2 min after having been active this boot (not during shutdown) | `Host Shutting Down` (`game-exited`), then power off |
| Fail-closed admin mode (reason read from `/run/bradfordly-games/admin-reason`, amendment A4) | `Host Admin Mode` once. No power-off: the lifecycle stops failed starts, and the 4 h backstop covers owner sessions |
| conntrack unreadable | `peers: null` and `idleDetection: false`. The no-join and idle rules are disabled because the agent never powers off on unknown activity. The ready-timeout and game-exited rules still apply, and the 12 h backstop bounds cost |
| `PutEvents` fails | Log and retry up to 3 times with backoff. It never delays a power-off |

Power-off is `systemctl poweroff`, which is ADR-0002's graceful path: `game@<id>` stops with its `docker stop` grace period. The resulting EC2 `stopping`/`stopped` events drive DNS and tag cleanup in the lifecycle exactly as ADR-0002 requires. ADR-0002's backstop units call `game-agent notify --reason backstop-admin|backstop-uptime` before powering off (best effort), so the stop reason is known.

**Failure modes:**

- **Agent crashes:** systemd restarts it (`Restart=on-failure`). Idle timers restart from zero, which delays shutdown by at most one window. If it stays dead, reconcile rule 4 stops the host within about 20–25 minutes.
- **Agent missing** (download or checksum failure at boot): the boot continues and the same rule applies.
- **No ready line:** the 30-minute ready timeout, with reconcile rule 5 as backup.
- **conntrack unavailable:** handled as in the table. This is a first-deploy test.

Host events are advisory. The lifecycle drops any host event whose `serverId` isn't the active server. It never starts the instance, tags it, or writes DNS because of a host event, so a compromised host can mislead status and notifications but can't start servers or redirect players.

### Events

All events go on the `bradfordly-games` custom bus except EC2's, which arrive on the default bus. Every detail carries `v: 1`, `instanceId`, `at` (RFC 3339) and, where it applies, `serverId`. Domain events also carry `startId`. Details stay well under 1 KB and never contain secrets or player IPs. Consumers ignore fields they don't know.

| Source | Detail-type | Type-specific detail | Producer | Consumers |
|---|---|---|---|---|
| `aws.ec2` (default bus) | `EC2 Instance State-change Notification` (`pending`, `running`, `stopping`, `stopped`) | AWS-defined | EC2 | lifecycle |
| `bradfordly-games.host` | `Host Heartbeat` | `mode` (`booting`/`game`/`admin`), `peers` (int or null), `ready` | agent | lifecycle |
| | `Game Ready` | `basis` (`pattern`/`unverified`), `bootSeconds` | agent | lifecycle |
| | `Game Auth Required` | `hint?` (≤200 chars; content decided by 0004) | agent | lifecycle |
| | `Host Admin Mode` | `reason` | agent | lifecycle |
| | `Host Shutting Down` | `reason` (`idle`, `no-join`, `ready-timeout`, `game-exited`, `backstop-admin`, `backstop-uptime`) | agent | lifecycle |
| `bradfordly-games.cli`, later `.wake` (0005) and `.discord` | `Start Requested`, `Stop Requested` | `cause` | owner CLI, 0005, future slash commands | lifecycle |
| `bradfordly-games.lifecycle` | `Server Starting` | `cause` `{kind: user\|cli\|lookup\|external, user?, name?}` | lifecycle | notifier |
| | `Server Ready` | `endpoint` `{hostname, ports[]}`, `basis`, `idleDetection` | lifecycle | notifier |
| | `Server Auth Required` | `hint?` | lifecycle | notifier |
| | `Server Start Failed` | `reason` (`ec2-error`, `admin-mode`, `ready-timeout`, `start-timeout`, `ec2-timeout`, `agent-lost`), `detail?` | lifecycle | notifier |
| | `Server Stopped` | `reason` (`requested`, `idle`, `no-join`, `ready-timeout`, `game-exited`, `start-failed`, `agent-lost`, `backstop-admin`, `backstop-uptime`, `external`), `uptimeSeconds` | lifecycle | notifier |
| | `Request Rejected` | `command`, `reason`, `cause` | lifecycle | none now (0005 or slash commands may subscribe) |

How the lifecycle reacts to host events:

- **Game Ready:** valid only during `STARTING` for the same server. Moves to `READY` and emits `Server Ready`.
- **Game Auth Required:** emits `Server Auth Required`, at most once per 10 min per start.
- **Host Admin Mode during a requested start:** emits `Server Start Failed` (`admin-mode`, with the launcher's reason) and runs Stop. Previously the host sat in admin mode until the 4 h backstop. Outside a requested start, the reason is only recorded.
- **Host Shutting Down:** moves to `STOPPING` and records the reason. `ready-timeout` also emits `Server Start Failed`.

Only the lifecycle's own events reach the notifier, so stale or duplicate host events never become messages. The UI (0006) reads state through `GET /status` by polling; it doesn't consume events.

Rules:

- default bus, `aws.ec2` state changes for this instance → `lifecycle`;
- custom bus, source `bradfordly-games.host` → `lifecycle`;
- custom bus, command detail-types from allow-listed sources → `lifecycle`;
- custom bus, source `bradfordly-games.lifecycle` → `notifier`.

The lifecycle also checks the (source, detail-type) pair in code before acting. Target retry policies are capped at a 1 h maximum event age, with no DLQs: the reconcile repairs state, and notifications are best-effort.

### Discord notifier

- **Webhook URL storage:** a SecureString parameter `/bradfordly-games/control/discord-webhook-url` (standard tier, `aws/ssm`).
  - The owner creates it with the CLI, because CloudFormation can't create SecureStrings [14].
  - The `DISCORD_WEBHOOK_PARAM` environment variable names it.
  - The notifier caches the value in memory for 15 min.
  - If the parameter is missing, notifications are disabled with a log warning.
  - Secrets Manager would cost $0.40 per month for one URL [16]; a plaintext Lambda environment variable would show in the function configuration.
  - Rotation: create a new webhook in Discord, overwrite the parameter, delete the old webhook.
- **Messages:** built only from event fields, with `allowed_mentions: {"parse": []}` [17] and hints truncated to 200 characters.

| Owner's message | Event |
|---|---|
| starting | `Server Starting` with cause `user`, `cli` or `external` ("Starting X, requested by …") |
| lookup detected | `Server Starting` with cause `lookup` ("Lookup detected for X.games.bradfordly.com; cold start underway") |
| ready | `Server Ready` (hostname and ports; "readiness unverified" or "idle detection unavailable" when flagged) |
| stopped (idle) | `Server Stopped` with reason `idle` or `no-join` |
| stopped | `Server Stopped` with any other reason |
| auth required | `Server Auth Required` |
| start failed | `Server Start Failed` |

**Rate limits and failure isolation:**

- Reserved concurrency of 1 keeps posts serial. Throttled asynchronous invocations queue and are retried [9], and the 1 h maximum event age caps the wait.
- **429:** honour `retry_after`. Retry once in-function if the wait is 5 s or less; otherwise return an error so Lambda's two retries apply.
- **5xx or network errors:** return an error; after the retries the message is discarded.
- **401, 403 or 404:** log at error level and don't retry. Repeated invalid requests risk Discord's temporary ban, and a 404 means the webhook is gone [18].
- The lifecycle publishes domain events after its state write and only logs a `PutEvents` failure. It never calls Discord and never waits for the notifier, so a notification failure can't block or roll back a lifecycle action.

### IAM

**`lifecycle` role:**

- ADR-0002's Grants, unchanged, with conditions exactly as listed there:
  - `ec2:StartInstances` and `ec2:StopInstances` on the instance ARN;
  - `ec2:DescribeInstances`;
  - `ec2:CreateTags` and `ec2:DeleteTags`;
  - `ssm:GetParameter` and `ssm:GetParametersByPath` on the catalog path;
  - `route53:ChangeResourceRecordSets` on the zone.
- Additions, all internal to the control plane:
  - `dynamodb:GetItem` and `dynamodb:UpdateItem` on the `lifecycle` table ARN;
  - `events:PutEvents` on the bus ARN, conditioned to source `bradfordly-games.lifecycle` and the six domain detail-types, with Null checks;
  - writes to its own log group (14-day retention).

**`notifier` role:**

- `ssm:GetParameter` on the webhook parameter's ARN only, plus its log group.
- First-deploy test: if `aws/ssm` decryption fails without an IAM statement [13], add `kms:Decrypt` on the `aws/ssm` key ARN with `kms:ViaService = ssm.us-east-1.amazonaws.com` and `kms:EncryptionContext:PARAMETER_ARN` set to that parameter.

**Scheduler execution role:** `lambda:InvokeFunction` on the `lifecycle` function. Its trust policy is limited to `scheduler.amazonaws.com` with `aws:SourceAccount`. CloudFormation passes this role at deploy time; the control plane never does.

**Instance role** (the one new permission, amendment A2):

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

The agent sends one entry per call. Whether `StringEquals` or `ForAllValues:StringEquals` is the correct operator depends on how PutEvents presents these keys (unverified). The first deploy proves that an allowed event succeeds and that a wrong source, a wrong detail-type and a different bus are all denied. If that can't be expressed, the fallback is a host-only bus (see Revisit).

**Not needed:** nothing on ADR-0002's "Never granted" list.

- Timeouts use the deploy-time schedule instead of per-start schedules, which avoids `iam:PassRole` [5].
- Readiness is pushed by the agent instead of pulled with `ssm:SendCommand`.
- No `ec2:ModifyInstanceAttribute`, `RunInstances`, `TerminateInstances`, volume, snapshot or instance-profile actions are used.
- No `events:PutRule`, `PutTargets`, `scheduler:*`, `iam:*` or `lambda:*` permissions are granted to any runtime role.

**Left for 0004 to change:** ADR-0002's Grants enumerate catalog ids in `aws:RequestTag/game` and enumerate the game record names in `…NormalizedRecordNames`. Runtime-created servers need both changed (for example to `ForAllValues:StringLike` [25] with an explicit Deny on reserved names). This ADR keeps them as written. 0005 owns any change to the UPSERT/DELETE semantics, and 0006 owns the front door's invoke permission.

### Interfaces for later ADRs

- **0004:**
  - Supply the package fields in the table above.
  - Replace the one server-lookup function in the lifecycle, which resolves an id to {exists, hostname, public ports} and currently reads `/bradfordly-games/games/<id>`.
  - Keep ids matching `^[a-z0-9][a-z0-9-]{0,31}$`.
  - Define the admin routes.
  - If package or server parameters move, change the instance's `GetParameter` path.
  - Decide per-server idle overrides and the content of `hint`.
- **0005:**
  - Emit `Start Requested` with source `bradfordly-games.wake` and cause `{kind: lookup, name}`, using its own source-scoped `PutEvents` grant and one rule-pattern entry.
  - Decide abuse limits and where they are enforced.
  - Amend the two DNS write points in the lifecycle (on `running`; on stop or `stopped`) for placeholder records.
  - It may subscribe to `Request Rejected`.
- **0006:**
  - Provide the front door that invokes `lifecycle` with a verified `{username, groups}`.
  - Map `admin` and member to the route levels above.
  - Poll `GET /status`.
- **Future slash commands:** another command producer (`bradfordly-games.discord`) and a consumer of domain events for follow-ups. No lifecycle rework.

### Cost

Unit prices are verified [3][7][8][10][12][15]; the volumes are estimates.

| Item | Idle month (no sessions) | One 2-hour session |
|---|---|---|
| Lambda arm64, 128 MB (8,640 reconcile runs ≈ 216 GB-s; ~40 invocations per session) | $0 in the free tier; about $0.005 without it | < $0.001 |
| DynamoDB on-demand (about 8,640 reads idle; about 60 writes per session; item well under 1 KB) | about $0.001 | < $0.001 |
| EventBridge custom bus (about 30 host events + 10 domain events per session) | $0 | < $0.001 |
| EventBridge Scheduler (8,640 invocations) | $0 (14M free) | $0 |
| CloudWatch Logs (about 5 MB/month, 14-day retention) | $0 in the 5 GB free tier; about $0.003 without it | negligible |
| SSM SecureString (`aws/ssm`) and KMS decrypts | $0 | a few decrypts, inside 20,000 free |
| Discord webhook | $0 | $0 |
| **Total** | **about $0.00 ($0.01 without free tiers)** | **< $0.01** |

ADR-0002's $1.26–1.76 of headroom is effectively untouched and remains available to 0004–0006. Idle shutdown saves EC2 time:

- A session that would previously have run to the 12 h backstop now ends about 15 minutes after the last player leaves, saving up to about $1.07 (11.75 h × $0.091).
- A start nobody joins ends about 20 minutes after "ready".

## Amends ADR-0002

Each item names the ADR-0002 section, the old rule and the new rule. ADR-0002 should get "Amended by 0003" in its status line in the same PR.

- **A1. Contract with ADR-0003 → Signals.**
  - Old: "no on-host player-count signal in v0… an on-host signal would amend this contract."
  - New: a host agent (`game-agent`) reports heartbeat (approximate peers from conntrack), readiness, auth required, admin mode and its own shutdowns as events on the `bradfordly-games` bus, and powers the host off on idle. Host events are advisory: the control plane never starts, tags or writes DNS because of one.
- **A2. Instance IAM.**
  - Old: Session Manager minimum plus `ssm:GetParameter` on the catalog path; "Nothing else".
  - New: also `events:PutEvents` on the `bradfordly-games` bus only, conditioned on source `bradfordly-games.host` and the five host detail-types, with Null checks. Still no EC2, Route 53, S3 or CloudWatch Logs permissions.
- **A3. Server-plane invariants → 1. Boot sequence.**
  - Old: mount, patch, maybe reboot, Docker, IMDS rule, launcher.
  - New: after the mount attempt and before patching:
    - if the pinned `game-agent` binary isn't on the root volume, download it from this repo's GitHub Release over HTTPS and verify its SHA-256, which is pinned in user data;
    - start `game-agent.service`, which doesn't require `/srv`;
    - a failure is logged and non-fatal.
  - Changing the pin changes user data, which restarts the instance [ADR-0002 14], so agent upgrades follow the "deploy, then stop" runbook.
- **A4. Server-plane invariants → 2. Fail closed to admin mode.**
  - Old: the listed checks drop the host into admin mode.
  - New: unchanged checks, and the launcher also writes the failed check's name to `/run/bradfordly-games/admin-reason` (tmpfs, root-owned, no secrets) for the agent to report.
- **A5. Server-plane invariants → 7. Backstop timers.**
  - Old: power off after 4 h of admin mode with no game unit, and after 12 h of uptime.
  - New: both backstops are unchanged, and the backstop units first run `game-agent notify` (best effort). In addition the agent powers off:
    - 15 min after peers drop to zero (once there was at least one peer);
    - 20 min after ready with no peer;
    - 30 min after `game@<id>` starts without ready;
    - 2 min after `game@<id>` stops unexpectedly.
  - It never powers off on unknown activity or in admin mode.
- **A6. Game catalog → value shape.**
  - Old: the listed fields.
  - New: plus an optional `readyPattern` (1–200 printable ASCII characters, case-sensitive substring, asserted at synth). This is interim: 0004 replaces the catalog and carries the requirement forward.
- **A7. Contract → Single-flight lifecycle.**
  - Clarified: the lock is the DynamoDB lease above, shared by Start, Stop, the EC2 handler and the reconcile.
  - New behaviour: when the launcher fails closed during a requested start, the control plane stops the instance (`Server Start Failed`) instead of leaving it to the 4 h backstop. An admin session started without a `game` tag is unaffected.
- **A8. Contract → Grants.**
  - Old: the listed server-plane grants.
  - New: unchanged, extended with control-plane-internal grants (the DynamoDB table, source-scoped `PutEvents`, the Scheduler target role). Nothing from "Never granted" is used.
- **A9. Consequences → Revisit if.** The trigger "ADR-0003 needs an on-host player-count or readiness signal: amend the contract" is fulfilled by A1–A5.

Not amended here, left to later ADRs:

- **0004:** the enumerated tag values and record names in Grants, the secrets model, the launcher's directory rule.
- **0005:** "the A record exists only while the instance runs" and Stop's DELETE.

## Consequences

Positive:

- Control-plane idle cost is about $0, so ADR-0002's headroom survives for 0004–0006.
- Idle shutdown happens on the host and works even when the control plane is down. The 4 h and 12 h backstops still bound everything else.
- One writer of state, idempotent operations, and a reconcile that repairs lost events, crashed handlers and a dead agent within about 25 minutes.
- Game-agnostic: readiness is a log substring and activity is conntrack on declared ports. No per-game code in the control plane, and no game query protocol is needed (Hytale has none).
- The new instance permission is narrow and unusable from containers. Host events can't start servers, tag the instance or touch DNS.
- Discord failures are isolated from the lifecycle. 0005, 0006 and future slash commands plug in as producers and consumers without lifecycle rework.

Negative:

- A new on-host component to build, release, test and patch. Agent upgrades change user data and restart the instance.
- A compromised host can forge or spam host events: it can mislead status and notifications and incur $1 per million events. That is bounded but real.
- The player count is approximate. A scanner that gets replies can postpone idle shutdown (bounded by 12 h), and friends sharing an IP count as one.
- Readiness depends on log text the game vendor can change. A missed pattern costs a 30-minute ready timeout and a failed start, not a stuck host.
- Failures that only the reconcile detects take up to about 25 minutes to surface.
- Until 0006, starts and stops go through the AWS CLI.
- Three facts are unverified until the first deploy: conntrack visibility on AL2023, the PutEvents condition operator, and whether `aws/ssm` decryption needs an IAM statement.

Revisit if:

- conntrack is unavailable or too noisy on AL2023: add per-package join/leave log patterns (0004) or per-protocol probes.
- The PutEvents conditions can't express the restriction: move host events to a dedicated bus and scope the instance by resource only.
- A second host or concurrent servers appear: the item is already keyed per host, so add a lock per host and a host id in the API.
- Discord rate limits bite or members want commands: write an interactions-endpoint ADR (Ed25519 verification, 3 s deadline [19]).
- The UI needs history or an audit trail: add an EventBridge archive or an event log table.
- Agent supply-chain requirements grow: sign releases or add build attestations, or switch to an S3 asset with a scoped `s3:GetObject`.
- Free tiers lapse: the estimate is still about $0.01/month.

## Follow-ups (not part of this ADR)

1. **Infra PR for this ADR:** the bus and rules, the table, the two Lambdas, the reconcile schedule, the agent unit, and a release workflow for `game-agent` (static linux/amd64 build, SHA-256 published, third-party actions pinned to commit SHAs per CLAUDE.md). Expected cdk-nag suppressions: DynamoDB point-in-time recovery (AwsSolutions-DDB3; the state can be rebuilt from EC2 by the reconcile) and IAM5 for `ec2:DescribeInstances` on `*`.
2. **First-deploy tests:**
   - conntrack entries are visible to the agent for a UDP and a TCP flow to a published port;
   - an allowed host event succeeds, and a wrong source, a wrong detail-type and another bus are denied;
   - a container still can't get an IMDSv2 token;
   - `journalctl CONTAINER_NAME=game-<id>` shows the game's lines;
   - the notifier reads the SecureString with `ssm:GetParameter` alone, or `kms:Decrypt` is added as scoped above;
   - idle, no-join and ready-timeout power-offs each produce the right DNS and tag cleanup and the right Discord message;
   - stopping the agent leads to a reconcile stop (`agent-lost`);
   - two concurrent Starts produce one 202 and one 200 or 409.
3. **Runbooks:**
   - create and rotate the webhook parameter;
   - start and stop with `aws events put-events` (source `bradfordly-games.cli`);
   - upgrade the agent (deploy, then stop);
   - `systemctl stop game-agent` during hands-on admin work, so the agent doesn't power off mid-edit.
4. Measure boot-to-ready time and agent CPU in the first sessions, and tune the 30-minute ready timeout.
5. ADR-0002 status line "Amended by 0003" (same PR).

## Sources

All accessed 2026-10-12.

1. EventBridge actions and condition keys (China-partition Service Authorization Reference) — https://docs.amazonaws.cn/service-authorization/latest/reference/list_amazoneventbridge.html
2. EventBridge IAM conditions — https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-use-conditions.html
3. EventBridge pricing (including Scheduler) — https://aws.amazon.com/eventbridge/pricing/
4. EventBridge retry policy — https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-rule-retry-policy.html
5. EventBridge Scheduler permissions — https://docs.aws.amazon.com/scheduler/latest/UserGuide/security_iam_id-based-policy-examples.html ; execution role — https://docs.aws.amazon.com/scheduler/latest/UserGuide/setting-up.html
6. Step Functions pricing — https://aws.amazon.com/step-functions/pricing/
7. DynamoDB on-demand pricing — https://aws.amazon.com/dynamodb/pricing/on-demand/
8. Lambda pricing — https://aws.amazon.com/lambda/pricing/
9. Lambda asynchronous error handling — https://docs.aws.amazon.com/lambda/latest/dg/invocation-async-error-handling.html
10. CloudWatch pricing — https://aws.amazon.com/cloudwatch/pricing/
11. EC2 CloudWatch metrics — https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/viewing_metrics_with_cloudwatch.html
12. Systems Manager pricing — https://aws.amazon.com/systems-manager/pricing/
13. KMS encryption for SecureString parameters — https://docs.aws.amazon.com/kms/latest/developerguide/services-parameter-store.html
14. `AWS::SSM::Parameter` — https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ssm-parameter.html
15. AWS KMS pricing — https://aws.amazon.com/kms/pricing/
16. Secrets Manager pricing — https://aws.amazon.com/secrets-manager/pricing/
17. Discord webhooks — https://docs.discord.com/developers/resources/webhook
18. Discord rate limits — https://docs.discord.com/developers/topics/rate-limits
19. Discord interactions — https://docs.discord.com/developers/interactions/overview ; https://docs.discord.com/developers/interactions/receiving-and-responding
20. Docker journald logging driver — https://docs.docker.com/engine/logging/drivers/journald/
21. Hytale Server Manual — https://support.hytale.com/hc/en-us/articles/45326769420827 (read through the Help Center API, https://support.hytale.com/api/v2/help_center/en-us/articles/45326769420827.json; article updated 2026-06-15)
22. Pelican eggs — https://pelican.dev/docs/eggs/creating-a-custom-egg ; Hytale egg — https://raw.githubusercontent.com/pelican-eggs/games-standalone/main/hytale/egg-hytale.yaml
23. itzg/minecraft-server AUTOSTOP — https://docker-minecraft-server.readthedocs.io/en/latest/misc/autopause-autostop/autostop/
24. doctorray117/minecraft-ondemand — https://github.com/doctorray117/minecraft-ondemand
25. Route 53 IAM conditions — https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/specifying-rrset-conditions.html
26. Repository — https://github.com/Bradfordly/bradfordly-games
