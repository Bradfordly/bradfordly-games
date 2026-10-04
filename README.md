# Bradfordly Games

On-Demand Game Server Hosting

## How work is tracked

Tasks are GitHub issues on the `bradfordly-games` project board. The columns are Backlog, In-Progress, and Review. Agents open issues without a category label. @Bradfordly applies one label, `documentation`, `config change`, `bug fix`, or `feature`, and only then can a matching agent be assigned. The builder agent takes `feature` and `bug fix`. It deploys a cloud sub-agent to each labeled, unassigned `feature` or `bug fix` issue.

Pull requests target `main`. @Bradfordly's merge is the approval. Agents do not approve or merge. Versions follow semantic versioning. Milestones are `vMAJOR.MINOR`, and the current milestone is `v0.0`. A new milestone is created for a major or minor update. A patch stays on the current milestone. @Bradfordly assigns the milestone. The operating rules are in `AGENTS.md`. The board, labels, and branch protection are created by `scripts/github_project.py`.
