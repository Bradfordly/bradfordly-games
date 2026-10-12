# bradfordly-games

On-demand game server hosting on AWS, infrastructure defined with AWS CDK.

## Work tracking

- All work starts as a GitHub issue in this repo, added to the [v1 project](https://github.com/users/Bradfordly/projects/8).
- The v1 project is the road to the 1.0 release. Milestones (`v0.0`, `v0.1`, …) track major/minor progress toward it.
- Board columns (the project's `Status` field): `Planning` → `In-progress` → `Review` → `Done`.
  - Move an issue to `In-progress` when work starts and to `Review` when its PR is opened. Closing or merging moves it to `Done`.
- Every tracked issue gets exactly one type label: `feature`, `bug`, `documentation`, or `config change`.
- One issue per branch/PR. Reference the issue in the PR body (`Closes #N`).

## Agents

Subagents live in `.claude/agents/`. The main session owns planning and integration; it delegates bounded jobs:

| Agent | Use for | Writes code? |
|---|---|---|
| `architect` | Design/architecture questions, stack choices, ADRs in `docs/adr/` | ADRs only |
| `docs-writer` | READMEs, ADR polish, runbooks, API/ops docs | Docs only |
| `code-reviewer` | Style, readability, correctness, resiliency review of a diff | No |
| `security-reviewer` | Run scanners, triage CVEs/CWEs, review for insecure patterns | No |

Before opening a PR, run `code-reviewer` and `security-reviewer` on the diff. CI runs the same agents on PRs.

Implementation agents (infra/CDK, backend, game-server) are intentionally not defined yet. Add them once v0.0 ADRs settle the boundaries.

## Engineering principles

- KISS and YAGNI first: build for the current milestone, not hypothetical ones. DRY only once duplication is real (rule of three).
- 12-factor: config via environment, stateless processes, backing services as attached resources, dev/prod parity.
- Security findings come from scanners (osv-scanner, Trivy, cdk-nag/Checkov once CDK exists), not from model memory.
- Pin third-party GitHub Actions to full commit SHAs.

## Git

- Branch names: `<type>/<short-desc>` where type is `feat`, `fix`, `docs`, or `chore`.
- Do not add `Co-Authored-By` trailers or otherwise attribute Claude in commits or PRs.
