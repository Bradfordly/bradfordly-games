# bradfordly-games

On-Demand Game Server Hosting

## How work is tracked

Tasks are GitHub issues on the `bradfordly-games` project board. The columns are Backlog, In-Progress, and Review. Agents open issues without a category label. @Bradfordly applies one label, `documentation`, `config change`, `bug fix`, or `feature`, and only then can a matching agent be assigned.

Pull requests target `main`. @Bradfordly's merge is the approval. Agents do not approve or merge. The operating rules are in `AGENTS.md`. The board, labels, and branch protection are created by `scripts/github_project.py`.
