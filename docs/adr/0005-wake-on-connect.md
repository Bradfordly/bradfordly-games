# 0005. Wake on connect

- Status: Proposed
- Date: 2026-10-16

## Context

Issue #18. ADR-0002 (server plane), ADR-0003 (control-plane core) and ADR-0004 (packages) are Proposed. The parts this ADR builds on:

- **ADR-0002** creates the public zone `games.bradfordly.com` in CDK. Only the control plane writes the per-game A records `<id>.games.bradfordly.com` (TTL 60), and "the A record exists only while the instance runs".
- **ADR-0003** runs:
  - one lifecycle item with a lease lock;
  - a 5-minute reconcile in the `events` Lambda;
  - an 80 h/month run-hours cap (O2);
  - Discord notifications through an incoming webhook.

  Its hand-off to this ADR:
  - a `Start Requested` producer with source `bradfordly-games.wake` and cause kind `lookup` (member level, can't override the cap);
  - abuse limits. Sustained fake peers can hold every wake to the 12 h 15 min bound, so about 7 hostile wakes exhaust the 80 h cap: an availability denial of service;
  - the DNS write points for placeholder records, if any;
  - optionally, `Request Rejected` as a Discord input;
  - "Left to later ADRs → 0005": "the A record exists only while the instance runs" and Stop's DELETE.
- **ADR-0004** makes the server id the package id: one server per game, and only one running at a time. Record names are generated from the `catalog/` ids.

This ADR decides how a DNS lookup of a stopped server's hostname starts it:

- what DNS answers while the server is stopped;
- how lookups are read;
- the abuse and cost limits;
- the Discord messages;
- the amendments to ADR-0002's DNS contract and to ADR-0003's wake interface.

Owner decisions (authoritative):

- (2026-10-12) Cost first: total idle cost stays under $5/month, and ADR-0002 leaves $1.26–1.76 of headroom. Long cold starts are acceptable. At most 10 players.
- (2026-10-12) The wake trigger is a Route 53 DNS-lookup trigger (query logs), not an always-on listener. Messages say "lookup detected", never "login attempted".
- (2026-10-13) Lookup wakes are member level and never override the 80 h cap (ADR-0003).
- (2026-10-16) **W1 Trigger:** pull. The existing 5-minute reconcile reads the query log with `FilterLogEvents`. There is no `wake` function, no `.wake` producer and no extra reserved concurrency.
- (2026-10-16) **W2 Stopped DNS:** NXDOMAIN, with no record while stopped (ADR-0002's rule is kept). The SOA record's TTL and its negative-caching minimum are both 60 s, set once by a CLI UPSERT.
- (2026-10-16) **W3 Run-hours:** lookup wakes are refused once the month reaches 60 run-hours. Manual starts keep the full 80 h. Confirmed after review: the threshold is checked only at start, so a lookup run that fake peers hold can end as late as 72 h 15 min. Members then keep at least 7 h 45 min, and about 20 h when lookup runs end normally.
- (2026-10-16) **W4 Limits:** at most 3 lookup wakes per UTC day, and none within 30 minutes of any stop.
- (2026-10-16) **W5 Scanners:** no extra filter in v0. Only A and AAAA queries for exact names of existing servers count, matched case-insensitively. Measure the noise and state a revisit trigger.
- (2026-10-16) **W6 Discord:** refused lookup wakes are posted at most once per refusal reason per 6 h.
- (2026-10-16) **W7 Kill switch:** a `WAKE_ENABLED` environment variable on the function that runs the pull, set by deploy. Edit it in the console in an emergency, and redeploy to persist the change.

Verified facts (as of 2026-10-16, us-east-1 unless noted; "(unverified)" marks the rest):

- **Query logging setup** [1][2][3][4]:
  - The log group must be in us-east-1, in the same account as the zone. AWS recommends the prefix `/aws/route53/`.
  - Route 53 needs a CloudWatch Logs resource policy that lets it create log streams and put log events.
    - `aws:SourceArn` (the zone ARN) and `aws:SourceAccount` can scope that policy.
    - An account can have at most 10 Logs resource policies per region [10].
  - Each zone can have one logging config. Route 53 creates one log stream per edge location, named `<zoneId>/<edge>`.
  - On `AWS::Route53::HostedZone`, `QueryLoggingConfig` updates with no interruption. CDK's `PublicHostedZone` takes `queryLogsLogGroupArn`.
  - Route 53 charges nothing for query logging; the charges are CloudWatch's.
- **What is logged** [1][2]:
  - **Fields:** each line holds the format version, query timestamp, zone ID, query name, query type, response code, protocol, edge location, resolver IP, and the EDNS client subnet ("if available from the DNS resolver") or `-`.
  - **Example lines:** AWS's examples show names without a trailing dot, and NXDOMAIN answers being logged.
  - **Only queries that resolvers forward to Route 53 are logged.** A resolver that has cached an answer doesn't query again until its TTL expires.
  - **Delivery latency:** AWS says logs appear "within several minutes" of creating a config. Per-query delivery latency isn't documented; prior art reports up to about 30 s (unverified) [17].
  - **Letter case:** whether a logged name keeps the case the resolver sent is undocumented (unverified). Google Public DNS randomises the letter case of more than 90% of its UDP queries to authoritative servers [15].
  - **Query names are attacker-chosen:** anyone can send any name of up to 255 octets, and a label may hold any byte [21]. How Route 53 logs spaces or non-printable bytes in a name is undocumented (unverified).
- **Negative caching** [5][6][7]:
  - The default SOA is `… 1 7200 900 1209600 86400`, with a record TTL of 900 s.
  - Resolvers cache NXDOMAIN and NODATA for the lesser of the SOA TTL and the SOA minimum: 900 s by default.
  - Both values can be changed by editing the SOA record. Negative answers are billed as standard queries.
  - Record changes "generally propagate to all Route 53 name servers within 60 seconds".
  - CDK's `PublicHostedZone` has no SOA property. `AWS::Route53::RecordSet` lists `SOA` as a type, but whether it can take over a zone's existing SOA is unverified.
- **Prices** [8][9]:
  - Route 53 query logs are vended logs: $0.50/GB ingested for the first 10 TB, and $0.03/GB-month stored (Price List offer effective 2026-10-01).
  - CloudWatch Logs has 5 GB/month free (ADR-0003 [13]).
  - Route 53 queries cost $0.40 per million (ADR-0002 [11]).
- **Reading the log** [10][11][12][13]:
  - **`FilterLogEvents` parameters:**
    - `startTime` and `endTime` are in ms on the event timestamp;
    - the filter pattern is at most 1,024 characters;
    - `limit` goes up to 10,000.
  - **Pages and order:** pages may be partly full or empty while a `nextToken` remains, and results are sorted by event timestamp. `startFromHead=false` returns the newest events first; it needs a `startTime` on or after 2024-01-01.
  - **Quota:** 25 calls per second in us-east-1.
  - **Filter patterns:**
    - they are case-sensitive;
    - regex goes between `%…%`, with character classes and `|` but no parentheses;
    - a space-delimited pattern allows at most 2 regexes.
  - **IAM:** `logs:FilterLogEvents` acts on the `log-group` resource, `arn:${Partition}:logs:${Region}:${Account}:log-group:${LogGroupName}`, and has no condition keys of its own.
  - **Cost:** the CloudWatch pricing page lists no charge for `FilterLogEvents` calls. That no charge exists is unverified.
- **Lambda logging default** [20]: the managed policy `AWSLambdaBasicExecutionRole` allows `logs:CreateLogGroup`, `logs:CreateLogStream` and `logs:PutLogEvents` on `*`. aws-cdk-lib 2.273.0 attaches it to any `lambda.Function` created without a `role` (checked in `infra/node_modules`).
- **Clients:**
  - Java caches failed lookups for 10 s by default [16].
  - Windows' client negative-cache limit, and whether it honours the SOA value, are unverified.
  - Whether the Hytale client joins by hostname, and whether its saved-server list resolves names when opened, are unverified (first-deploy tests).
- **Prior art:**
  - doctorray117/minecraft-ondemand uses query logs, a subscription filter and a Lambda, with a placeholder A record (TTL 30) while stopped. It says any lookup starts the server, including a friend's client scanning a server list that contains it, and it has no abuse controls [17].
  - lazymc wakes on connection through an on-host proxy, so it needs an always-on host [18].

## Options considered

All options keep ADR-0003's lifecycle, lease lock and Start semantics. None adds meaningful idle cost.

### A. Simplest viable option: pull with defaults

The reconcile reads the query log and calls Start with cause `lookup`. There is no record while stopped, the SOA keeps its default, and there are no limits beyond ADR-0003's (single-flight Start, the 80 h cap, the 12 h backstops).

Rejected:

- **Default SOA:** a player's resolver keeps NXDOMAIN for up to 15 minutes after their first lookup, so a server that starts quickly stays invisible to them.
- **No wake limits:**
  - every scanner or link-preview lookup starts a run of about 0.5 run-hours (about $0.05; estimate), bounded only by the 80 h cap;
  - about 7 hostile wakes held by fake peers lock members out for the month (ADR-0003).

### B. A plus limits and a 60 s negative TTL (chosen)

The same pull, plus:

- the SOA at 60 s;
- a cooldown after stops;
- a daily cap;
- a lower monthly threshold for lookup wakes;
- throttled Discord notices;
- a kill switch.

Details under Decision.

### C. Push: a subscription filter and a `wake` Lambda (ADR-0003's planned interface)

A subscription filter sends matching lines to a new function, which CloudWatch Logs invokes asynchronously [14][19]. That function puts `Start Requested` from `bradfordly-games.wake`. It would react within about a minute (unverified), against 0–5 minutes for B.

Rejected (W1). It adds all of these to save a few minutes on a cold start the owner already accepts as long:

- a function and its role;
- a subscription and an invoke permission;
- async settings;
- a bus-policy producer and a rule entry;
- a third reserved concurrency, which needs an account quota of at least 103.

B adds two IAM statements and one reconcile rule, and a query flood can't become a flood of invocations.

### D. Placeholder A record while stopped

As in minecraft-ondemand [17]: a TEST-NET address (`192.0.2.1`) with TTL 60, replaced at start.

Rejected (W2):

- The name always looks live, to scanners and passive DNS alike.
- Clients wait for a connection timeout instead of failing fast with "unknown host".
- It changes ADR-0002's record rule, and ADR-0003's Stop, its `stopping`/`stopped` handlers and reconcile rule 7. The 60 s SOA gives the same benefit without any of that.

### Other alternatives (rejected)

- **Wildcard or CNAME placeholders:** every label resolves, which invites more noise.
- **The SOA through a CDK custom resource:** no manual step, but it adds a deploy-time Lambda with Route 53 write rights, for a value set once. CDK can't own the SOA directly [4][7].
- **A metric filter with an alarm per game:** a custom metric and an alarm for each game, 1–3 minutes of alarm delay and an SNS hop. That is more parts than reading the log.
- **CloudTrail:** it records Route 53 API calls, not DNS queries. Query logging is how Route 53 exposes queries [1].
- **Route 53 Resolver query logs:** they cover queries made from VPCs, not queries to a public zone.
- **An always-on listener** (a lazymc-style proxy [18] or a small instance): the owner rejected it. A public IPv4 address alone costs $3.65/month (ADR-0002 [2]).
- **A start button only:** that belongs to 0006's UI or future Discord commands, and it complements wake.
- **A shorter no-join timeout for lookup-started runs:**
  - it saves about 10 minutes of EC2 (about $0.015) per false wake;
  - the host would need to know the cause (a new tag key, which the control plane may not write), or the reconcile would need another rule;
  - YAGNI.
- **Resolver or client-subnet allowlists:** the client subnet is optional for each resolver [1], home addresses change, and storing player subnets has a privacy cost.
- **Requiring a second lookup, or hard-to-guess labels:** deferred to "Revisit if" (W5). Labels are permanent package ids (ADR-0004), and every id is public in this repo, so hard-to-guess labels would only help as a secret suffix kept out of it.
- **A log-volume or `DNSQueries` alarm:** for the same flood, Route 53's query charges are 2–5× the log charges, depending on name length, and they exist without this ADR. An alarm wouldn't stop either. ADR-0002's free budget alert stays the slow cost signal, and the flood runbook can stop ingestion (Follow-ups).
- **A read checkpoint on the item:** it would still need overlap for late delivery, and then a dedupe rule. A fixed window with idempotent effects is simpler (see "Why a window and no checkpoint").

## Decision

**Option B.**

- While a server is stopped, its name answers NXDOMAIN, which resolvers cache for 60 s.
- Route 53 logs every query that reaches it.
- Every 5 minutes, the reconcile reads the last 15 minutes of A and AAAA lookups for existing servers' names. If the limits allow, it starts the looked-up server with cause `lookup`.

### DNS while stopped

- **Record rule:** ADR-0002's rule is kept. `<id>.games.bradfordly.com` exists only while the instance runs, and while stopped the name answers NXDOMAIN.
- **SOA:** the record TTL and the minimum are both 60, so resolvers cache NXDOMAIN for 60 s [5].
  - **Who sets it:** the owner, once, after the zone exists, with a CLI UPSERT (Follow-ups, runbooks). CDK doesn't own the SOA [4].
  - **Who can change it:** only the owner. The control plane's Route 53 grant is limited to `RecordTypes ["A"]` (ADR-0002).
  - **Verification and drift:**
    - the deploy preflight (ADR-0003 follow-up 1) reads the SOA with `ListResourceRecordSets` and fails the deploy if its TTL or minimum isn't 60, naming the runbook;
    - it skips the check if the zone doesn't exist yet;
    - a first-deploy test also confirms the value with `dig`.
  - **Why a deploy-time check is enough:** the SOA changes only through a manual edit or a new zone, and the zone is retained with a pinned logical ID (ADR-0002).
- **The A record** is still UPSERTed at EC2 `running`, and deleted at Stop and on EC2 `stopping`/`stopped` (ADR-0003). Players may therefore resolve the IP before the game is ready. Discord's "ready" message says when to join.

One wake, end to end (estimates; the first deploy measures them):

| Step | Time |
|---|---|
| The player's resolver gets NXDOMAIN and caches it for 60 s | At the lookup |
| The log line reaches CloudWatch Logs | About 30 s later (unverified) |
| The next reconcile run reads it and calls Start | 0–5 min later |
| EC2 `running`; the A record is UPSERTed and propagates | About 1–2 min later [6] |
| Boot, patching and game load (ADR-0002, ADR-0004) | Minutes, per game |
| A player who retries sees the record | Within 60 s of it existing, plus any client cache (Java: 10 s [16]) |

### Query logging (CDK)

| Resource | Setting |
|---|---|
| Log group | `/aws/route53/games.bradfordly.com` in us-east-1, retention 3 days, default encryption |
| Logs resource policy | Principal `route53.amazonaws.com`. Actions `logs:CreateLogStream` and `logs:PutLogEvents`, on `arn:aws:logs:us-east-1:<account>:log-group:/aws/route53/games.bradfordly.com:*`. Conditions `StringEquals aws:SourceAccount` = the account and `ArnLike aws:SourceArn` = `arn:aws:route53:::hostedzone/*` [2] |
| Zone | ADR-0002's retained zone gets `queryLogsLogGroupArn` (an in-place update [3][4]) and a dependency on the policy |

- **Why the wildcard zone ARN:** the policy must exist before the zone's logging config. Naming this zone's ID in the policy would make the policy depend on the zone, a cycle.
  - `aws:SourceAccount` is the effective guard. It blocks other accounts, and the log group must be in the zone's account anyway [2].
  - Inside the account, another zone's logging config could point at this group (the parent `bradfordly.com` zone lives here). Rule 10's zone-ID check drops those lines, but they are still read and billed.
- **Why 3 days:** the pull needs only 15 minutes. Three days leaves time to investigate a false wake without keeping resolver IPs and client subnets for long.
- **Privacy:** those two values never leave the log group.
  - The wake step logs only aggregates: one line per run for each server id and decision, with a count and the newest query time. It never logs a raw line.
  - The item, events and Discord never carry them.
- **Preflight:** also check that the account has fewer than 10 Logs resource policies [10].

### The wake step (reconcile rule 10)

It runs at the end of every reconcile run while `WAKE_ENABLED` is `true`, after rules 0–9.

1. **Read.** Call `FilterLogEvents` on the log group [12]:
   - `startTime` = now − `WAKE_WINDOW_MINUTES` (15), `endTime` = now;
   - newest first (`startFromHead=false` [12]);
   - the filter pattern below;
   - at most 5 calls and 10 s per run. Empty pages that still carry a `nextToken` are normal [12].
   - If either budget runs out, log `wake-truncated` and decide on what was read. The next run reads its own window again, so under sustained volume the oldest unread lookups are missed, never the newest.
2. **Pattern.** It is generated at synth from the `catalog/` ids as a space-delimited pattern with one regex.
   - Character classes make it case-insensitive, because patterns are case-sensitive and resolvers may randomise case [11][15].
   - For Hytale:

     ```
     [version, time, zone, qname = %^[Hh][Yy][Tt][Aa][Ll][Ee]\.[Gg][Aa][Mm][Ee][Ss]\.[Bb][Rr][Aa][Dd][Ff][Oo][Rr][Dd][Ll][Yy]\.[Cc][Oo][Mm]\.?$%, qtype = A || qtype = AAAA, ...]
     ```

   - More packages join the regex with a top-level `|`, because parentheses aren't supported [11].
   - Synth asserts that the pattern is at most 1,024 characters [12]. That leaves room for about eight short ids.
   - If the trailing `...` isn't accepted, the pattern names all ten fields (first-deploy test).
3. **Parse and re-check in Go.** The pattern is only a pre-filter. A line counts as a lookup only if all of these hold:
   - it has exactly 10 space-separated fields. Any other count is counted as `wake-malformed` and dropped, so an odd query name can't shift the fields;
   - its zone ID is the configured zone;
   - its format version is `1.0`. Any other version is logged as `wake-format`, ignored and reported (see "Read errors"), so a format change fails closed. Checking the zone first keeps a forged line from raising it;
   - its type is `A` or `AAAA`;
   - its name, lower-cased and without a trailing dot, is `<id>.games.bradfordly.com` for a catalog id;
   - that id's server record exists (ADR-0004's server lookup). Lookups for ids without a server are dropped silently;
   - its time field is within 2 minutes of the event's `timestamp`, which the writer sets and a query name can't change [12]. The query time used everywhere below is that `timestamp`, never the text. Whether Route 53 sets it to the query time is unverified.

   The checks run in this order.
4. **Decide.** Using a fresh read of the item, take each lookup through these rows in order. The first matching row applies:

   | Situation | Result |
   |---|---|
   | The query time is before the item's `stateSince` | Dropped silently. It was made in an earlier state, and an earlier run judged it |
   | The looked-up server is the active one, in `STARTING` or `READY` | Nothing; players are connecting |
   | The item isn't `STOPPED` (another server running, `ADMIN`, `STOPPING`), or `blocked` is set | A refusal, with the reason Start would give (`other-server-active`, `admin-mode`, `stopping`, `flood-guard`). No Start call |
   | `STOPPED`, and the query time is less than `WAKE_COOLDOWN_MINUTES` after `stateSince` (the last stop) | A refusal: `cooldown` |
   | Otherwise | Eligible |

   If any lookup is eligible:
   - pre-check the daily and monthly limits on the read. If one fails, refuse every eligible lookup with `daily-limit` or `wake-budget`;
   - otherwise call Start(id, cause `lookup`, query time) with the latest eligible lookup. Other eligible lookups for that server are covered by it. Those for other servers are refused with `other-server-active` once the Start succeeds;
   - a run that starts a server drops that server's refusals, so "not starting" never follows "cold start underway".

   There is at most one Start per run.
5. **Start** applies ADR-0003's steps under the lease lock and re-checks the lookup limits, with the cooldown against the passed query time (see "Limits and counters"). A `busy` lock is neither a refusal nor a notice: the lookup is still inside the next run's window.
6. **Notices.** Each distinct refusal reason in the run goes to "Refusals and Discord".
7. **Errors.** Rule 10 catches its own errors and never fails the reconcile run, so a broken read neither blocks rules 0–9 nor triggers Lambda's retries (see "Read errors").

**Why a window and no checkpoint.** Every effect is idempotent across overlapping windows:

- **Start** can succeed only once. The state becomes `STARTING`, and later runs see it.
- **The daily count** increases only in Start's own `STARTING` write.
- **A lookup made before a stop can never start the server after it.** Rule 10 drops lookups made before `stateSince`, and the cooldown compares the lookup's query time with the stop time, not with the run time.
- **Notices** are throttled per reason by a conditional write.

So reading a lookup twice is harmless.

- **When a lookup is missed:** only if no run succeeds between the line's arrival in the log and 15 minutes after the lookup, or if sustained query volume exhausts the read budget. The first case tolerates one skipped run (busy lock, throttling, an error) plus several minutes of delivery delay.
- **Why not a checkpoint:** it would still need overlap for late lines, and then a dedupe rule.
- **Stale lookups:** a lookup older than 15 minutes never starts anything, even after an outage or a re-enable. The player retries, and every retry after the 60 s negative cache is a fresh lookup.

### Limits and counters

The limits apply only to cause `lookup`. API, CLI and future Discord starts are unchanged.

| Limit | Value (env var) | Checked by | Refusal reason |
|---|---|---|---|
| No lookup start within this time after any stop | 30 min (`WAKE_COOLDOWN_MINUTES`) | Rule 10, per lookup; Start | `cooldown` |
| Lookup starts per UTC day | 3 (`WAKE_DAILY_MAX`) | Rule 10's pre-check; Start under the lock | `daily-limit` |
| The month's run-hours at which lookup starts stop | 60 h (`WAKE_RUN_HOURS_CAP`) | Rule 10's pre-check; Start under the lock | `wake-budget` |
| ADR-0003's own refusals | Unchanged | Rule 10 (from the item); Start | `flood-guard`, `other-server-active`, `admin-mode`, `stopping` |

- **Order:** for cause `lookup`, Start checks `cooldown`, `daily-limit` and `wake-budget` before ADR-0003's 80 h cap.
- **The last stop** is `stateSince` while the item is `STOPPED`, which is when it entered `STOPPED` (F4). That covers the `stopped` handler, including reconcile rule 1's repair, and item creation. A failed start that returns to `STOPPED` restarts the cooldown.
- **Run-hours:** the threshold uses ADR-0003's `runSeconds` and `runMonth`; a different month counts as 0.
  - Synth asserts `WAKE_RUN_HOURS_CAP` < `RUN_HOURS_CAP`, so `budget-exhausted` never reaches a lookup.
  - Like ADR-0003's cap, the threshold is checked only at start, and run-hours accrue at stop. A lookup run that starts just under 60 h can last until the 12 h 15 min backstop.
  - So members and the CLI keep about 20 h when lookup runs end normally (a no-join run adds about 0.5 h). If fake peers hold the last lookup run, they keep at least 7 h 45 min (W3). An admin can still override the 80 h cap.
- **Worst cases:**
  - **False wakes:** at most 3 a day, so about 93 a month. At about 0.5 h each that is about 46 h and $4.25 (estimate). They stop at 60 h, plus the run in progress.
  - **Hostile wakes:** fake peers holding every run (ADR-0003) can burn about 23 run-hours a day, because runs are sequential with a 30-minute cooldown between them. They reach 60 h in under three days and can hold the last run to about 72 h 15 min, leaving members at least 7 h 45 min. Such runs needn't end `no-join` or report a backstop: the attacker can stop just before it, and the host's own 12 h power-off reports best-effort. So `wakeSeconds` sums the hours of lookup runs, whatever their stop reason ("Measuring noise").
  - **Draining the daily pool:** anyone who knows a hostname can use up the 3 daily lookup starts early in the UTC day, and every hostname is public in this repo. Real players then get `daily-limit` until 00:00 UTC and need a manual start, which until 0006 ships means asking the owner to use the CLI. This is an accepted risk of W4 (see "Revisit if").

New item attributes. Every write is a conditional `UpdateItem` on named attributes, as in ADR-0003:

| Attribute | Written by | Update and condition |
|---|---|---|
| `wakeDay` (UTC `YYYY-MM-DD`), `wakeCount` | Start's `STARTING` write for cause `lookup`, under the lock | If `wakeDay` is today: `SET wakeCount = wakeCount + :one`, condition `wakeDay = :today AND wakeCount < :max`. Otherwise: `SET wakeDay = :today, wakeCount = :one`, condition `attribute_not_exists(wakeDay) OR wakeDay <> :today`. Both also require `lockOwner = :me` |
| `wakeMonth` (`YYYY-MM`), `wakeStarts` | The same write | The same pattern, by month |
| `wakeSeconds`, `wakeNoJoin` | The `stopped` handler's existing write, when the run's `cause` is `lookup`. `wakeSeconds` adds the run's seconds (the amount added to `runSeconds`); `wakeNoJoin` adds 1 when `stopReason` is `no-join` | `SET #c = if_not_exists(#c, :zero) + :n`. Added to that write only when the locked read shows `wakeMonth` = the current month, and never part of its condition. So a run that crosses a month boundary isn't counted and can't fail the stop. The month reset (above) sets both to 0 |
| `wakeNotice-<reason>`: one flat timestamp per refusal reason, plus `wakeNotice-unavailable` for "Read errors" | Rule 10, lock-free | `SET #n = :now`, condition `attribute_not_exists(#n) OR #n < :cutoff`, where `:cutoff` = now − `WAKE_NOTICE_HOURS` |

- **Flat notice attributes** avoid a parent map, which would have to exist before its first nested write.
- **Duplicate runs:** a duplicate Scheduler delivery or a retried run can neither start twice nor count twice. The lock, the state and these conditions all agree.

### Refusals and Discord

- **Logging:** `events` logs one line per run for each server id and reason, with a count and the newest query time. So a flood of refused lookups adds a few lines per run, not one per lookup.
- **`Request Rejected` for cause `lookup`:**
  - it is emitted only after the conditional notice write succeeds, so at most once per reason per `WAKE_NOTICE_HOURS` (6), across all servers;
  - its detail is `{command: "start", reason, cause: {kind: "lookup"}, serverId}`;
  - a `busy` lock produces no notice.
- **The notifier** posts `Request Rejected` only when the cause is `lookup`. Its rule already receives every `bradfordly-games.lifecycle` event (ADR-0003). CLI and API rejections are still not posted.
- **Hostnames always go in Discord code spans** (`` `hytale.games.bradfordly.com` ``).
  - This keeps link previews and client prefetching from resolving the name and waking the server.
  - Whether Discord would otherwise linkify a bare hostname is unverified.
  - The notifier builds a hostname only from `serverId`, after re-checking it against `^[a-z0-9][a-z0-9-]{0,31}$`, and never from log text. So a backtick or newline in a query name can't reach Discord.

| Event | Message |
|---|---|
| `Server Starting`, cause `lookup` | "Lookup detected for `X.games.bradfordly.com`; cold start underway" (ADR-0003's text, with the hostname in a code span) |
| `Request Rejected`, cause `lookup` | "Lookup detected for `X.games.bradfordly.com`; not starting: <reason>" |
| `Wake Unavailable` | "Wake on connect isn't working (<code>); ask an admin to start servers" |

The `<reason>` texts:

| Reason | Text |
|---|---|
| `cooldown` | "it stopped less than 30 minutes ago" |
| `daily-limit` | "3 lookup starts already today; ask an admin to start it" |
| `wake-budget` | "lookup starts pause at 60 run-hours a month; ask an admin to start it" |
| `other-server-active` | "another server is running" |
| `admin-mode` | "the host is in admin mode" |
| `stopping` | "it is still stopping" |
| `flood-guard` | "starts are blocked by the flood guard" |

### Read errors

A broken read must not look like "nobody tried to join".

- **Lasting errors** are exactly four codes:
  - `access-denied` (`AccessDeniedException`);
  - `not-found` (`ResourceNotFoundException`, for example after the log group is deleted);
  - `invalid-parameter` (`InvalidParameterException`, for example a rejected pattern);
  - `format` (a format version other than `1.0`).
- **Everything else is transient,** including throttling, 5xx errors, timeouts, deadlines and cancellations, which a flood can cause. It is logged as `wake-read-error`, and the next run reads the same window again.
- **On a lasting error:**
  - it is logged as `wake-read-error` or `wake-format`;
  - `events` emits `Wake Unavailable` from `bradfordly-games.lifecycle`, but only after the conditional `wakeNotice-unavailable` write succeeds. So it is posted at most once per `WAKE_NOTICE_HOURS`. Its type-specific detail is `code`, next to ADR-0003's common fields (`v`, `instanceId`, `at`);
  - `code` is one of the four values, and the notifier drops any other. AWS error messages, which can name the role and account, never go into events or Discord;
  - the notifier's rule already receives every `.lifecycle` event (ADR-0003).

### Kill switch

- **`WAKE_ENABLED`** (deployed as `true`) is an environment variable on `events`, the function that runs the reconcile. It is set from CDK context.
- **Fails closed:** rule 10 runs only when the value is exactly `true`. Any other value, or none, disables it and logs `wake-disabled`, so a mistyped emergency edit still turns wake off.
- **When disabled:** rule 10 doesn't run. There are no `FilterLogEvents` calls, no Starts with cause `lookup` and no notices. Query logging continues, at about $0.
- **In an emergency:** the owner edits the variable in the Lambda console, and it applies to new invocations. Persist the change by setting the context flag and redeploying.
- **Deploys can't silently re-enable it:** the deploy preflight fails when the live value isn't `true` but the context value is. To re-enable, set the live value to `true` in the console, then deploy.
  - It also fails when the zone's query logging config is missing or points at another log group, so a config deleted in a flood can't silently stay deleted.
  - Both checks skip the first deploy with wake, when `events` doesn't exist yet or has no `WAKE_*` variables, as the SOA check skips a missing zone.
- **Re-enabling** never acts on lookups older than 15 minutes. Lookups made in the last 15 minutes while wake was off can still start a server.

### IAM

The `events` role gains two statements: the read below, and the `Wake Unavailable` `PutEvents` statement (F11). `api` gets neither: the internet-facing function never reads resolver data.

```
Effect: Allow
Action: logs:FilterLogEvents
Resource:
  - arn:aws:logs:us-east-1:<account>:log-group:/aws/route53/games.bradfordly.com
  - arn:aws:logs:us-east-1:<account>:log-group:/aws/route53/games.bradfordly.com:*
```

- **Two ARN forms:** `FilterLogEvents` acts on the `log-group` resource [13], and both forms name only this log group. CDK's `logGroupArn` uses the `:*` form, while the Service Authorization Reference uses the bare form. The first-deploy test shows which one IAM evaluates.
- **"Never granted" check** against ADR-0002's list and ADR-0003's and ADR-0004's additions:
  - nothing on the list is used: no `iam:PassRole`, `lambda:*`, `scheduler:*` or SSM writes;
  - Route 53 rights are unchanged, and `RecordTypes ["A"]` keeps SOA and NS out of reach;
  - E5 extends the list.
- **Only Route 53 writes the query log group.** A role that could write to it could forge lookups and start servers, which would break ADR-0003's rule that host input can't start servers.
  - So every function's logging grant names only its own log group, as ADR-0003 already states.
  - No function uses `AWSLambdaBasicExecutionRole`, which allows `logs:CreateLogStream` and `logs:PutLogEvents` on `*` [20]. CDK attaches it to any function created without a `role`, so the infra PR passes explicit roles.
  - CDK's own helper functions attach it too (checked in aws-cdk-lib 2.273.0). The `LogRetention` function behind the deprecated `logRetention` prop also gets `logs:PutRetentionPolicy` and `logs:DeleteRetentionPolicy` on `*`, and the core custom-resource provider attaches the managed policy. So the infra PR sets retention with the `logGroup` prop, never `logRetention`.
  - A synth assertion checks every role in the stack: none attaches `AWSLambdaBasicExecutionRole`, and no statement allows `logs:` writes or retention changes on `*`. A CDK deploy-time provider that can't avoid it goes on a reviewed allowlist in that assertion.
- **Nothing else is added:** no function, role, reserved concurrency, subscription, invoke permission or bus producer. The one new event, `Wake Unavailable`, gets its own `PutEvents` statement on the `events` role only (F11), so the internet-facing `api` can't emit it. The account-concurrency prerequisite stays at 102 or more.

### Configuration

New environment variables on `events` (12-factor):

- `WAKE_ENABLED=true`
- `WAKE_LOG_GROUP=/aws/route53/games.bradfordly.com`
- `WAKE_FILTER_PATTERN` (generated at synth)
- `WAKE_WINDOW_MINUTES=15`
- `WAKE_COOLDOWN_MINUTES=30`
- `WAKE_DAILY_MAX=3`
- `WAKE_RUN_HOURS_CAP=60`
- `WAKE_NOTICE_HOURS=6`

The zone ID and the record names are already configured (ADR-0002).

### Game clients must join by hostname

Only a lookup can wake a server. The public IP also changes at every start (ADR-0002), so players need the hostname anyway.

- **Onboarding:** package onboarding confirms that the game client joins by `hostname:port`.
- **IP-only joins:** Steam titles joined from the server browser or by IP never trigger a wake, and their saved address breaks at each start.
- **ADR-0004:** its "How to add a game package" isn't edited here (Follow-ups).

### Measuring noise

- **`GET /status`** (ADR-0003) gains `wake`:
  - lookup starts today;
  - lookup starts this month;
  - how many of this month's lookup starts ended `no-join`, meaning nobody joined (`wakeNoJoin`);
  - how many run-hours this month's lookup runs used, whatever their stop reason (`wakeSeconds`).
- **Refusal notices** in Discord show how often the limits bite.
- **Investigating one false wake:** a Logs Insights query over the query log group, within its 3 days (runbook).
- **Revisit triggers:**
  - more than 10 lookup starts in a calendar month end `no-join`, or more than half of a month's lookup starts do. Then choose between requiring a second lookup and a secret hostname suffix (W5);
  - lookup runs reach `wake-budget`, `wakeSeconds` is far above the hours the group actually played, or `daily-limit` refusals arrive on days nobody played. These point to a deliberate attacker, which only a secret hostname suffix stops.

### Cost

Unit prices are verified ([8][9]; ADR-0002 [11]; ADR-0003 [9][10][13]). Volumes are estimates. Each log event is taken as about 150 bytes (a line of about 125 characters plus overhead), or up to about 400 bytes for the longest names [21]; the billed size is unverified.

| Item | Idle month | One session woken by a lookup |
|---|---|---|
| Route 53 query logging | $0 [1] | $0 |
| Log ingestion: idle noise of about 10,000 queries ≈ 1.5 MB at $0.50/GB | $0 in the free tier (about $0.001 without) | < $0.001 |
| Log storage: 3 days at $0.03/GB-month | $0 | $0 |
| `FilterLogEvents`: 8,640 calls a month | $0 (no charge listed; unverified) | $0 |
| Extra reconcile time: about 0.3 s × 8,640 runs at 128 MB ≈ 324 GB-s | $0 in the free tier (about $0.004 without) | < $0.001 |
| DynamoDB: counters and notices, a few hundred writes | < $0.001 | < $0.001 |
| Route 53 queries ($0.40 per million) | Already billed without this ADR | Already billed |
| **Total** | **About $0.00 ($0.01 without free tiers)** | **< $0.01, plus EC2 as in ADR-0003** |

- **Idle headroom:** ADR-0002's $1.26–1.76 is untouched.
- **A false wake** costs about 0.4–0.6 h of EC2 and IPv4 at $0.091/h, so about $0.04–0.06 (estimate: boot, ready, then 20 minutes of no-join).
- **False wakes per month:** at most 3 a day, so at most about $4.25/month (estimate). They stop at 60 h, plus the run in progress. ADR-0003's active ceiling of about $8.40/month is unchanged.
- **A query flood:**
  - 1 million logged queries is about 0.15–0.4 GB, or about $0.08–0.20 of ingestion (estimate). That comes on top of the $0.40 of Route 53 queries the zone already incurs.
  - `WAKE_ENABLED` doesn't stop ingestion; the flood runbook does (Follow-ups).
  - The read side can't amplify a flood: at most 5 calls and 10 s per run, and a few summary log lines.

## Amends ADR-0002

ADR-0002's status line records these amendments.

- **E1. Decision → DNS.**
  - Old: CDK creates the zone, outputs its NS records and publishes CAA. Only the control plane writes A records (TTL 60).
  - New: unchanged, plus:
    - query logging to `/aws/route53/games.bradfordly.com` (3-day retention), with its Logs resource policy;
    - an SOA with TTL 60 and minimum 60, set once by the owner outside CDK and checked by the deploy preflight.
- **E2. Contract → Endpoint.**
  - Kept: "The A record exists only while the instance runs with that IP", and Stop's DELETE. This resolves ADR-0003's "Left to later ADRs → 0005".
  - New: while stopped, the name answers NXDOMAIN, which resolvers cache for 60 s, and a lookup of it may start the server.
- **E3. Contract → Unrequested runs happen.** Adds lookup-started runs (cause `lookup`). They are bounded by:
  - the 30-minute cooldown;
  - 3 a day;
  - the 60 h threshold;
  - `WAKE_ENABLED`;
  - on top of the backstops and the 80 h cap.
- **E4. Contract → Configuration.** Adds the query log group name.
- **E5. Contract → Grants and Never granted.**
  - Grants: `events` gets `logs:FilterLogEvents` on the query log group only.
  - Never granted, for every control-plane role and the instance role:
    - Route 53 query-logging actions (`route53:CreateQueryLoggingConfig`, `route53:DeleteQueryLoggingConfig`);
    - any `logs:*` action on the query log group or its streams, except `events`' `FilterLogEvents`. Route 53 is its only writer. This covers `logs:CreateLogStream` and `logs:PutLogEvents` (forged lines could start servers), and policy, retention and subscription changes (`logs:PutResourcePolicy`, `logs:DeleteResourcePolicy`, `logs:PutRetentionPolicy`, `logs:DeleteLogGroup`, `logs:PutSubscriptionFilter`);
    - `AWSLambdaBasicExecutionRole`, or any `logs:` write or retention change on `*`, for any role in the stack, including CDK's deploy-time helpers outside a reviewed allowlist. Each function writes only its own log group.
  - SOA and NS writes stay excluded by `RecordTypes ["A"]`.
- **E6. Follow-ups → Runbooks and owner actions.** Adds:
  - the one-time SOA UPSERT;
  - the preflight's SOA and resource-policy checks;
  - the wake runbooks in this ADR's Follow-ups.

## Amends ADR-0003

ADR-0003's status line records these amendments.

- **F1. Interfaces for later ADRs → 0005, plus Commands, Events and the bus policy.**
  - Old: 0005 emits `Start Requested` from `bradfordly-games.wake`, with its own grant, a rule entry and a bus-policy producer entry.
  - New: there is no `.wake` producer.
    - The cause table loses the `bradfordly-games.wake` row, and the command rule's pattern never lists `.wake`.
    - In the Events table, "`bradfordly-games.cli`, later `.wake` and `.discord`" becomes "`bradfordly-games.cli`, later `.discord`", and its producers "owner, 0005, future" become "owner, future".
    - The bus-policy table loses the `.wake` row. Principals outside the producer set hit the catch-all Deny for that source, and no producer's source-scoped grant includes it.
    - A synth assertion checks that no rule pattern, cause mapping or grant names `bradfordly-games.wake`.
  - Cause kind `lookup` stays. Only reconcile rule 10 sets it, at member level, and it never overrides a cap.
- **F2. Reconcile.** Adds rule 10, the wake step. It runs last in every run while `WAKE_ENABLED` is `true`.
- **F3. Start, run-hours cap and Stop.**
  - For cause `lookup`, Start takes the lookup's query time, and step 4 first refuses with:
    - `cooldown`: the query time is less than 30 minutes after the last stop;
    - `daily-limit`: 3 lookup starts this UTC day;
    - `wake-budget`: the month's run-hours are at or above 60 h.
  - Only then does it check the 80 h cap, which a lookup never reaches.
  - Step 5's `STARTING` write also updates `wakeDay`/`wakeCount` and `wakeMonth`/`wakeStarts`, with the conditions in "Limits and counters".
- **F4. Item and the `stopped` handler.**
  - New attributes: `wakeDay`, `wakeCount`, `wakeMonth`, `wakeStarts`, `wakeSeconds`, `wakeNoJoin`, one `wakeNotice-<reason>` per refusal reason, and `wakeNotice-unavailable`.
  - Every transition into `STOPPED` sets `stateSince` to the transition time. That covers the `stopped` handler (including rule 1's repair) and item creation (rule 0).
  - For lookup runs, the `stopped` handler adds the run's seconds to `wakeSeconds` and counts `no-join` endings in `wakeNoJoin`. Neither is ever part of the write's condition.
- **F5. Events → `Request Rejected`.**
  - New reasons, for lookups only (the API never returns them): `cooldown`, `daily-limit` and `wake-budget`.
  - The detail adds `serverId`.
  - Consumer: the notifier, for cause `lookup` only.
  - For lookups, it is emitted at most once per reason per 6 h.
- **F6. Discord notifier.**
  - Every hostname is posted in a code span, built only from a re-checked `serverId`.
  - New messages: "Lookup detected for `X.games.bradfordly.com`; not starting: <reason>", and the `Wake Unavailable` message.
- **F7. IAM.**
  - The `events` role adds `logs:FilterLogEvents` on the query log group. `api` and every other role are unchanged.
  - Every function's logging grant stays limited to its own log group, and none uses `AWSLambdaBasicExecutionRole` (E5).
- **F8. Components and Configuration.**
  - New: the `WAKE_*` variables on `events`.
  - There is no new function, role or reserved concurrency, so the account-concurrency prerequisite stays at 102 or more.
  - Follow-up 1's preflight adds the SOA check, the Logs resource-policy count, the live `WAKE_ENABLED` check and the query-logging-config check.
- **F9. API → `GET /status`.** Adds the `wake` counters.
- **F10. "Left to later ADRs → 0005".** Resolved by E2. No placeholder write points are needed, so the running handler, Stop and reconcile rule 7 are unchanged.
- **F11. Events.** Adds `Wake Unavailable` (`code`) on `bradfordly-games.lifecycle`, produced by `events` and consumed by `notifier` ("Read errors").
  - The `events` role gets one `PutEvents` statement for it: source `bradfordly-games.lifecycle`, detail-type `Wake Unavailable`, with `Null` checks.
  - It isn't added to the grant that `api` shares, so `api` can't emit it.

## Interfaces for later ADRs

- **0006 (frontend and authentication):**
  - **Status:** show the `wake` counters from `GET /status`. A Start button complements wake and isn't subject to the lookup limits.
  - **Hostnames are public; the limits are the only control.** Every server name follows from a `catalog/` id in this public repo.
    - A certificate for any name under `games.bradfordly.com` would also put names into Certificate Transparency logs, which automated scanners watch, and every lookup of a server name can wake that server.
    - ADR-0002's CAA records forbid issuance today. 0006 decides; one option is a frontend name outside this zone.
  - **No DNS prefetch:** browsers may resolve the hostnames of links before a click, so one member opening the status page could wake a stopped server and use up the daily pool. Show hostnames as plain text, not links, and send `X-DNS-Prefetch-Control: off` (browser support unverified).
  - **Refusal texts:** once members can start servers in the UI, point "ask an admin to start it" at the app instead.
- **Future Discord commands:** `bradfordly-games.discord` stays the planned producer (ADR-0003), and the lookup limits don't apply to it.

## Consequences

Positive:

- **For players:** they start a stopped server by trying to join it, with about $0 of idle cost and nothing always on.
- **Fewer parts than ADR-0003 planned:** two IAM statements, one reconcile rule and a log group. There is no new function, reserved concurrency or bus producer.
- **Bounded:**
  - noise costs at most 3 runs a day;
  - lookup starts stop at 60 h, so members keep about 20 h, or at least 7 h 45 min if fake peers hold the last lookup run;
  - a query flood can't become a flood of invocations;
  - one variable turns wake off.
- **DNS keeps its meaning:** ADR-0002's record rule stands, so a resolving name still means "running", and NXDOMAIN fails fast for players.
- **Privacy:** resolver IPs and client subnets stay in a 3-day log group that only `events` and the owner can read.

Negative:

- **Reaction time:** a start begins 0–5 minutes after the lookup, plus the log delay.
- **Noise and false refusals:**
  - Any lookup counts. A scan, a link preview or a friend's server list can start a run, up to 3 times a day (W5).
  - A lookup within 30 minutes of a stop is refused, even when it comes from a real player.
  - Anyone who knows a hostname can use up the daily pool, and real players then need a manual start until 00:00 UTC. Until 0006, that means asking the owner.
- **Missed lookups:** a lookup that no run reads within 15 minutes is dropped, and the player must retry. Under sustained query volume, the read budget drops the oldest lookups in the window.
- **Fails closed:** a broken read or a change in Route 53's log format stops wake until it is fixed. Discord shows `Wake Unavailable` at most every 6 h.
- **Outside CDK:** the SOA is set by hand and checked only at deploy.
- **Fails open on item loss:** if the item is lost, the day and month counters reset, like ADR-0003's run-hours cap.
- **IP-only games:** games joined by IP (some Steam titles) don't wake.
- **Unverified until the first deploy:**
  - the per-query log delay;
  - the letter case of logged names, and how Route 53 logs spaces and non-printable bytes in them;
  - that each event's `timestamp` is the query time;
  - the trailing `...` in space-delimited patterns;
  - which ARN form IAM evaluates for `FilterLogEvents`;
  - that `FilterLogEvents` calls aren't billed;
  - the billed bytes per log event;
  - that a CloudFormation update leaves the SOA alone;
  - Hytale's hostname join and its saved-server list behaviour;
  - how Discord renders hostnames in code spans.

Revisit if:

- More than 10 lookup starts in a month end `no-join`, or more than half of them do: require a second lookup, or add a secret hostname suffix kept out of this repo.
- Lookup runs reach `wake-budget`, `wakeSeconds` is far above real play, or `daily-limit` refusals arrive on days nobody played: add the secret suffix.
- Players find the reaction too slow: move to push (option C).
- The generated pattern nears 1,024 characters: match only the zone suffix server-side, and the ids in Go.
- Route 53 changes its log format version.
- The daily limit or the 60 h threshold blocks real play.
- 0006 issues a certificate under the zone.
- `FilterLogEvents` appears on the bill, or a query flood does: add a `DNSQueries` alarm.

## Follow-ups (not part of this ADR)

1. **Infra PR:**
   - **Resources:**
     - the log group and the Logs resource policy;
     - `queryLogsLogGroupArn` on the zone, with a dependency on the policy;
     - the two `events` statements and the `WAKE_*` variables;
     - explicit roles for every function, so none gets `AWSLambdaBasicExecutionRole`;
     - log retention through the `logGroup` prop, never `logRetention`.
   - **Synth assertions:**
     - the pattern is generated from `catalog/` and is at most 1,024 characters;
     - `WAKE_RUN_HOURS_CAP` < `RUN_HOURS_CAP`;
     - nothing names `bradfordly-games.wake` (F1);
     - no role in the stack attaches `AWSLambdaBasicExecutionRole` or allows `logs:` writes or retention changes on `*`, apart from a reviewed allowlist of CDK deploy-time providers.
   - **Preflight checks:**
     - the SOA TTL and minimum are 60 (fail otherwise);
     - the account has fewer than 10 Logs resource policies;
     - a live `WAKE_ENABLED` that isn't `true` blocks a deploy whose context value is `true`;
     - the zone's query logging config exists and points at the query log group.
     - The last two skip the first deploy with wake.
   - **IaC-scan findings:** expect findings on the log group's 3-day retention and default encryption, and suppress them with this ADR as the reason. They are likely Checkov CKV_AWS_338 and CKV_AWS_158 (IDs unverified).
2. **Backend PR:**
   - rule 10, with its read budget and error handling;
   - the lookup checks in Start;
   - the counters and the notice writes;
   - `Wake Unavailable`, the notifier messages and code spans;
   - golden parser tests for: mixed case, a trailing dot, AAAA, TXT, an IPv6 resolver, a client subnet present and absent, another zone's ID, an unknown id, a format version other than `1.0`, too many or too few fields, and names with escaped or raw spaces, backticks and newlines (`\032`, `\096`, `\010`);
   - a fuzz test of the parser.
3. **First-deploy tests:**
   - **Logging:**
     - `dig A hytale.games.bradfordly.com` against one of the zone's name servers produces a log line with exactly 10 fields. Record the delay from query to ingestion over 10 lookups, and the log stream names.
     - A mixed-case query (`HyTaLe.GaMeS…`) matches the pattern. Record the case as logged.
     - Queries whose names hold a space, a backtick and a newline (bytes 32, 96 and 10) are sent to the zone. Record how they're logged, and confirm that none splits into a second event or passes the parser.
     - Record whether each event's `timestamp` equals the query time in its text.
     - AAAA matches and TXT doesn't.
     - The update that adds logging to the retained zone succeeds without `InsufficientCloudWatchLogsResourcePolicy` and leaves the SOA unchanged.
   - **SOA:**
     - After the runbook, `dig SOA games.bradfordly.com` shows TTL 60 and minimum 60, and NXDOMAIN answers carry that SOA.
     - The preflight fails when given a deliberately wrong expected value.
   - **IAM:**
     - `simulate-principal-policy` allows `events` `FilterLogEvents` on the query log group and denies it on another log group.
     - It denies `logs:FilterLogEvents`, `logs:CreateLogStream` and `logs:PutLogEvents` on the query log group to `api`, `hostevents`, `notifier` and the instance role, and `logs:PutLogEvents` to `events`.
     - A real call shows which ARN form is needed.
     - Removing the `events` read statement produces one `Wake Unavailable` notice, and no second one within 6 h.
   - **End to end:**
     - After the server has been stopped for more than 30 minutes, a lookup leads to "Lookup detected" in Discord, a start, the A record and a join once ready.
     - A start nobody joins ends `no-join` and increments `wakeNoJoin`.
   - **Limits:**
     - A lookup within 30 minutes of a stop gives `cooldown` and one notice; a repeat within 6 h gives no notice.
     - The fourth lookup start in a UTC day gives `daily-limit`.
     - A lowered `WAKE_RUN_HOURS_CAP` gives `wake-budget`.
     - Another server running gives `other-server-active`.
     - Lookups for the same server while it is `STARTING` are silent.
     - An A and an AAAA lookup in the same run give one Start and no refusal.
     - A lookup made just before an idle stop gives no notice afterwards.
   - **Window:**
     - With the lock held across one run, the lookup still wakes the server on the next run.
     - A lookup older than 15 minutes never wakes it.
     - Two concurrent reconcile invocations give one Start and one count.
   - **Kill switch:** `WAKE_ENABLED=false` (and `False`) in the console stops wake, and the preflight then blocks a deploy with context `true`. Lookups older than 15 minutes don't fire after re-enabling. The first deploy with wake passes the preflight.
   - **Clients:**
     - The Hytale client joins by hostname.
     - Check whether opening its saved-server list causes lookups.
     - Discord shows hostnames as code, not links.
   - **Bill:** the first month shows no `FilterLogEvents` charge, and log ingestion inside the free tier.
4. **Runbooks:**
   - **Set the SOA**, once, after the zone exists:
     1. Read the current SOA with `aws route53 list-resource-record-sets --hosted-zone-id <id> --query "ResourceRecordSets[?Type=='SOA']"`.
     2. UPSERT it with `TTL` 60, keeping every field except the last one, the minimum, which becomes 60:

        ```
        aws route53 change-resource-record-sets --hosted-zone-id <id> --change-batch '{"Changes":[{"Action":"UPSERT","ResourceRecordSet":{"Name":"games.bradfordly.com.","Type":"SOA","TTL":60,"ResourceRecords":[{"Value":"<primary-ns>. <hostmaster>. 1 7200 900 1209600 60"}]}}]}'
        ```

   - **Kill switch:** edit `WAKE_ENABLED` in the console in an emergency, then set the context flag and redeploy. To re-enable, set the live value to `true` first, then deploy.
   - **Query flood:** `WAKE_ENABLED` alone doesn't stop ingestion.
     1. Set `WAKE_ENABLED=false` in the console, so wake is visibly off.
     2. Delete the zone's query logging config by hand (`aws route53 delete-query-logging-config`), which stops ingestion.
     3. After the flood, re-create it with `create-query-logging-config` and the same log group, because CloudFormation won't restore it while the template is unchanged. Then set `WAKE_ENABLED=true`.
     4. Until both are done, the preflight blocks deploys.
   - **Monthly:** compare the `wake` counters with the revisit trigger.
   - **Investigate a false wake:** run a Logs Insights query on the query log group within its 3 days. Copy nothing out of AWS.
5. **Docs:** add the hostname check to step 4 of ADR-0004's "How to add a game package", with an "Amended by" entry in ADR-0004. That is not done in this ADR's PR.

## Sources

Accessed 2026-10-16; [12], [20] and [21] on 2026-10-17.

1. Route 53 public DNS query logging — https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/query-logs.html
2. `CreateQueryLoggingConfig` — https://docs.aws.amazon.com/Route53/latest/APIReference/API_CreateQueryLoggingConfig.html
3. `AWS::Route53::HostedZone` — https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-route53-hostedzone.html
4. CDK `PublicHostedZone` — https://docs.aws.amazon.com/cdk/api/v2/docs/aws-cdk-lib.aws_route53.PublicHostedZone.html
5. NS and SOA records in a public hosted zone — https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/SOA-NSrecords.html
6. `ChangeResourceRecordSets` (propagation within 60 s) — https://docs.aws.amazon.com/Route53/latest/APIReference/API_ChangeResourceRecordSets.html
7. `AWS::Route53::RecordSet` — https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-route53-recordset.html
8. Route 53 logs as CloudWatch vended logs (2018) — https://aws.amazon.com/cn/about-aws/whats-new/2018/03/amazon-cloudwatch-adds-route53-logs-to-vended-logs/
9. Price List API, AmazonCloudWatch us-east-1 (offer effective 2026-10-01; `USE1-VendedLog-Bytes` $0.50/GB for the first 10 TB, `USE1-TimedStorage-ByteHrs` $0.03/GB-month) — https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonCloudWatch/current/us-east-1/index.json ; CloudWatch pricing — https://aws.amazon.com/cloudwatch/pricing/
10. CloudWatch Logs quotas — https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/cloudwatch_limits_cwl.html
11. CloudWatch Logs filter pattern syntax — https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/FilterAndPatternSyntax.html
12. `FilterLogEvents` — https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_FilterLogEvents.html
13. CloudWatch Logs actions and resources (China-partition Service Authorization Reference) — https://docs.amazonaws.cn/service-authorization/latest/reference/list_amazoncloudwatchlogs.html
14. Services that invoke Lambda (CloudWatch Logs: asynchronous) — https://docs.aws.amazon.com/lambda/latest/dg/lambda-services.html
15. Google Public DNS case randomisation — https://lists.dns-oarc.net/pipermail/dns-operations/2023-July/022239.html ; https://security.googleblog.com/2024/03/google-public-dnss-approach-to-fight.html
16. Java `InetAddress` caching — https://docs.oracle.com/en/java/javase/21/docs/api/java.base/java/net/InetAddress.html
17. doctorray117/minecraft-ondemand — https://github.com/doctorray117/minecraft-ondemand
18. lazymc — https://github.com/timvisee/lazymc
19. CloudWatch Logs subscription filters — https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/SubscriptionFilters.html
20. `AWSLambdaBasicExecutionRole` — https://docs.aws.amazon.com/aws-managed-policy/latest/reference/AWSLambdaBasicExecutionRole.html
21. RFC 2181 §11, name syntax (any binary label; names up to 255 octets) — https://www.rfc-editor.org/rfc/rfc2181#section-11
