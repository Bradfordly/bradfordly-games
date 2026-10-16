# 0004. Game packages and per-server configuration

- Status: Proposed
- Date: 2026-10-13

## Context

Issue #17. ADR-0002 runs one game container at a time from a deploy-time game catalog. ADR-0003 built the control plane and left these to this ADR:

- the package manifest and forms schema, including the fields its host agent needs (an anchored `readyPattern`, `authRequiredPattern`, `readyTimeoutSeconds`, and per-server idle and peer overrides);
- each game's server, created from the UI, plus the server-lookup function and the admin routes for create, config and secrets;
- the secrets model, the launcher's directory rule, and the enumerated tag values and record names in ADR-0002's Grants;
- the Hytale package.

Owner decisions (authoritative):

- (2026-10-12) Cost first: idle stays under $5/month. ADR-0002 leaves $1.26–1.76 of headroom and ADR-0003 uses about $0.
- (2026-10-12) New game types arrive only by reviewed PR. Servers are created and configured in the UI from existing packages, and images and ports are never set from the web.
- (2026-10-12) Admins create servers and edit config and secrets. Members view status and start or stop servers.
- (2026-10-12) 0006 picks the frontend, so the forms schema must not assume one.
- (2026-10-13) Replicate the convenience of Pterodactyl/Pelican "eggs" in one blueprint that suits games from any source: self-published binaries, SteamCMD, direct downloads and licence-gated apps. The MVP game is Hytale. Game *file* settings (Hytale's `Server/config.json`) are edited through detailed forms, not as raw files.
- (2026-10-13) **O5 Sources:** a package gets its flexibility from choosing a self-installing image. No typed source adapters, install scripts or installer containers.
- (2026-10-13) **O6 File writes:** a host-side merge at start.
  - `game-agent prepare <id>` runs from the bash launcher before `docker run`.
  - JSON is the only codec now; other codecs come by PR.
  - An unprivileged child writes as the package's `dataOwner`, using `openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS)`, a temp file and `renameat`.
  - Values are re-validated on the host, and any failure falls back to admin mode.
- (2026-10-13) **O7 Hytale image:** the Pelican yolk `ghcr.io/pelican-eggs/games:hytale`, pinned by digest, with MIT attribution.
- (2026-10-13) **O8 Managed keys:** the form always wins.
  - Every start rewrites managed keys from the stored values, and unmanaged keys are left alone.
  - The UI marks managed fields, and the host logs overrides of non-secret keys.
- (2026-10-13) **O9 MVP form scope:** scalar settings in `Server/config.json`, the yolk's env flags, and per-server idle overrides. No default-world settings and no list editors yet.
- (2026-10-13) **O10 Secrets:** credentials (licence keys, RCON or admin passwords) are write-only SecureStrings that `api` can never read, enforced by IAM. A join password is an ordinary setting: masked in the form and readable by admins.
- (2026-10-13) **O11 One server per game:** the app lets one small group of friends run a server for whichever game they're playing together.
  - Each game has exactly one server (one world), ever, and only one server runs at a time. So the server id is the package id.
  - This supersedes the owner's earlier "at most 3 servers" answer. It also refines ADR-0003's 2026-10-12 decision that "worlds are created in the UI from existing packages": the UI provisions a package's single server.
  - No delete in the UI; resetting or retiring a game's server is a runbook.
- (2026-10-13) **O12 Hytale re-auth:** manual on alert.
  - The auth pattern produces a Discord "authentication needed" message, never the code.
  - An admin reads the code over Session Manager and approves it within 15 minutes.

Verified facts (as of 2026-10-13 unless noted; "(unverified)" marks the rest):

- **Hytale server** [1][2][3]:
  - **Network:** QUIC on UDP 5520 (`--bind`). There is no SRV support and no query protocol.
  - **Versioning:** client and server must run the exact same protocol version. The ±2 tolerance is still "Coming Soon" [1].
  - **Server config:** `Server/config.json` is "read on server startup and written to when in-game actions occur", and manual changes made while the server runs "are likely to be overwritten" [1].
    - The config API exposes `ServerName`, `MOTD`, `Password`, `MaxPlayers`, `MaxViewRadius` (with a hard ceiling), `Defaults`, `RequireJoinPermission`, `Update`, `Backup` and a schema `VERSION` [3].
    - Seeds in the wild carry `Version` 3 and 4 [8][9].
    - Per-world settings live in `universe/worlds/<w>/config.json` [1].
  - **Command-line options:** `--bind`, `--auth-mode`, `--assets`, `--allow-op`, `--accept-early-plugins` and `--backup*` are command-line only. The official `--help` excerpt is truncated [1].
  - **Updates:** checks run hourly, files are staged, and the server exits with code 8 so a wrapper can apply them. `HYTALE_DISABLE_UPDATES` turns this off [1].
  - **Auth** [2]:
    - Device codes last 900 s, OAuth access tokens 1 h, and refresh tokens 30 days, rotated on every use. Game sessions last 1 h and refresh automatically.
    - Re-auth is needed after more than 30 days offline.
    - Failed token validation starts the server unauthenticated.
    - Both articles were edited on 2026-10-12.
  - **Undocumented:** saving on SIGTERM, the log format, and the download size. Third-party guides put `Assets.zip` at about 3.2 GB [7] (unverified).
  - **Operator policies** apply to every server, and enforcement can include suspending the server's tokens [26].
- **The Pelican Hytale yolk** [4] is built on `eclipse-temurin:25-jdk-noble` and runs as the non-root user `container`, with `STOPSIGNAL SIGINT` under `tini -g` and an MIT label. It last changed on 2026-05-08. Its Python entrypoint:
  - **Auth:** runs the OAuth device flow itself, with no stdin. It prints `Please visit: …` and a user code, waits up to 900 s, and on timeout starts the server without tokens.
  - **Token storage:** keeps tokens in `/home/container/.hytale-auth.json` (mode 0600) and passes them to the server as env vars, the OAuth refresh token included. It deletes the game session on exit.
  - **Downloads:** with `AUTO_UPDATE` on, it downloads through the Hytale API and checks SHA-256. It has no exit-8 loop and doesn't set `HYTALE_DISABLE_UPDATES`.
  - **Startup:** splits `STARTUP` with `shlex` (no shell) after expanding `{{VAR}}`/`${VAR}`, and runs it in `/home/container/Server`. It never writes `config.json`.
- **Other Hytale images:** none is official.
  - dustinrouillard's image maps `HTY_*` env vars into `config.json`, shows no licence, and ships RCON with the password `hytale` [5].
  - indifferentbroccoli's image states both GPL-3.0 and MIT, and its server auth needs a console [6].
- **Eggs and AMP** [8][9]:
  - **Egg variables** carry an env name, a default, Laravel rules and view/edit flags. Pelican's docs say those flags don't make a value secret.
  - **`config.files`** parsers (file, yaml, properties, ini, json, xml, toml) run before boot. Install scripts run once in an installer container.
  - **Readiness and stop:** `startup.done` is a substring, and `stop` is a console command or a signal.
  - **In the Hytale, Paper and Palworld eggs,** `config.files` is either empty or forces panel-owned values (ports, bind IP) into files, so gameplay settings are raw-file edits.
  - **The Palworld egg** downloads an unpinned config-parser binary from `releases/latest` and runs it at every start.
  - **AMP's Hytale template:**
    - maps 30 settings to `config.json` JSON paths or command-line fragments;
    - has anchored ready, join, leave and auth-URL regexes;
    - types `/auth login device` into the console when a log trigger fires.
- **Pelican itself** (2026-10-12) [10]:
  - Wings requires Docker CE, and its supported OS list excludes AL2023.
  - The Panel needs PHP 8.4/8.5, a web server and a database.
  - Wings listens on 8080 and serves SFTP on 2022 (third-party mirror).
- **Other target games** [11]–[15]:
  - **Minecraft:** itzg maps env vars to `server.properties`, and unset variables leave keys alone.
  - **Palworld:** every setting sits in one `OptionSettings=(…)` line that fails silently when malformed (third-party). thijsvanloef regenerates it from env.
  - **Valheim:** there is no server config file, only launch arguments (third-party), which lloesche maps from env.
  - **Foundry VTT:** felddy regenerates `options.json` from env at every start and accepts credentials from env or a file.
- **Validators and renderers** [16]–[19]:
  - **Go:** `santhosh-tekuri/jsonschema/v6` v6.0.3 (Apache-2.0) passes the JSON-Schema-Test-Suite through draft 2020-12.
  - **TypeScript:** Ajv 8.20.0 (MIT).
  - **Form renderers:** `@jsonforms/core` 3.8.0 (MIT) and `@rjsf/core` 6.11.0 (Apache-2.0). RJSF needs React 18 or later.
- **Go regular expressions** [20]: Go accepts `(?<name>…)` but lists no lookaround or backreferences, so a pattern can behave differently in Go and JavaScript.
- **Parameter Store and KMS** [21]–[24]:
  - **Tiers:** standard parameters hold 4 KB and are free. Advanced parameters hold 8 KB and cost $0.05 per parameter-month plus $0.05 per 10,000 API calls; they can't be downgraded.
  - **`PutParameter`:** `Overwrite` defaults to false, and an existing name fails with `ParameterAlreadyExists`. There is no version-conditional write. 100 versions are kept.
  - **`{{` values:** `PutParameter` rejects values "wrapped in double brackets `{{}}`" (exact scope unverified).
  - **KMS:** writing a standard SecureString needs `kms:Encrypt` and reading one needs `kms:Decrypt`, with encryption context `PARAMETER_ARN` [24].
    - The AWS managed `aws/ssm` key "grants Decrypt permission to all IAM principals in the AWS account", so a role needs no IAM statement to decrypt through SSM. To stop a role from decrypting, IAM must explicitly deny it [27][28].
    - This settles the open question in ADR-0003.
  - **Path reads:** `GetParametersByPath` "authorizes against the path in the request rather than against the parameters it returns". To keep a role out of a subtree, deny it on every ancestor path, up to `/` [29].
  - **History:** `GetParameterHistory` with `WithDecryption` returns decrypted SecureString values [30].
- **EBS** [25]: NVMe EBS devices carry their volume ID as the serial number (`lsblk -o +SERIAL`, `ebsnvme-id`). The serial has no hyphen (`vol0abc…`), unlike the volume ID (`vol-0abc…`) (security and code review, 2026-10-16).
- **Docker:** values passed with `--env-file` are stored in the container's config under Docker's data-root, which is `/srv/docker` (ADR-0002). Standard Docker behaviour; a first-deploy check.
- **Validator errors:** `santhosh-tekuri/jsonschema` v6.0.3 quotes the failing value in `pattern` and `format` errors [31].

## Options considered

All options keep ADR-0002's host, container template and launcher, and ADR-0003's agent and API. They differ in how settings reach the game.

### A. Simplest viable option: env-only settings

Each setting is typed and mapped to a container env var, and the image turns env vars into config files. No renderer code is needed.

Rejected, because file settings then exist only where the chosen image maps them, so each image dictates its form. For Hytale this forces either dustinrouillard (no licence, default RCON password) or an image pipeline of our own. Env targets remain the norm for packages whose image maps env, and B includes them.

### B. A plus managed JSON file targets, merged on the host at start (recommended)

At every start, `game-agent prepare` writes the stored values into the named keys of existing files, as an unprivileged writer, and leaves everything else alone. It works with any image, and only one codec (JSON) is needed now. Details under Decision.

### C. Egg- or AMP-compatible manifests with installers

This would import eggs or AMP manifests nearly verbatim: install scripts in installer containers, `{{VAR}}` startup templating, seven file parsers with wildcards, and console stop commands.

Rejected:

- It adds a second container and run mode, running unpinned "download latest" scripts.
- It splices user values into command lines.
- Console features need stdin, which ADR-0002 invariant 4 forbids.
- It brings parsers nobody needs: every other target game's image already maps env.

### Other alternatives (rejected)

- **Running Pelican (Panel and Wings):**
  - An always-on panel host costs about $7.36/month even at the cheapest size with IPv4 and disk, which alone breaks the budget.
  - Wings adds inbound ports where ADR-0002 allows only Session Manager.
  - Its OS list excludes AL2023 [10].
  - Its file manager and console assume a running host, and ours is stopped most of the time.
- **Typed source adapters** (vendor downloader, SteamCMD app id, URL plus checksum) run by us: root-run download code that duplicates what self-installing images already do (O5).
- **Full JSON Schema plus a JSON Forms or RJSF UI schema:** a second document per package, renderers that assume a single-page app [19], and premature before 0006.
- **Other places to write files:**
  - bash and `jq` in the launcher: JSON only, no schema re-validation, and hard to make symlink-safe;
  - a one-shot renderer container: needs an image pipeline of our own.
- **Other delivery paths:**
  - S3: adds `s3:GetObject` to the instance and a bucket to harden.
  - A GitHub Release asset, or embedding manifests in the agent: every package edit would restart the instance.
  - SSM Advanced for everything: costs money.
- **Server records in DynamoDB:** the host would need table reads or a second projection, and ADR-0003 keeps the host off the table.
- **Secrets Manager:** $0.40 per secret-month (ADR-0003 [22]).
- **Importing live values, two-way sync, or a raw file editor:** the host is stopped, and reporting values back would need a host channel that carries free text, which ADR-0003 forbids.
- **Other Hytale images:**
  - dustinrouillard and indifferentbroccoli: licence problems [5][6];
  - our own image: a pipeline to maintain (see "Revisit if").

## Pterodactyl/Pelican eggs: what we borrow, adapt and reject

| Egg element | Verdict | What it becomes here, and why |
|---|---|---|
| `docker_images` (user-selectable images; generic "yolks") | Adapt | One digest-pinned, self-installing image per package. Changing it is a PR (O5) |
| `startup_commands` with `{{VAR}}` | Adapt | An optional fixed command in `runtime.env` (the yolk's `STARTUP`). No user values in command lines, and no `{{` (SSM rejects it [23]) |
| `variables` (name, description, env var, default, sort) | Borrow | `settings[]` with `env` targets |
| `variables.rules` (Laravel strings) | Adapt | JSON Schema fragments, validated the same way in TypeScript and Go |
| `user_viewable` / `user_editable` | Adapt | Settings are admin-only. Fixed values go in `runtime.env`. Real secrets use `secret`, because egg variables never are |
| `config.files` parsers and `find` | Adapt | `files[]` with JSON Pointers to scalar keys. JSON only for now, more codecs by PR, no wildcards |
| Placeholders (`{{server.allocations…}}`, `{{server.environment…}}`) | Reject | Ports are fixed per package, and values are typed settings |
| `startup.done` (substring) | Adapt | An anchored RE2 `readyPattern` (ADR-0003) |
| `stop` (console command or signal) | Adapt | A signal only (the image's `STOPSIGNAL`) plus `stopTimeoutSeconds`. There is no stdin |
| `scripts.installation` (installer container) | Reject | Self-installing images do this. "Create the file if absent" replaces seed scripts |
| `features: eula` | Adapt | `acknowledgements[]`, accepted at create (plus `EULA=TRUE` env where a game needs it) |
| `features: java_version`, `pid_limit`, `steam_disk_space` | Reject, reject, adapt | The image is fixed per package; ADR-0002 already sets the PID limit; disk space becomes `diskEstimateGiB` |
| `file_denylist`, `config.logs` | Reject | There is no web file manager, and logs go to journald (ADR-0002) |
| `meta.update_url`, import and export | Reject | Definitions change only by PR. Porting an egg by hand, with MIT attribution, is fine |
| Allocations | Reject | One server runs at a time, and ports are fixed per package |

**AMP's settings manifest** [9], compared:

- **Borrowed:**
  - DisplayName, Description, Category, InputType, EnumValues labels, MinValue, Required, Placeholder and Suffix, as `label`, `description`, `group`, `schema`, `widget` and `unit`;
  - the managed-file declaration (`ConfigFile`, `ConfigType`), as `files[]`;
  - the anchored ready and auth-URL regexes (MIT), as the starting point for Hytale's patterns.
- **Adapted:** JSONPath `ParamFieldName` becomes a JSON Pointer.
- **Rejected:**
  - checkboxes that map to command-line fragments, and command-line templating;
  - hidden token fields;
  - console `CommandTriggers` (stdin);
  - `UpdateSources` stages (`bash -c` installers);
  - `Importable` (needs live files).

## Decision

**Option B.** Each game is a reviewed JSON package manifest, published as a projection to standard SSM.

- **Servers:** each package has at most one, stored as a record in SSM keyed by the package id and written by the admin API.
- **Credentials** are write-only SecureStrings.
- **Values reach the game** through env vars, or through JSON keys that `game-agent prepare` writes before every start, so the form always wins.
- **Hytale** runs on the Pelican yolk.

### Package manifest

`catalog/<package-id>.json` in the repo, validated against `catalog/package.schema.json`.

Consumers in the table: **S** = CDK synth, **A** = `api`, **P** = host (`prepare` and the launcher), **G** = the agent daemon, **U** = the UI, through `GET /packages`.

| Field | Type and constraint | Used by |
|---|---|---|
| `id` | `^[a-z0-9][a-z0-9-]{0,31}$`, unique, not on the reserved list. Permanent: it is also the server id, `game` tag value, DNS label, world directory and SSM key | S A P |
| `name`, `description` | 1–64 and ≤ 500 characters | U |
| `attribution[]` | Source and licence of borrowed material | U, review |
| `acknowledgements[]?` | `{id, text, url}`. An admin accepts each one at create | A U |
| `diskEstimateGiB` | Integer 1–50: game files, a starting world and the image | S (disk assertion) |
| `runtime.image` | `repo:tag@sha256:<digest>` | S P |
| `runtime.dataMountPath` | Absolute container path | P |
| `runtime.dataOwner` | `{uid, gid}`, 1–65534. Never root, and never the host's pinned `ssm-user` uid (C3) | P |
| `runtime.ports[]` | `{port, protocol: udp\|tcp, exposure: public\|admin}`, at least one public | S P G A |
| `runtime.query?`, `runtime.egressPorts[]?`, `runtime.capAdd[]?` | As in ADR-0002 | S P |
| `runtime.env` | `{NAME: value}` operational constants, including every image control variable (paths, startup command, auth state). Names match `^[A-Z_][A-Z0-9_]*$`. Values contain no `${` and no `{{`, because some entrypoints expand them (the yolk does, from any env var) | P |
| `runtime.stopTimeoutSeconds` | 1–120, default 60 | P |
| `runtime.updates` | `none` (version pinned) or `on-start` (the image updates the game at every start) | U, review |
| `signals.readyPattern` | Required. RE2, ≤ 300 characters, and every top-level alternative anchored with `^` (CI checks the parsed syntax tree, not the first character) | G |
| `signals.authRequiredPattern?` | As above | G |
| `signals.readyTimeoutSeconds?` | 300–3600, default 1800 | G, reconcile rule 6 |
| `signals.idleDefaults?` | `{emptyMinutes, noJoinMinutes, peerMinPackets}`, within the lifecycle bounds below | G A |
| `files[]` | `{id, path, format: "json"}`. `path` is relative to the data mount, has no `..`, and is ≤ 200 characters. Within one file, pointers are non-empty and none equals or is a prefix of another | P |
| `settings[]` | See "Settings vocabulary" | A P U |

**Projection:** CDK publishes each manifest at `/bradfordly-games/packages/<id>` as a standard String parameter.

- It drops the UI-only fields: `name`, `description`, `attribution`, the acknowledgement text, and each setting's `label`, `description`, `group`, `widget` and `unit`.
- Synth asserts that it's at most 4096 bytes and contains no `{{` [23]. Exceeding that needs an explicit Advanced opt-in in the PR ($0.05/month).
- Synth also asserts that the package's worst-case server record fits in 4096 bytes: 6 bytes for each character of every `maxLength` (escaped worst case), plus keys and overhead.

### Settings vocabulary

| Field | Meaning |
|---|---|
| `key` | `^[a-z][a-zA-Z0-9]{0,31}$`, unique in the package, and not one of the lifecycle keys below. The name used in the server record |
| `label`, `description?` | Form text, ≤ 64 and ≤ 300 characters |
| `group` | Form section. Array order is display order |
| `widget?` | Strings only. `password`: masked in the form, still readable by admins. `textarea`: multi-line display only (values still can't contain newlines) |
| `unit?` | Suffix shown after numbers, ≤ 16 characters |
| `schema` | A JSON Schema 2020-12 fragment, limited to: `type` (`string`, `integer`, `number`, `boolean`; **always required**), `oneOf` of `{const, title}` (choices; consts unique and of that `type`), `minimum`, `maximum`, `minLength`, `maxLength` (required for free strings, ≤ 256), `pattern` (RE2 syntax only) and `default` (**required** unless `secret`) |
| `target` | Exactly one of two forms (below) |
| `secret?` | A credential: stored only as a SecureString, write-only, `env` targets only, no `default` |
| `required?` | Only together with `secret`: a start fails closed while the secret is unset |

The two `target` forms:

- **`{env: NAME, true?, false?}`:**
  - Booleans become `true` or `false`, unless the `true`/`false` pair is given. The pair is all-or-nothing, and only for booleans.
  - Integers are written in plain decimal, numbers with Go's shortest form (`FormatFloat(v, 'f', -1, 64)`), and strings verbatim.
  - `NAME` is unique across all settings and `runtime.env`, and isn't on the meta-schema's denylist of interpreter and loader controls (`STARTUP`, `STARTUP_JSON`, `LD_*`, `JAVA_TOOL_OPTIONS`, `JDK_JAVA_OPTIONS`, `_JAVA_OPTIONS`, `PATH`, `HOME`).
- **`{file: <files[].id>, pointer: <RFC 6901>}`:** JSON types are preserved. `api` stores integers canonically, so `8.0` is stored and written as `8`.

Every string value, in every route and on the host, must contain no control characters and no `{{`. The first rule protects env-file lines, the second SSM [23].

Nothing in this system logs or returns a setting value in an error. The shared Go package reports validation failures as `{settingKey, keyword}` only, because the validator's own messages quote the value [31].

**Lifecycle settings on every server** (group "Lifecycle"; these are ADR-0003's overrides):

| Setting | Range | Default |
|---|---|---|
| `idleEmptyMinutes` | 5–60 | Package `idleDefaults`, else ADR-0003's 15 |
| `idleNoJoinMinutes` | 10–60 | Package `idleDefaults`, else 20 |
| `peerMinPackets` | 10–1000 | Package `idleDefaults`, else 60 |

A server record stores a lifecycle value only when an admin overrides it. The agent resolves each one at read time: the record, then the package's `idleDefaults`, then the unit's env var. So a later PR that changes `idleDefaults` reaches the existing server.

The values schema is `{type: object, properties: {<key>: <fragment>}, required: [<every non-secret key>], additionalProperties: false}`. Ajv, the Go validator and any form renderer read the same fragments, and a Go+htmx page or a single-page app renders four input kinds from `type`, `oneOf` and `widget`.

### Worked example: Hytale

```json
{
  "id": "hytale",
  "name": "Hytale",
  "description": "Hytale dedicated server, release patchline. Updates to the current release at every start.",
  "attribution": [
    "Image and launch settings: pelican-eggs/yolks games/hytale and the pelican-eggs Hytale egg (MIT)",
    "Log patterns and settings map: CubeCoders/AMPTemplates hytale (MIT)"
  ],
  "acknowledgements": [
    { "id": "hytale-server-policies", "text": "I have read the Hytale server operator policies.", "url": "https://hytale.com/server-policies" }
  ],
  "diskEstimateGiB": 6,
  "runtime": {
    "image": "ghcr.io/pelican-eggs/games:hytale@sha256:<digest>",
    "dataMountPath": "/home/container",
    "dataOwner": { "uid": 1001, "gid": 1001 },
    "ports": [{ "port": 5520, "protocol": "udp", "exposure": "public" }],
    "stopTimeoutSeconds": 90,
    "updates": "on-start",
    "env": {
      "STARTUP": "java -Xms128M -XX:MaxRAMPercentage=75 -jar HytaleServer.jar --bind 0.0.0.0:5520 --assets ../Assets.zip",
      "AUTO_UPDATE": "1",
      "PATCHLINE": "release",
      "HYTALE_API_AUTH": "1",
      "HYTALE_DISABLE_UPDATES": "1",
      "ENABLE_WORLD_BACKUP": "0",
      "ENABLE_SERVER_BACKUP": "0",
      "USE_AOT_CACHE": "1"
    }
  },
  "signals": {
    "readyPattern": "^\\[[0-9/]+ [0-9:]+\\s+INFO\\]\\s+\\[HytaleServer\\]\\s+Hytale Server Booted!",
    "authRequiredPattern": "^(\\s*Please visit: https://oauth\\.accounts\\.hytale\\.com/|\\[[^\\]]+\\]\\s+\\[HytaleServer\\]\\s+No server tokens configured)",
    "readyTimeoutSeconds": 2700
  },
  "files": [{ "id": "server", "path": "Server/config.json", "format": "json" }],
  "settings": [
    { "key": "serverName", "label": "Server name", "group": "General",
      "target": { "file": "server", "pointer": "/ServerName" },
      "schema": { "type": "string", "minLength": 1, "maxLength": 64, "default": "Bradfordly Hytale" } },
    { "key": "motd", "label": "Message of the day", "group": "General",
      "target": { "file": "server", "pointer": "/MOTD" },
      "schema": { "type": "string", "maxLength": 200, "default": "" } },
    { "key": "maxPlayers", "label": "Player limit", "unit": "players", "group": "General",
      "target": { "file": "server", "pointer": "/MaxPlayers" },
      "schema": { "type": "integer", "minimum": 1, "maximum": 10, "default": 10 } },
    { "key": "password", "label": "Join password", "group": "Access", "widget": "password",
      "description": "Share it with players. Empty means no password.",
      "target": { "file": "server", "pointer": "/Password" },
      "schema": { "type": "string", "maxLength": 64, "default": "" } },
    { "key": "allowOp", "label": "Allow /op self", "group": "Access",
      "target": { "env": "ALLOW_OP", "true": "1", "false": "0" },
      "schema": { "type": "boolean", "default": false } },
    { "key": "gameMode", "label": "Default game mode", "group": "Gameplay",
      "target": { "file": "server", "pointer": "/Defaults/GameMode" },
      "schema": { "type": "string",
                  "oneOf": [{ "const": "Adventure", "title": "Adventure" }, { "const": "Creative", "title": "Creative" }],
                  "default": "Adventure" } },
    { "key": "viewRadius", "label": "Maximum view radius", "unit": "chunks", "group": "Performance",
      "target": { "file": "server", "pointer": "/MaxViewRadius" },
      "schema": { "type": "integer", "minimum": 4, "maximum": 12, "default": 12 } },
    { "key": "ignoreBrokenMods", "label": "Start even if a mod fails to load", "group": "Mods",
      "target": { "env": "IGNORE_BROKEN_MODS", "true": "1", "false": "0" },
      "schema": { "type": "boolean", "default": false } }
  ]
}
```

Notes:

- **Values to confirm at first deploy:** `dataOwner`, `diskEstimateGiB`, `MaxRAMPercentage` (within ADR-0002's limit of host RAM minus 1 GiB) and both patterns.
- **The patterns** adapt AMP's, and match the yolk's device prompt and the server's own "no tokens" line.
- **Operational env vars are fixed by the package, not exposed as settings:** updates, patchline, backups and the AOT cache. `ACCEPT_EARLY_PLUGINS` (unsupported binary injection) is left out deliberately.
- **Patchline:** the yolk lets `Update.Patchline` in `Server/config.json` override `PATCHLINE`. A hand edit to `pre-release` would move the server off the release client's version. The first deploy checks this. If it matters, the key becomes a managed setting with the single choice `release`.
- **Updates `on-start`:** exact protocol matching means each start must take the current release.
  - The yolk downloads it.
  - `HYTALE_DISABLE_UPDATES=1` keeps the in-game updater from exiting with code 8 mid-session, which ADR-0003 would treat as `game-exited`.
- **Hytale's own backups are off:** DLM snapshots cover worlds, and the disk is needed.
- **Hytale's OAuth tokens** live in `/home/container/.hytale-auth.json`.
  - They sit on the world volume and in snapshots (`aws/ebs`, public sharing blocked), readable by the game and its mods.
  - This is accepted: they are the game's credentials, not ours.
- **Re-auth (O12):**
  - `authRequiredPattern` raises `Game Auth Required`, and Discord gets its fixed message.
  - The admin reads the code with `journalctl CONTAINER_NAME=game-<id>` over Session Manager and approves it within 15 minutes.
  - `readyTimeoutSeconds` of 2700 covers the download, that window and the boot.
  - Any start within 30 days renews the refresh token [2].

### The format generalises: Minecraft (illustrative, not onboarded)

itzg maps env to `server.properties` [11], so the package needs no `files[]`:

```json
{
  "id": "minecraft",
  "acknowledgements": [{ "id": "minecraft-eula", "text": "I accept the Minecraft EULA.", "url": "https://aka.ms/MinecraftEULA" }],
  "runtime": {
    "image": "itzg/minecraft-server:<tag>@sha256:<digest>",
    "dataMountPath": "/data",
    "dataOwner": { "uid": 1000, "gid": 1000 },
    "ports": [{ "port": 25565, "protocol": "tcp", "exposure": "public" }],
    "env": { "EULA": "TRUE", "VERSION": "<pinned>" },
    "updates": "none"
  },
  "signals": { "readyPattern": "^\\[[0-9:]+\\] \\[Server thread/INFO\\]: Done \\(" },
  "files": [],
  "settings": [
    { "key": "motd", "label": "MOTD", "group": "General",
      "target": { "env": "MOTD" }, "schema": { "type": "string", "maxLength": 64, "default": "" } },
    { "key": "difficulty", "label": "Difficulty", "group": "Gameplay", "target": { "env": "DIFFICULTY" },
      "schema": { "type": "string", "oneOf": [{ "const": "peaceful", "title": "Peaceful" }, { "const": "easy", "title": "Easy" },
                            { "const": "normal", "title": "Normal" }, { "const": "hard", "title": "Hard" }], "default": "easy" } },
    { "key": "rconPassword", "label": "RCON password", "group": "Admin", "secret": true, "required": true,
      "target": { "env": "RCON_PASSWORD" }, "schema": { "type": "string", "minLength": 12, "maxLength": 64 } }
  ]
}
```

Palworld (thijsvanloef), Valheim (lloesche) and Foundry (felddy) fit the same way. Foundry's licence key and account password become `secret` settings.

### Server records and secrets

`/bradfordly-games/servers/<id>`, where `<id>` is the package id. It's a standard String, written by `api`, at most one per package:

```json
{
  "v": 1,
  "createdAt": "2026-10-13T18:00:00Z", "createdBy": "brad",
  "acknowledged": { "hytale-server-policies": "2026-10-13T18:00:00Z" },
  "values": { "serverName": "Bradfordly", "motd": "", "maxPlayers": 8, "password": "…",
              "allowOp": false, "gameMode": "Adventure", "viewRadius": 10, "ignoreBrokenMods": false },
  "lifecycle": { "idleEmptyMinutes": 30 },
  "secretsSet": []
}
```

How values behave:

- Defaults are copied into `values` at create. `lifecycle` holds only admin overrides (see "Settings vocabulary").
- A key added to the package later takes its package default, both in `api` and when `prepare` fills absent keys.
- Unknown keys are ignored by the host and never returned by `GET /servers/{id}/config`, so the next save drops them.
- A stored value that fails the current package schema fails closed (`server-invalid`) until an admin saves it again.

Credentials live at `/bradfordly-games/secrets/<id>/<key>` (standard SecureString, `aws/ssm`), one parameter per `secret` setting. They get their own top-level prefix, so that no grant on `servers/…` can reach them.

- `secretsSet` is a hint for the form ("set" or "not set") and nothing more. `prepare` checks that the parameter itself exists.
- A stored secret that fails the current package schema (a PR tightened it) is reported as `secret-missing`: re-enter it. That's the only fix, because the API can't read it back.

### Storage and delivery

| Data | Where | Written by | Read by |
|---|---|---|---|
| Manifests and meta-schema | `catalog/*.json` in the repo | Reviewed PR | Synth, Go tests |
| Full manifests | Copied into the `api` Lambda asset at synth | CDK | `api` (forms, validation) |
| Package projections | `/bradfordly-games/packages/<id>`, standard String, ≤ 4 KB | CDK | Instance, `api`, `events` |
| Server records | `/bradfordly-games/servers/<id>` (at most one per package), standard String | `api` | Instance, `api`, `events` |
| Credentials | `/bradfordly-games/secrets/<id>/<key>`, standard SecureString | `api` (write-only) | Instance only |
| Data-volume ID | Written by user data to `/etc/bradfordly-games/data-volume-id` | CDK | `prepare` |
| Env-file | `/run/bradfordly-games/game.env` (tmpfs, root, mode 0600) | `prepare` | `docker run` |

- **Server lookup** (ADR-0003) means: the server record exists; the hostname is `<id>.games.bradfordly.com`; the ports are the projection's public ports.
- **Exact names only.** Every package id and secret key is known at synth, so every role reads and writes enumerated parameter names with `GetParameter(s)` and `PutParameter`. No role uses `GetParametersByPath` on these paths.
- **`/bradfordly-games/control/*`** stays outside every name the instance can read.
- **The data-volume ID** changes only when ADR-0002's restore-a-snapshot runbook swaps the volume. That runbook now also updates the pin, which changes user data and restarts the instance, as a restore does anyway.
- **Accepted exposure:** credentials passed with `--env-file` are kept in the container's config under `/srv/docker`. A daily snapshot taken during a session captures them, and freed blocks can persist into later snapshots. This is the same exposure ADR-0002 accepted for world data and this ADR accepts for Hytale's tokens: snapshots are encrypted with `aws/ebs`, and snapshot public access is blocked.

### Validation

| Where | With | Checks |
|---|---|---|
| Synth | TypeScript, Ajv (2020-12) | Manifests against the meta-schema; defaults against their fragments; key, env-name and pointer uniqueness and prefix rules; env names off the denylist and not overlapping `runtime.env`; no `${` or `{{` in `runtime.env`; id and path rules; package ids not on the reserved list; `dataOwner` isn't root or the pinned `ssm-user` uid; digest pin; at least one public port; projection ≤ 4096 bytes with no `{{`; worst-case server record ≤ 4096 bytes; the sum of every package's `diskEstimateGiB` ≤ 80% of the data volume's size; no generated parameter ARN contains `//`. The security group is built from the union of public ports, and the grants' names, tag values and record names from the package ids and secret keys |
| CI | Go, `santhosh-tekuri/jsonschema/v6` | The same meta-schema. Every default and const passes the Go validator. Every pattern compiles with Go `regexp`, every top-level alternative of its parsed syntax tree starts with a line or text anchor, it matches its captured sample lines and it rejects chat samples. Golden write tests per file, including the hostile cases in Follow-up 3. A failing secret's error contains none of its bytes |
| `api` | Go, same library | Values on every write, with the values schema's `required` and `additionalProperties: false`; string rules; integer canonicalisation; the package exists in the bundled catalog; acknowledgements |
| `prepare` (host) | Go, same library | Re-validates the record and secrets against the projection, because they were written by an internet-facing function. Fails closed |
| Agent daemon | Go, same library | Validates the record's `lifecycle` overrides; a bad value falls back to the package default and is logged by key |

One shared Go package serves `api`, `events` (lookup) and the agent. That is ADR-0001's third occurrence.

### `prepare` and the writer

The launcher keeps ADR-0002's mount, tag and IMDS checks. It then runs `prepare` as root, inside invariant 3's strict-mode rules:

```bash
if ! reason=$(timeout 120 /usr/local/lib/bradfordly-games/game-agent prepare "$id"); then
  enter_admin_mode "$reason"  # checks the token against the enum; empty or unknown becomes prepare-failed
fi
```

- `prepare` prints only a reason token on stdout. All logs, including the writer's, go to stderr (and so to the journal).
- Exit 0 means run the game. Anything else means admin mode: a non-zero exit, the timeout, a crash, or a missing or unverified binary (ADR-0003 A3 deletes one). An empty or unknown token becomes `prepare-failed`. The launcher writes `admin-reason`, and stays that file's only writer.
- On success, the launcher runs `docker run` as before, with the image, ports and mount taken from the projection.

What `prepare` does:

1. **Read inputs:** the projection, the server record and the package's secret parameters, by exact name with `GetParameters`, decrypted.
   - Each call has a 10 s timeout and up to 4 attempts with jittered backoff.
   - `ParameterNotFound` for the record or projection, or a record that fails validation, gives `server-invalid`. A missing, or now invalid, required secret gives `secret-missing`. Any other error (throttling, credentials not ready after a reboot) gives `prepare-failed`, so a healthy record is never reported as invalid.
2. **Decode once, then validate.** The record is decoded a single time, duplicate keys are rejected, and that same decoded object is validated and used. Absent keys are filled from the projection's defaults. Env names come only from the projection.
3. **World directory:**
   - Compare the pinned data-volume ID, with its hyphen removed, against the serial of the disk mounted at `/srv` (`findmnt -no SOURCE /srv`, then `/sys/block/<disk>/device/serial`) [25]. On a mismatch, fail with `volume-mismatch`.
   - If `/srv/games/<id>` is absent, create it (mode 0750, owned by `dataOwner`).
   - If it exists but isn't a directory, or its owner differs from `dataOwner`, fail with `prepare-failed`. The log names the cause (`owner-mismatch`), and the `chown` runbook fixes it.
4. **Env-file:** write `game.env` from `runtime.env`, then the env-targeted settings and secrets. The writer itself rejects `\r`, `\n` and NUL, whatever the schema allows.
5. **Managed files:** for each file with managed keys, run `game-agent write-file` in a transient sandbox, with the patch on stdin:

   ```
   systemd-run --pipe --wait --collect --uid=<uid> --gid=<gid> \
     -p NoNewPrivileges=yes -p PrivateNetwork=yes -p ProtectSystem=strict \
     -p ReadWritePaths=/srv/games/<id> -p MemoryMax=256M -p RuntimeMaxSec=30 \
     /usr/local/lib/bradfordly-games/game-agent write-file ...
   ```

   `PrivateNetwork` also keeps this host process away from IMDS, which ADR-0002's `DOCKER-USER` rule covers only for containers. Any failure, including the time limit, is `prepare-failed`.

What the writer does:

- Opens the data directory and everything below it with `openat2(RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS)`, creating missing parent directories beneath it. The target file is opened `O_RDONLY|O_NONBLOCK|O_NOCTTY|O_CLOEXEC`, so a FIFO can't block it.
- After opening, `fstat` must show a regular file with exactly one link and at most 1 MiB. A symlink (ELOOP), a hard link, a FIFO, a device or an oversized file fails closed. If the file is absent, the writer starts from `{}`.
- Reads through a size-limited reader. It parses a JSON object with nesting capped at 64, and rejects duplicate keys.
- Keeps every unmanaged value as its raw token, so large integers and number formats survive, and keeps key order.
- Sets each pointer, creating intermediate objects, and fails on an intermediate that isn't an object.
- Writes a temp file in the same directory, `fsync`s it and `renameat`s it over the target.
- Logs `override <file> <key>` when a value changed. It never logs values, old or new.

The world directory is writable by an untrusted container. These rules keep a planted link, FIFO or oversized file from turning `prepare` into a write elsewhere on the host, a hang, or memory exhaustion. Hard links can't leave the container's own bind mount anyway, because `/srv` is a separate filesystem.

### Desired state while the host is stopped

- The server record holds the desired values, and the UI never reads live files.
- **The form always wins (O8):**
  - Every start rewrites managed keys from the stored values, so in-game or hand edits to those keys revert at the next start.
  - The UI marks managed fields as "applied at every start".
- Unmanaged keys, other files and world data are never touched. Admins edit them over Session Manager, in ADR-0002 admin mode or under `game-agent hold`.
- **Games that rewrite their config while running** (Hytale [1]) do no harm: writes happen only while the container is stopped, and the game reads our values at start.
- **First start:** a missing file is created with only the managed keys, and the game fills in the rest (a first-deploy test).
- **Malformed file:** it fails closed (`prepare-failed`) and is never overwritten.
- **Edits saved while a server runs** take effect at its next start, and the API response says so.

### One server per game (O11)

- **Identity:** a game's server uses the package id everywhere: the server record, the secrets prefix, the world directory, the `game` tag, the DNS name and the container name. There is no separate server id or display name; the UI shows the manifest `name`.
- **Create** provisions the package's only server. `PutParameter` with `Overwrite=false` makes a second create fail with 409 `exists`, so the rule needs no counter.
- **Disk:** synth asserts that the sum of every package's `diskEstimateGiB` is at most 80% of the data volume's size. It counts packages that have no server yet, so it's conservative. A PR that breaks it grows the volume in the same PR. It's an estimate, not a quota: a mod that fills its world can still fill `/srv` (see Revisit if).
- **Reserved names:** a reserved list in CDK context (it includes 0006's frontend label) is checked against package ids at synth. There's no runtime check and no IAM Deny.
- **Reset or retire:** not offered in the UI. The runbook stops the host, deletes the server record and its secret parameters, and removes `/srv/games/<id>` over Session Manager. To retire a package, run it before the PR that removes the package from `catalog/`. Otherwise the record, secrets and directory are orphaned.
- **Ids are permanent:** renaming a package strands its record, secrets and world.

### API (admin routes)

| Route | Level | Behaviour |
|---|---|---|
| `GET /packages` | admin | Form metadata from the bundled manifests: labels, groups, widgets, fragments, acknowledgements, ports |
| `POST /servers` | admin | Body `{package, values, acknowledged}`. Checks that the package exists, every acknowledgement and the values, then calls `PutParameter` at `servers/<package>` with `Overwrite=false`. `ParameterAlreadyExists` returns 409 `exists`, which keeps each game to one server. Success returns 201 |
| `GET /servers/{id}/config` | admin | Values (only keys in the current package), lifecycle overrides, `secretsSet`, managed markers and an `etag`: a hash of `{values, lifecycle}`. Never returns a credential |
| `PUT /servers/{id}/config` | admin | Body `{etag, values, lifecycle}`, a full replacement of those two fields. It reads the record fresh, returns 409 `stale` if the hash differs, and otherwise writes onto the fresh read, so a secret write in between doesn't make the form stale. SSM has no compare-and-swap [23], which is acceptable with one admin. Applies at the next start |
| `PUT /servers/{id}/secrets/{key}` | admin | Body `{value}`, for a `secret` setting only. 404 if the server doesn't exist yet. Writes the SecureString, then updates the `secretsSet` hint. Returns 204 and never echoes the value |
| `DELETE /servers/{id}/secrets/{key}` | admin | Deletes the secret and updates the `secretsSet` hint. Returns 204 |

- Bodies are JSON of at most 16 KB. Unknown fields are rejected with 400 and the offending field paths, never their values.
- Every `PutParameter` sends `Tier: Standard`, so an account default can't silently make a parameter Advanced. An SSM `ValidationException` maps to 400.
- New 409 reasons: `exists` and `stale`.
- Members keep ADR-0003's routes. `GET /servers` lists every catalog package with its manifest `name`, whether its server exists, and which one is active.

### IAM

Every SSM name below is enumerated by CDK from `catalog/`: `packages/<id>` and `servers/<id>` for each package, and `secrets/<id>/<key>` for each `secret` setting. Hytale has no secret settings, so nothing gets a secrets grant for it.

| Role | Change |
|---|---|
| `api` | `ssm:GetParameter` and `ssm:GetParameters` on the `packages` and `servers` names, replacing the catalog path. No `GetParametersByPath` |
| | `ssm:PutParameter` on the `servers` and `secrets` names, and `ssm:DeleteParameter` on the `secrets` names. No other SSM writes |
| | Credentials are write-only (O10) by two explicit Denies: `ssm:GetParameter*` with `NotResource` set to the `packages` and `servers` names, which covers every secret and every ancestor path up to `/` [29], including `GetParameterHistory` [30]; and `kms:Decrypt` on `*` when `kms:ViaService` is `ssm.us-east-1.amazonaws.com`, because the `aws/ssm` key lets every principal decrypt [27] |
| | No KMS Allow: writing through `aws/ssm` needs none [27] (first-deploy test) |
| `events` | `ssm:GetParameter` and `ssm:GetParameters` on the `packages` and `servers` names (lookup), with the same two Denies. No writes. The SSM writes stay on `api`'s role, outside ADR-0003's shared lifecycle grant set |
| Instance | `ssm:GetParameter` and `ssm:GetParameters` on the `packages`, `servers` and `secrets` names, replacing `games/*`. No KMS statement [27] |
| Lifecycle grants (`api`, `events`) | Unchanged from ADR-0002. `ec2:CreateTags` keeps `StringEquals aws:RequestTag/game` set to the package ids, plus both `aws:TagKeys` conditions. Route 53 keeps `ForAllValues:StringEquals` on `<id>.games.bradfordly.com` for each package id. CDK now generates both lists from `catalog/`, so a package PR widens them on deploy without an instance restart |

**"Never granted":** nothing on ADR-0002's list is used, and C11 adds to it.

- The host pulls its config at boot, so there is no `ssm:SendCommand`.
- There is no `iam:PassRole`, and no instance, metadata, volume or snapshot change.
- A first-deploy test runs `aws iam simulate-principal-policy` for `api` and `events`. It must show `explicitDeny` for `GetParametersByPath` on `/`, `/bradfordly-games` and `/bradfordly-games/secrets`, for `GetParameterHistory` on a secret, and for `kms:Decrypt` through SSM.

### How to add a game package

This replaces ADR-0002's onboarding checklist.

1. **Id:** pick the package `id`. It's permanent, because it's also the server id, tag value and DNS label, and it must not be on the reserved list.
2. **Image:** choose a self-installing amd64 image with an acceptable licence that installs or updates the game itself, and either maps env vars to its config or uses JSON config. Pin it by digest, and verify that digest against the publisher's build (its workflow run, or `gh attestation verify` where attestations exist); a `source` label alone can be forged. Check that Trivy is clean.
3. **Image facts:** record `dataMountPath`, `dataOwner` (run `id` in the image) and `STOPSIGNAL`.
4. **Network and privileges:** declare ports and their exposure, and any egress beyond 443. Use bridge networking, and capabilities only from ADR-0002's allowlist.
5. **Updates:** pin the version through env (`updates: none`), or declare `on-start` with a reason and make sure no in-game updater can end a session.
6. **Settings:**
   - env targets where the image maps env, and JSON file targets otherwise;
   - credentials as `secret` env targets;
   - operational constants in `runtime.env`, including every image control variable (paths, startup command, auth state), so no setting can target one;
   - audit the entrypoint for how it uses env vars (`eval`, a shell, template expansion such as the yolk's `${VAR}` in `STARTUP`), and keep free-text settings away from any variable it expands;
   - don't expose settings that change how the server runs (updates, backups, binary plugins).
7. **Signals:** boot once and capture the logs. Write anchored patterns, commit the captured lines (and a chat line) as Go test samples, and set `readyTimeoutSeconds`.
8. **Saving:** turn autosave on, then run the stop test: change the world, stop through the normal path, start, and check. Set `stopTimeoutSeconds`.
9. **Measurements:** measure disk use for `diskEstimateGiB`, and packet rates for `idleDefaults.peerMinPackets`.
10. **Write tests:** add a golden write test per file, including a first start with the file absent.
11. **Provenance:** record mod checksums, and credit borrowed material in `attribution`.
12. **Review:** synth and the Go tests pass (projection ≤ 4 KB; the disk sum fits, or the same PR grows the volume), and code-reviewer and security-reviewer run on the PR. Deploying a new package widens the control-plane grants and needs no instance restart.

### Cost

- **Idle impact: $0.**
  - Standard parameters (packages, one record per package and a few secrets) are free [22].
  - KMS calls (a few per start and per secret write) stay inside the 20,000 free requests a month (ADR-0003 [21]).
  - Bundling manifests into `api` changes nothing.
- **Disk is the lever:**
  - Hytale needs about 6 GiB (estimate). That fits in the 16 GiB the 80% rule allows on the 20 GiB volume, leaving about 10 GiB for other packages.
  - Later packages may grow the volume by PR, at up to $0.13 per GiB-month (ADR-0002, worst case).
  - The synth assertion protects saves from a full disk, and any growth shows up in PR review.

## Amends ADR-0002

ADR-0002's status line records these amendments.

- **C1. Decision and Game catalog.**
  - Old: "game definitions from a TypeScript catalog published to SSM Parameter Store", one parameter per game at `/bradfordly-games/games/<id>`, and the listed value shape.
  - New:
    - Reviewed JSON package manifests in `catalog/` are published as projections at `/bradfordly-games/packages/<id>`. Each package has at most one server record, at `/bradfordly-games/servers/<id>`, keyed by its package id. The shape is this ADR's manifest.
    - `env` splits into `runtime.env` and settings, and `requiresSecrets` becomes `secret`/`required` settings.
    - New fields: `dataOwner`, `updates`, `signals`, `files`, `settings`, `acknowledgements` and `diskEstimateGiB`.
    - This supersedes ADR-0003's A6 interim `readyPattern`.
- **C2. Data volume layout.** `/srv/secrets/` is retired. Credentials live in SSM and reach the container through `/run/bradfordly-games/game.env`. Docker keeps them in the container config under `/srv/docker`, so they can appear in snapshots (accepted; see "Storage and delivery").
- **C3. Invariant 1 (boot) and user data.**
  - The launcher runs `game-agent prepare <id>` after its own checks and before `docker run`, with the exit and timeout contract in "`prepare` and the writer".
  - User data writes the data-volume ID to `/etc/bradfordly-games/data-volume-id`. The restore-a-snapshot runbook updates it.
  - User data pre-creates `ssm-user` with a fixed uid (a CDK constant, which synth forbids as a `dataOwner`) and writes its sudoers entry. SSM Agent writes that entry only when it creates the user itself. Without this, `ssm-user` would likely get uid 1001, the same as the Hytale yolk's `container` user.
- **C4. Invariant 2 (fail closed).**
  - "GetParameter for that id fails" becomes `server-invalid`.
  - "/srv/games/<id> doesn't already exist (the launcher never creates it)" becomes: `prepare` creates it only on the pinned volume, otherwise `volume-mismatch`.
  - The `/srv/secrets/<id>.env` check becomes `secret-missing`.
  - New: `prepare-failed` (including a timeout, a crash or a world directory not owned by `dataOwner`).
  - The other checks are unchanged.
- **C5. Invariant 4 (container template).**
  - Old: `[--env-file /srv/secrets/<id>.env] [-e KEY=value …]`.
  - New: `--env-file /run/bradfordly-games/game.env`. Everything else is unchanged.
- **C6. Instance IAM.**
  - Old: `ssm:GetParameter` on `games/*`.
  - New: as in this ADR's IAM table. `/bradfordly-games/control/*` stays outside.
- **C7. Contract → Configuration.** The catalog parameter path becomes the packages and servers paths. Record names are unchanged, because the ids are package ids.
- **C8. Contract → Grants.**
  - Tag values and record names stay enumerated. CDK generates them from the `catalog/` package ids.
  - `api` and `events` get the SSM grants above.
- **C9. Follow-ups → per-game onboarding checklist.**
  - Replaced by "How to add a game package".
  - `updates: on-start` is allowed when declared, provided no in-game updater can end a session.
- **C10. Context and Revisit if (O11).**
  - Old: "Several game servers/worlds, one running at a time", and the revisit trigger "Two games must run at once".
  - New: several games, with exactly one server (one world) per game, ever. The game id is the server id, and only one server runs at a time. The trigger moves to this ADR's "Revisit if", reworded as a change of the owner's rule.
- **C11. Contract → Never granted.** Adds, for every control-plane role:
  - SSM writes on `packages/*`, which CDK alone owns and the host trusts for images, capabilities and paths;
  - `ssm:DeleteParameters` and `ssm:PutResourcePolicy`;
  - for `api` and `events`, any SSM read of a `secrets` name or an ancestor path, and `kms:Decrypt` through SSM. ADR-0003's notifier keeps reading its one webhook parameter.

## Amends ADR-0003

ADR-0003's status line records these amendments.

- **D1. Start step 1 and server lookup.** The interim catalog becomes the server record plus the package projection. The lookup id is the package id, and `exists` means its server record exists.
- **D2. API.**
  - "There are no request bodies" becomes: admin routes take JSON bodies.
  - The row "Create, edit, config and secrets | admin | Defined by 0004" becomes the admin routes above.
  - `GET /servers` lists catalog packages, whether each has a server, and which one is active.
  - Responses add 201 (created), 204 (secret written or deleted) and 400 (invalid body), and the 409 reasons `exists` and `stale`.
- **D3. Host agent → Inputs and required package fields.**
  - "The catalog parameter" becomes the package projection and the server record.
  - Patterns are RE2, matched on ANSI-stripped lines.
  - The unit's `IDLE_*` and `PEER_MIN_PACKETS` variables become fallbacks behind per-server `lifecycle` values and package defaults.
  - `readyTimeoutSeconds` comes from the package.
  - A6's interim field and its substring matching are removed. `readyPattern` is now required, so the `unverified` basis and its 60 s fallback are removed too.
  - The daemon validates `lifecycle` overrides with the shared package, and falls back to the package default on a bad value.
- **D4. Agent commands and admin reasons.**
  - The binary gains `prepare` and `write-file`. The launcher runs them, not the daemon unit.
  - The launcher stays `admin-reason`'s only writer. `prepare` is the only writer of `/run/bradfordly-games/game.env` (root, 0600).
  - `Host Admin Mode` gains the reasons `server-invalid`, `secret-missing`, `volume-mismatch` and `prepare-failed`, which `hostevents` and the notifier must accept.
- **D5. IAM.**
  - "Left to later ADRs → 0004" is resolved by keeping the enumerated tag values and record names, generated from `catalog/` (C8), rather than the wildcards ADR-0003 anticipated.
  - `api` and `events` get the SSM reads and Denies above. Only `api` gets SSM writes, outside the shared lifecycle grant set.
  - ADR-0003's open question is settled: the notifier needs no `kms:Decrypt` statement to read its `aws/ssm` webhook parameter [27].
- **D6. Interfaces for later ADRs → 0004.** Fulfilled.
- **D7. Context → owner decisions (O11).**
  - Old: "(2026-10-12) Worlds are created in the UI from existing packages", and "Group `admin` creates servers".
  - New: each game has exactly one server, provisioned in the UI from its package. The server id is the package id, and only one server runs at a time. ADR-0003's single item, lease lock and `other-server-active` rule already assume this, so its design is unchanged.

## Consequences

Positive:

- **One blueprint for any self-installing image:** env settings for images that map env, JSON file settings for the rest. Both are typed, and validated from one schema by synth, CI, the API and the host.
- **Owner experience:** the convenience the owner asked for. File settings get detailed forms while the host is stopped, without reading live files.
- **Trust boundary:**
  - Images, ports, env names, file paths and patterns come only from reviewed PRs, through CDK.
  - The web writes values only, and the host re-validates them.
  - Credentials are write-only by explicit IAM Denies that cover every read path, including path reads, history and KMS.
- **Cost and footprint:** idle cost is $0, nothing new runs on the host, and the pinned agent binary and its release pipeline are reused.
- **Tight grants:** every id, tag value and DNS name is known at synth, because each game has one server. ADR-0002's enumerated grants stay.
- **Hytale:** runs with no stdin and no console automation.

Negative:

- **Security-sensitive code:** `prepare` and `write-file` are ours to maintain and test, against hostile files.
- **Managed keys revert:** in-game or hand changes to them are undone at the next start.
- **Agent releases:** a new codec or a writer fix needs one, and that restarts the instance (ADR-0003 A3).
- **Hytale:**
  - The game updates at every start, which lengthens cold starts; a Hytale CDN outage blocks starts.
  - A third-party entrypoint holds the game account's tokens.
  - Tokens and the join password sit on the volume and in snapshots, and so do env-file credentials in Docker's container config (accepted).
  - Re-auth needs an admin within 15 minutes.
- **Disk:** the synth rule reserves space for every package, including ones without a server yet, so adding packages may force a volume grow.
- **Permanent ids:** renaming a package strands its record, secrets and world.
- **Concurrent edits:** config updates have no compare-and-swap, only a read-then-write hash check; this is acceptable with one admin.
- **Disk is estimated, not enforced:** a mod that fills its world can still fill `/srv` for every game.
- **Stranded records:** package PRs that tighten or rename settings leave server records failing closed (`server-invalid`) until an admin saves them again.
- **Authoring friction:**
  - JSON manifests need escaped regexes.
  - Go and JavaScript regex semantics differ, so the Go tests are the gate.
- **Deletion:** no delete in the UI.
- **Unverified until the first deploy:** the items in Follow-up 3.

Revisit if:

- A game without env mapping needs a file in a format other than JSON (add a codec), or a credential must live in a file.
- The yolk fails us (refresh tokens rotate away, maintenance stops): build a thin image of our own.
- Hytale ships protocol tolerance: consider `updates: none` with pinned versions.
- More disk is wanted: grow the volume. A mod fills `/srv`: add an XFS project quota per world directory.
- The owner wants more than one server per game, or two running at once: runtime server ids, a cap, a runtime disk check and wildcard grants come back.
- 0006 picks a single-page app that wants JSON Forms: generate its UI schema from `group`.
- The owner wants per-world settings or list editors (whitelist, bans, permissions).
- A projection nears 4 KB.
- A game needs a console command before stopping (ADR-0002).

## Follow-ups (not part of this ADR)

1. **Infra PR:**
   - `catalog/`, the meta-schema and the synth assertions (including reserved ids and the disk sum);
   - the projections, and copying the manifests into the `api` asset;
   - the security group built from the union of ports;
   - the data-volume ID and the pinned `ssm-user` in user data;
   - the IAM changes, with tag values and record names generated from catalog ids;
   - the reserved-name context;
   - a `trivy image` job over every `runtime.image` in `catalog/`, on PRs and on the weekly scan (carries ADR-0002 follow-up 2 forward).
2. **Backend PR:**
   - the shared Go catalog package;
   - the admin routes and the lookup;
   - `prepare` and `write-file`;
   - golden tests, including hostile cases and hostile server records (duplicate keys, newlines, unknown secret names).
3. **First-deploy tests:**
   - **Writer safety:** each of these fails closed, with nothing written outside the server directory:
     - `Server/config.json` as a symlink to a host file;
     - the same file as a symlink into another package's directory;
     - a hard link;
     - a FIFO (must fail, not hang);
     - a file over 1 MiB, and one nested deeper than 64 levels;
     - duplicate keys;
     - malformed JSON;
     - an intermediate key that isn't an object;
     - a world directory owned by the wrong uid;
     - a `prepare` that hangs or crashes: the launcher enters admin mode with `prepare-failed`.
   - **Writer fidelity:** an unmanaged integer above 2^53 and the key order survive a write.
   - **Hytale on the yolk:**
     - the container uid (sets `dataOwner`);
     - the ready and auth lines match the RE2 patterns on ANSI-stripped journal output, and a chat message can't;
     - the server saves on SIGINT (ADR-0002's stop test);
     - two starts more than 1 h apart: the second must not ask for device auth (refresh-token rotation);
     - a first start with no `Server/` directory, so `prepare` creates `Server/config.json` with only the managed keys: the server boots, fills in defaults and keeps our values;
     - QUIC buffers: if the yolk warns, set `net.core.rmem_max` and `wmem_max` to at least 2097152 at boot;
     - disk: measure `/srv/games/<id>` after the first boot and after a session, plus the image, to set `diskEstimateGiB`;
     - the download host works with 443-only egress;
     - the digest matches a pelican-eggs/yolks publishing run (not just its `source` label);
     - a hand edit of `Update.Patchline` doesn't move the server off the release patchline;
     - `id ssm-user` shows the pinned uid, not 1001.
   - **SSM:**
     - values containing `{{x}}` and `a{{b`, to establish the restriction's scope;
     - `api` writes a SecureString with no KMS statement, and the instance reads one;
     - the `simulate-principal-policy` checks in "IAM";
     - a record write with `Tier: Standard` succeeds, and an oversized one returns 400.
   - **Env-file:** values with spaces, quotes and `=` arrive unchanged, and `grep` of `/srv/docker` confirms where credentials are kept.
   - **Volume check:** the hyphen-stripped pin matches the real serial, a new server's directory is created on it, and a wrong pin gives `volume-mismatch`.
4. **Runbooks:**
   - reset or retire a game's server (record, secrets and directory; before any PR that removes the package);
   - fix a world directory's owner (`chown -R --no-dereference` in admin mode) after a `dataOwner` correction;
   - restore a snapshot: also update the data-volume pin;
   - Hytale re-authentication;
   - rotate a credential;
   - prune unused images when `/srv` gets tight.

## Sources

Accessed 2026-10-13 unless noted.

1. Hytale Server Manual (read through the Help Center API; edited 2026-10-12) — https://support.hytale.com/hc/en-us/articles/45326769420827
2. Hytale Server Provider Authentication Guide (Help Center API; edited 2026-10-12) — https://support.hytale.com/hc/en-us/articles/45328341414043
3. `HytaleServerConfig` API reference — https://docs.hytale.com/api/com/hypixel/hytale/server/core/HytaleServerConfig
4. Pelican Hytale yolk — https://raw.githubusercontent.com/pelican-eggs/yolks/master/games/hytale/Dockerfile ; https://raw.githubusercontent.com/pelican-eggs/yolks/master/games/hytale/entrypoint.py ; https://github.com/pelican-eggs/yolks/commits/master/games/hytale
5. dustinrouillard/hytale-docker — https://github.com/dustinrouillard/hytale-docker
6. indifferentbroccoli/hytale-server-docker — https://github.com/indifferentbroccoli/hytale-server-docker
7. Hytale disk guidance (third-party) — https://www.oxygenserv.com/en/complete-guide-to-hosting-a-hytale-server ; https://low.ms/knowledgebase/how-to-create-a-hytale-server
8. Pelican eggs — https://pelican.dev/docs/eggs/creating-a-custom-egg ; https://raw.githubusercontent.com/pelican-eggs/games-standalone/main/hytale/egg-hytale.yaml ; https://raw.githubusercontent.com/pelican-eggs/minecraft/main/java/paper/egg-paper.yaml ; https://raw.githubusercontent.com/pelican-eggs/games-steamcmd/main/palworld/egg-palworld.yaml
9. CubeCoders AMPTemplates, Hytale (MIT): `hytale.kvp`, `hytaleconfig.json`, `hytalemetaconfig.json`, `hytaleserverconfig.json`, `hytaleupdates.json` — https://github.com/CubeCoders/AMPTemplates
10. Pelican Wings and Panel (2026-10-12) — https://pelican.dev/docs/wings/install ; https://pelican.dev/docs/panel/getting-started ; Wings ports (third-party mirror) — https://mintlify.wiki/pterodactyl/wings/introduction
11. itzg/minecraft-server, server properties — https://docker-minecraft-server.readthedocs.io/en/latest/configuration/server-properties/
12. Palworld configuration — https://docs.palworldgame.com/settings-and-operation/configuration/ ; file format (third-party) — https://www.gameserverkings.com/knowledge-base/palworld/world-settings/
13. Valheim launch arguments (third-party) — https://xgamingserver.com/docs/valheim/server-launch-parameters
14. felddy/foundryvtt-docker — https://raw.githubusercontent.com/felddy/foundryvtt-docker/develop/README.md
15. thijsvanloef/palworld-server-docker and lloesche/valheim-server-docker (2026-10-12) — https://github.com/thijsvanloef/palworld-server-docker ; https://github.com/lloesche/valheim-server-docker
16. santhosh-tekuri/jsonschema v6 — https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v6
17. Ajv (npm registry) — https://registry.npmjs.org/ajv/latest
18. google/jsonschema-go (runner-up Go validator; no draft or release stated) — https://github.com/google/jsonschema-go
19. JSON Forms and RJSF (npm registry) — https://registry.npmjs.org/@jsonforms/core/latest ; https://registry.npmjs.org/@rjsf/core/latest
20. Go `regexp/syntax` — https://pkg.go.dev/regexp/syntax
21. Parameter Store tiers — https://docs.aws.amazon.com/systems-manager/latest/userguide/parameter-store-advanced-parameters.html
22. Systems Manager pricing — https://aws.amazon.com/systems-manager/pricing/
23. `PutParameter` — https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_PutParameter.html
24. KMS for Parameter Store SecureStrings — https://docs.aws.amazon.com/kms/latest/developerguide/services-parameter-store.html
25. Mapping EBS volumes to NVMe devices — https://docs.aws.amazon.com/ebs/latest/userguide/identify-nvme-ebs-device.html
26. Hytale server operator policies (2026-10-12) — https://hytale.com/server-policies
27. Restricting decryption with `aws/ssm` (2026-10-16) — https://docs.aws.amazon.com/systems-manager/latest/userguide/ps-restrict-decryption.html ; https://docs.aws.amazon.com/systems-manager/latest/userguide/parameter-store-setting-up.html
28. Restricting access to parameters (2026-10-16) — https://docs.aws.amazon.com/systems-manager/latest/userguide/ps-restrict-parameter-access.html
29. Parameter retrieval authorization (2026-10-16) — https://docs.aws.amazon.com/systems-manager/latest/userguide/ps-retrieval-authorization.html
30. `GetParameterHistory` (2026-10-16) — https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_GetParameterHistory.html
31. santhosh-tekuri/jsonschema v6.0.3 error kinds (2026-10-16) — https://github.com/santhosh-tekuri/jsonschema/blob/v6.0.3/kind/kind.go
