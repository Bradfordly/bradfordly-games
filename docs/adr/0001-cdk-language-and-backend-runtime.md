# 0001. CDK language and backend runtime

- Status: Proposed
- Date: 2026-10-12

## Context

bradfordly-games is on-demand game server hosting on AWS, with infrastructure defined in AWS CDK. v0.0 must settle the CDK language and whether the backend (control-plane code such as start/stop/status handlers, idle detection and scale-to-zero logic) shares it. Nothing exists yet besides CI scaffolding:

- `.github/dependabot.yml` only covered `github-actions` before this ADR, with a note to add the CDK ecosystem (npm or pip) once chosen. The `npm` entry is added alongside this ADR.
- `.github/workflows/security-scan.yml` runs osv-scanner and Trivy. CLAUDE.md names cdk-nag/Checkov as the IaC scanners to add once CDK exists.
- CLAUDE.md priorities: KISS/YAGNI first, 12-factor, security and cost as requirements, and security findings must come from scanners.
- The game servers themselves are expected to be off-the-shelf containers (for example Minecraft or Valheim images). Their language is irrelevant to this decision. Only our own code (CDK app and backend handlers) is affected.

Constraints: the team is small, and every extra language adds a toolchain, a lockfile, a Dependabot ecosystem and scanner configuration, which is a recurring cost.

Not verified: current Lambda runtime versions, CDK release details and cdk-nag feature parity were not checked against live documentation. Re-check the AWS docs (Lambda runtimes, aws-cdk-lib and cdk-nag READMEs) before implementing.

## Options considered

1. **Simplest viable option: TypeScript for CDK and TypeScript for the backend (one language, one toolchain).** One `package.json` and lockfile, one Dependabot ecosystem (`npm`), and one test runner. Shared types between infra and handlers (for example resource names, env var contract, event shapes).
2. **Python for CDK and Python for the backend.** Also a single language. Good if the team is Python-first. CDK Python runs through the jsii bridge (a Node.js process is still required), so it does not remove Node from the toolchain.
3. **TypeScript CDK with a Python (or other) backend.** Two toolchains, two lockfiles and two Dependabot ecosystems, with no concrete requirement that justifies it today.
4. **Python CDK with a TypeScript backend.** Same two-toolchain cost as option 3, with the added jsii indirection for infrastructure.
5. **Backend in Go or Rust.** Faster cold starts and smaller artifacts, but this is speculative optimization (YAGNI). Handlers are short-lived API glue around AWS SDK calls, where cold start is not a stated requirement.

Why the others were rejected:

- Option 2 is close and viable. CDK's primary language is TypeScript, so its docs, examples, construct libraries (including cdk-nag, which is TypeScript-first) and issue answers are TypeScript first. Python adds the jsii layer, with occasionally less precise errors and typing. Nothing in the repo yet favors Python, so the tie goes to the native language.
- Options 3 and 4 double the maintenance surface without a requirement.
- Option 5 is premature. Revisit if measured cold-start or cost data justifies it.

## Decision

Use **TypeScript for the CDK app** and **TypeScript (Node.js) for the backend runtime**, in a single npm-managed repo. Handlers run as AWS Lambda functions bundled with CDK's `NodejsFunction` (esbuild), configured via environment variables (12-factor) and holding no local state. Game server workloads stay language-agnostic containers and are out of scope here.

Note on flexibility: the backend language is not locked to the CDK language. If a future component has a real need (for example a game-specific sidecar), it may use another language, recorded in its own ADR.

## Consequences

Positive:

- One language, one lockfile, one test runner, and shared types between infra and handlers, which reduces DRY violations of knowledge such as env var names and event schemas.
- CDK is native TypeScript, with the best docs and example coverage and no jsii indirection for our own code.
- Smallest scanner and Dependabot surface (one ecosystem).

Negative:

- If contributors are Python-first, there is a learning cost.
- npm supply-chain risk is real. Mitigate with a committed lockfile, `npm ci` in CI, osv-scanner on the lockfile, and Dependabot grouping.
- Node.js Lambda runtimes are deprecated on a schedule, so the runtime version needs periodic bumps (Dependabot will not cover the Lambda runtime setting).

Revisit if:

- The team composition shifts to Python-first, or a backend component needs a Python-only library.
- Measured Lambda cold-start or cost makes a different runtime worthwhile.

### Concrete follow-ups (separate issues and PRs)

1. `.github/dependabot.yml`: the `npm` entry at `/` was added with this ADR. Dependabot reports an error until `package.json` exists. If the CDK app lives in a subdirectory, update `directory` accordingly.
2. `.github/workflows/security-scan.yml`:
   - **cdk-nag**: add the `AwsSolutionsChecks` Aspect in the CDK app (code, not workflow) so `cdk synth` fails on unsuppressed findings. Ensure CI runs `cdk synth`. Every suppression requires a written reason.
   - **Checkov**: add a job that runs `cdk synth` and scans `cdk.out/*.template.json` (`--framework cloudformation`), with the action pinned to a full commit SHA per CLAUDE.md. Trivy already scans misconfigurations, so confirm Checkov adds value beyond Trivy and cdk-nag before keeping all three (avoid redundant scanners).
   - osv-scanner already picks up `package-lock.json` through `--recursive`, so no change is needed there.
3. Scaffold the CDK app (`cdk init app --language typescript`), commit `package-lock.json`, and add `tsconfig`, lint and test scripts.
4. Add an `infra/CDK` implementation agent only after the layout is settled, per CLAUDE.md.
