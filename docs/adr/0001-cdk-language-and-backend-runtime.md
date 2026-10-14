# 0001. CDK language and backend runtime

- Status: Accepted
- Date: 2026-10-11
- Amended by: [0003](0003-control-plane-server-lifecycle-and-idle-shutdown.md#amends-adr-0001) (B1: the on-host agent ships as a SHA-256-pinned static Go binary from a GitHub Release, not a CDK container image)

## Context

Issue #2. We need a language for the AWS CDK app and a language for application (control-plane) code. The owner is a solo maintainer, comfortable in JS/TS and Go, and asked for "JavaScript" for CDK ("for best CDK support") and Go for application code.

Out of scope: the hosting model for game servers (ECS/Fargate vs EC2 vs GameLift) is ADR-0002 (#3). Note that the game server *process* is usually a third-party binary (e.g. a dedicated server image); this ADR only covers code we write: infrastructure and the control plane (start/stop/status APIs, idle shutdown, etc.).

Verified facts (as of 2026-10-11):

- CDK is developed in TypeScript; other languages (including JavaScript) are bindings/consumers, and "much of the AWS CDK example code is written in TypeScript" [1]. JavaScript is fully supported and stable [2].
- Every CDK language needs Node.js: the CDK CLI and jsii runtime run on it. CDK supports Node 24.x until 2028-10-30, 22.x until 2027-10-30, 20.x until 2026-10-30 [3].
- Go on Lambda runs on the OS-only runtime `provided.al2023` (supported until 2029-06-30); the `go1.x` runtime is deprecated. Zip and container image deployment are both supported [4].
- `@aws-cdk/aws-lambda-go-alpha` (`GoFunction`) is still experimental: not semver, breaking changes possible, and its default runtime is `PROVIDED_AL2`, not AL2023 [5]. AWS advises pinning alpha modules to exact versions [2].
- Current Go is 1.27 (released 2026-08-19); each major is supported until two newer majors ship, so 1.26 and 1.27 are supported [6].
- cdk-nag publishes to npm for TypeScript/JavaScript (and other languages) [7].
- Latest `aws-cdk-lib` is 2.273.0 (npm registry, 2026-10-08); `@aws-cdk/aws-lambda-go-alpha` is still published as `2.273.0-alpha.0`. CDK v2 is the only supported major (v1 ended 2023-06-01) [1].

## Options considered

### A. Simplest viable option: TypeScript everywhere (CDK + Node.js Lambdas)

One language, one toolchain (Node), one Dependabot ecosystem (npm), `NodejsFunction` is stable in `aws-cdk-lib`. Cheapest to run as a solo maintainer.
Rejected (narrowly): the owner prefers Go for application code, Go's static binaries suit both Lambda (`provided.al2023`, arm64) and minimal container images, and the marginal cost of Go is low because npm is unavoidable anyway (see C). If no Go code is written by the time the first backend feature lands, revisit and pick A.

### B. Go everywhere (CDK in Go + Go application code)

Looks like "one language" but is not: CDK in Go still requires Node.js and the npm-distributed CLI [1][3]. CDK Go code is verbose (`jsii.String(...)` pointers everywhere), examples and third-party constructs are TypeScript-first, and nested jsii binaries are slower. Rejected: pays the Node cost without the CDK ergonomics.

### C. TypeScript for CDK + Go for application code (recommended)

TypeScript gets type checking of construct props, the best docs/examples, and first-party construct libraries and cdk-nag on npm. Go is used only for code we deploy. Cost of two languages: two toolchains (Node 24, Go 1.27), two Dependabot ecosystems (npm, gomod), two lint/test steps in CI. Because Node is required for any CDK language, the true incremental cost of Go is one toolchain and one ecosystem.

### D. Plain JavaScript for CDK + Go (owner's literal request)

Same runtime as C, but without types. AWS's own JavaScript guide warns that a misspelled optional prop is "silently ignored" [2], which for IaC means silently insecure or costly defaults (e.g. missing encryption or retention settings). TS examples must be hand-translated. The only saving is a `tsc` step that takes seconds [2]. Rejected.

### Go build/deploy sub-options (for C)

1. **Stable `lambda.Function` + `Runtime.PROVIDED_AL2023` + `Code.fromAsset` of a binary built by `go build` (GOOS=linux, GOARCH=arm64, `bootstrap`)** — recommended. No alpha dependency; one build command.
2. `GoFunction` (alpha) — less boilerplate, but experimental API, exact pinning required, and must override the AL2 default runtime [5]. Adopt only if hand-rolled bundling grows past a few lines.
3. Container image (`DockerImageAsset` / `DockerImageCode`) — use only where the hosting model needs containers (likely if ADR-0002 picks ECS/Fargate for any long-running control-plane process).

## Decision

- **CDK language: TypeScript**, on **Node.js 24 LTS**. This deliberately differs from the owner's stated "JavaScript": TypeScript *is* the JavaScript-family language with the best CDK support; plain JS gives up prop type checking for no meaningful gain.
- **Application/control-plane code: Go 1.27** (track the latest supported major), deployed to Lambda on `provided.al2023`, arm64, using the stable `aws-lambda` module with a prebuilt `bootstrap` binary (sub-option 1). No `GoFunction` alpha for now.
- **Interaction with ADR-0002:** this ADR does not constrain the hosting model. Go fits all three candidates (Lambda for the API, static binaries for containers on ECS/EC2). If ADR-0002 needs a long-running agent or sidecar, it ships as a Go container image built by CDK assets; the game server image itself is language-agnostic.

Minimal repo layout (create directories only when the first file needs them):

```
infra/            # cdk init app --language typescript (bin/, lib/, cdk.json, package.json, tsconfig.json)
backend/          # created with the first Go function: go.mod, cmd/<function>/main.go
```

No shared libraries, workspaces, monorepo tooling (Nx/Turbo/projen), or `internal/` packages until a third occurrence demands them.

## Consequences

Positive:
- Compile-time checking of construct props; TS examples and constructs usable verbatim.
- Go binaries are small, fast to cold start, and arm64-friendly (cost).
- No dependence on experimental CDK modules.

Negative:
- Two toolchains and two dependency ecosystems to keep current; CI must install both.
- Hand-rolled Go build step (`go build` before `cdk synth`) that `GoFunction` would otherwise hide.
- No code sharing between infra and backend (e.g. env var names must be kept in sync by convention; 12-factor config via environment keeps this surface small).

Revisit if:
- No Go code exists when the first backend feature ships (switch to option A).
- `aws-lambda-go-alpha` graduates to stable in `aws-cdk-lib` (adopt it).
- ADR-0002 selects a model whose SDK or tooling strongly favours another language.
- Node 24 nears CDK end of support (2028-10-30) or Go 1.27 falls out of support (when Go 1.29 ships).

### Follow-ups (not part of this ADR)

1. `.github/dependabot.yml`: add `npm` (directory `/infra`) now, and `gomod` (directory `/backend`) when `backend/` exists. Pin `@aws-cdk/*-alpha` packages exactly if any are ever added.
2. Add cdk-nag (`AwsSolutionsChecks`) to the CDK app so violations fail `cdk synth`.
3. `.github/workflows/security-scan.yml`: add a job that runs `cdk synth` so `cdk.out` exists, then scan it. Trivy's `misconfig` scanner already covers CloudFormation; evaluate whether Checkov on `cdk.out` adds signal before adding a second IaC scanner.
4. CI: pin Node 24 and Go 1.27 via `setup-node`/`setup-go` (SHA-pinned) and `.nvmrc` / `go.mod` `go` directive.

## Sources

1. Supported programming languages for the AWS CDK — https://docs.aws.amazon.com/cdk/v2/guide/languages.html
2. Working with the AWS CDK in JavaScript — https://docs.aws.amazon.com/cdk/v2/guide/work-with-cdk-javascript.html
3. Supported Node.js versions for the AWS CDK — https://docs.aws.amazon.com/cdk/v2/guide/node-versions.html
4. Building Lambda functions with Go — https://docs.aws.amazon.com/lambda/latest/dg/lambda-golang.html
5. aws-lambda-go-alpha README — https://docs.aws.amazon.com/cdk/api/v2/docs/aws-lambda-go-alpha-readme.html
6. Go release history — https://go.dev/doc/devel/release
7. cdk-nag — https://github.com/cdklabs/cdk-nag
