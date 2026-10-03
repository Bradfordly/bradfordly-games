# Agent rules

This repository tracks work on a GitHub Project board named `bradfordly-games`. The board is a kanban with three columns: **Backlog**, **In-Progress**, and **Review**.

## Pull requests

- Open pull requests against `main`. GitHub attributes them to @Bradfordly. That account cannot approve its own pull request, and the Cursor GitHub App cannot be granted permission to open pull requests in its place.
- The pull request body must include a closing keyword for the issue it finishes, for example `Closes #12`.
- Do not approve a pull request. Do not submit an approving review, do not dismiss a request for changes in order to clear the way, and do not merge.
- Do not enable auto-merge.
- @Bradfordly's merge is the approval. Branch protection requires a pull request and does not require a separate approving review. Do not turn required approving reviews or required code-owner reviews back on. Bot approvals are dismissed.

## Issues

- Open an issue before starting work. Do not add a category label, and do not assign anyone. New issues stay in **Backlog**.
- @Bradfordly applies exactly one category label: `documentation`, `config change`, `bug fix`, or `feature`. That label is the gate. It chooses which kind of agent may take the issue.
- Do not assign an agent, and do not start implementation, until that label is present. After @Bradfordly adds the label, the matching agent may be assigned. Assignment moves the issue to **In-Progress**. An assignee without a category label leaves the issue in **Backlog**.
- When the pull request to `main` is open, the issue moves to **Review**.
- After @Bradfordly merges the pull request, the linked issue is closed. Do not close an issue while its pull request is still open.
- Do not open implementation issues until the design has been agreed and split into tasks. A design note still waits for @Bradfordly to apply the `documentation` label.

## Commits

Each commit contains one specific change. The message follows Conventional Commits:

```
<type>[optional scope]: <description>
```

Use `feat` for a feature, `fix` for a bug fix, `docs` for documentation, `chore` for a config change or maintenance, `test` for a test-only change, and `refactor` for a behavior-preserving restructure. A breaking change adds `!` after the type or scope and includes a `BREAKING CHANGE:` footer. The description is imperative and names that change. The body explains why when the subject is not enough.

## Board automation

`scripts/github_project.py` creates the project, labels, and branch protection. `.github/workflows/project-board.yml` moves cards after `PROJECT_TOKEN` is stored as a repository secret. That secret is a classic personal access token with the `project` and `public_repo` scopes. A fine-grained token cannot access a project owned by a user account, and GitHub does not show a Projects account permission for one. Until that secret exists, still follow the labels, assignment, and `Closes #` rules above.
