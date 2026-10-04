# Agent rules

This repository tracks work on a GitHub Project board named `bradfordly-games`. The board is a kanban with three columns: **Backlog**, **In-Progress**, and **Review**.

## Pull requests

- Open pull requests against `main`. GitHub attributes them to @Bradfordly. That account cannot approve its own pull request, and the Cursor GitHub App cannot be granted permission to open pull requests in its place.
- The pull request body must include a closing keyword for the issue it finishes, for example `Closes #12`.
- Do not approve a pull request. Do not submit an approving review, do not dismiss a request for changes in order to clear the way, and do not merge.
- Do not enable auto-merge.
- @Bradfordly's merge is the approval. Branch protection requires a pull request and does not require a separate approving review. Do not turn required approving reviews or required code-owner reviews back on. Bot approvals are dismissed.

## Commits

Each commit contains one specific change. The message follows Conventional Commits:

```
<type>[optional scope]: <description>
```

Use `feat` for a feature, `fix` for a bug fix, `docs` for documentation, `chore` for a config change or maintenance, `test` for a test-only change, and `refactor` for a behavior-preserving restructure. A breaking change adds `!` after the type or scope and includes a `BREAKING CHANGE:` footer. The description is imperative and names that change. The body explains why when the subject is not enough.

## Design

Follow YAGNI, DRY, and KISS.

- YAGNI: build only what the current task requires.
- DRY: keep one representation of each rule or piece of logic.
- KISS: use the simplest implementation that meets the requirement.

## Issues

- Open an issue before starting work. Do not add a category label, and do not assign anyone. New issues stay in **Backlog**.
- @Bradfordly applies exactly one category label: `documentation`, `config change`, `bug fix`, or `feature`. That label is the gate. It chooses which kind of agent may take the issue.
- Do not assign an agent, and do not start implementation, until that label is present. After @Bradfordly adds the label, the matching agent may be assigned. Assignment moves the issue to **In-Progress**. An assignee without a category label leaves the issue in **Backlog**.
- When the pull request to `main` is open, the issue moves to **Review**.
- After @Bradfordly merges the pull request, the linked issue is closed. Do not close an issue while its pull request is still open.
- Do not open implementation issues until the design has been agreed and split into tasks. A design note still waits for @Bradfordly to apply the `documentation` label.

## Builder agent

The builder agent implements issues labeled `feature` or `bug fix`. It does not take `documentation` or `config change`.

- Create unit tests targeting at least 80% coverage for all production code.
- Build a local testing environment so the stack can be tested with BDD tests before shipping.
- Do not open the pull request until unit coverage holds and the relevant BDD scenarios pass locally.

The builder orchestrates. It deploys one cloud sub-agent per labeled, unassigned `feature` or `bug fix` issue. After the category label exists, it assigns @Bradfordly and starts that sub-agent. It does not assign or start unlabeled issues. It skips issues that already have an assignee or an open pull request. The parent does not implement the issue in its checkout.

When a sub-agent has a question, the builder forwards it to @Bradfordly with a suggested answer and a brief explanation of why it reached that conclusion. It does not answer the sub-agent until @Bradfordly reviews the suggestion.

## Versions

Versions follow semantic versioning, `MAJOR.MINOR.PATCH`. GitHub milestones are named `vMAJOR.MINOR`. The current milestone is `v0.0`.

- Classify every change as a patch, a minor update, or a major update.
- A patch stays on the current milestone. Do not create a milestone for a patch.
- Create a new milestone for a minor update, such as `v0.1`, and for a major update, such as `v1.0`. While the major version is 0, a minor milestone may include breaking changes.
- Name that milestone in the issue and in the pull request.
- The Cursor GitHub App cannot create milestones or assign them on issues. Those calls return HTTP 403. @Bradfordly creates each new major or minor milestone and assigns the issue and the pull request.

## Board automation

`scripts/github_project.py` creates the project, labels, and branch protection. `.github/workflows/project-board.yml` moves cards after `PROJECT_TOKEN` is stored as a repository secret. That secret is a classic personal access token with the `project` and `public_repo` scopes. A fine-grained token cannot access a project owned by a user account, and GitHub does not show a Projects account permission for one. Until that secret exists, still follow the labels, assignment, and `Closes #` rules above.
