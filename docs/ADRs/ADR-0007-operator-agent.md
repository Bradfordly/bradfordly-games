# ADR-0007: Operator agent for documentation and config

## Status

Accepted

## Date

2026-10-04

## Context

[AGENTS.md](../../AGENTS.md) gates work with one category label: `documentation`, `config change`, `bug fix`, or `feature`. Only the builder agent is named there. It takes `feature` and `bug fix`, orchestrates cloud sub-agents, and refuses documentation and config.

[`.cursor/rules/planning-agent.mdc`](../../.cursor/rules/planning-agent.mdc) owns adversarial design on specs and ADRs. It does not implement labeled issues.

Labeled `documentation` and `config change` issues therefore have no matching agent. Config that does not cite a spec or ADR can drift from the design.

Options considered:

- **Leave the gap:** builder keeps refusing docs/config. Those issues stall.
- **Widen the builder:** one always-on agent implements every label. Feature chats load docs hygiene they do not need, and the builder’s dispatch rules collide with in-checkout docs work.
- **Add an operator agent** (chosen): a dedicated chat implements assigned `documentation` and `config change` issues in its checkout. Planning still designs. Builder still builds.
- **Split the operator now** into a docs steward and a repo-config steward, plus an always-apply router: more files before the first operator issue ships.

## Decision

1. **The repository has three agents.** Planning designs. The operator implements `documentation` and `config change`. The builder implements `feature` and `bug fix`.
2. **The operator works in its own checkout.** It does not dispatch cloud sub-agents. After the category label exists, it assigns @Bradfordly and implements.
3. **Keep `docs/specs/` and `docs/ADRs/`.** Do not migrate to Diátaxis folders. The index is [docs/README.md](../README.md). New ADRs and specs follow [docs/templates/adr.md](../templates/adr.md) and [docs/templates/spec.md](../templates/spec.md).
4. **Every config change cites the spec heading and/or ADR that authorizes it.** If none exists, stop and ask for a planning design note.
5. **Do not split the operator yet.** Split when [`.cursor/rules/operator-agent.mdc`](../../.cursor/rules/operator-agent.mdc) exceeds about 50 lines or docs hygiene and repo-config start changing on different cadences. Then add `docs-steward.mdc` and `repo-config.mdc`. Add `agent-router.mdc` only if the three agents keep colliding in one chat.

This ADR authorizes [`.cursor/rules/operator-agent.mdc`](../../.cursor/rules/operator-agent.mdc) and the operator section in [AGENTS.md](../../AGENTS.md).

## Cost comparison

Agent rules have **no AWS line**. The bill is context tokens: what loads in every chat versus what loads only when matching files are open. Budget model, not a quote. Snapshot October 2026.

### Unit rates used

| Resource | Rate | Always-on monthly |
| --- | --- | --- |
| EC2 / EKS / SaaS agent host | — | $0 |
| Always-apply Cursor rule | Full rule text in every chat | Paid on feature work that does not need it |
| Glob-scoped Cursor rule | Full rule text when globs match | $0 when those files are closed |

### Alternatives

| Option | Loaded every chat | Loaded on specs, ADRs, or repo config | What you pay extra for |
| --- | --- | --- | --- |
| A · one always-on builder | Builder + docs + config | Same | Docs hygiene on every feature chat; dispatch rules on docs issues |
| **B · three scoped agents (chosen)** | Builder | Planning on specs/ADRs; operator on docs and config globs | A second dedicated chat for docs/config |
| C · split operator now + router | Builder + router | Two operator rules + planning | Files we do not need yet |

Play-hour scenario: one documentation or config session. B loads operator (and planning on specs/ADRs) for that session only. A loads the same extra text on the next feature session too. C loads more files for the same session.

Break-even: B wins as soon as feature work is more common than docs/config work, which is the current board mix. C is cheaper only after the operator rule is too large to keep as one concern.

### What extra money would buy

- **Always-apply operator:** docs hygiene in every chat. Rejected; that is option A’s cost without option A’s simplicity.
- **Split now (C):** separate docs and repo-config rules. Rejected until the split trigger in the Decision fires.

## Consequences

- [AGENTS.md](../../AGENTS.md) and [README.md](../../README.md) name the operator next to the builder.
- The builder rule still refuses to implement or dispatch documentation and config. Those issues belong to the operator.
- Planning still writes design notes and splits unlabeled issues. The operator executes them after @Bradfordly applies `documentation` or `config change`.
- Existing ADRs 0001–0005 are not retrofitted with Cost comparison in this change.
- Product "operator" in the panel specs still means the human who uses `games.bradfordly.com`. This ADR is the repository agent.
