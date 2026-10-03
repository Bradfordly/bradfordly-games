# Agent rules

This repository tracks work on a GitHub Project board named `bradfordly-games`. The board is a kanban with three columns: **Backlog**, **In-Progress**, and **Review**.

## Pull requests

- Open pull requests against `main`.
- The pull request body must include a closing keyword for the issue it finishes, for example `Closes #12`.
- Do not approve a pull request. Do not submit an approving review, do not dismiss a request for changes in order to clear the way, and do not merge.
- Do not enable auto-merge.
- Approval and merge belong only to @Bradfordly. Branch protection requires a code-owner review from that account. Bot approvals are dismissed.

## Issues

- Open an issue before starting work. Use exactly one category label: `documentation`, `config change`, `bug fix`, or `feature`.
- New issues stay in **Backlog** while they have no assignee.
- Assign yourself when you start the work. That moves the issue to **In-Progress**. Do not start implementation while the issue is unassigned.
- When the pull request to `main` is open, the issue moves to **Review**.
- After @Bradfordly merges the pull request, the linked issue is closed. Do not close an issue while its pull request is still open.
- Do not open implementation issues until the design has been agreed and split into tasks. A `documentation` issue is appropriate for a design note that needs review.

## Board automation

`scripts/github_project.py` creates the project, labels, and branch protection. `.github/workflows/project-board.yml` moves cards after `PROJECT_TOKEN` is stored as a repository secret. That secret is a classic personal access token with the `project` and `public_repo` scopes. A fine-grained token cannot access a project owned by a user account, and GitHub does not show a Projects account permission for one. Until that secret exists, still follow the labels, assignment, and `Closes #` rules above.
