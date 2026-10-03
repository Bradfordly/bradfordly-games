#!/usr/bin/env python3
"""Create the bradfordly-games board and keep issue status in sync with it.

bootstrap creates labels, the kanban project, repository variables, and branch
protection. sync-event is the GitHub Actions entry point that moves cards.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[1]
LABELS_PATH = REPO_ROOT / ".github" / "labels.json"

STATUS_BACKLOG = "Backlog"
STATUS_IN_PROGRESS = "In-Progress"
STATUS_REVIEW = "Review"
STATUS_NAMES = (STATUS_BACKLOG, STATUS_IN_PROGRESS, STATUS_REVIEW)

PROJECT_TITLE = "bradfordly-games"
PROJECT_SUMMARY = "Kanban for bradfordly-games: Backlog, In-Progress, Review."

CLOSING_ISSUE = re.compile(
    r"(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)\b"
)

STATUS_SPECS = (
    {
        "name": STATUS_BACKLOG,
        "color": "GRAY",
        "description": "No assignee yet. Work has not started.",
        "aliases": (STATUS_BACKLOG, "Todo"),
    },
    {
        "name": STATUS_IN_PROGRESS,
        "color": "YELLOW",
        "description": "An agent is assigned and the work is underway.",
        "aliases": (STATUS_IN_PROGRESS, "In Progress"),
    },
    {
        "name": STATUS_REVIEW,
        "color": "ORANGE",
        "description": "A pull request to main is open and waiting for approval.",
        "aliases": (STATUS_REVIEW,),
    },
)


def desired_status(*, assignee_count: int, has_open_pr: bool) -> str:
    if has_open_pr:
        return STATUS_REVIEW
    if assignee_count > 0:
        return STATUS_IN_PROGRESS
    return STATUS_BACKLOG


def linked_issue_numbers(body: str | None) -> list[int]:
    found: list[int] = []
    for match in CLOSING_ISSUE.findall(body or ""):
        number = int(match)
        if number not in found:
            found.append(number)
    return found


def load_labels() -> list[dict[str, str]]:
    labels = json.loads(LABELS_PATH.read_text())
    names = [label["name"] for label in labels]
    if names != list(dict.fromkeys(names)):
        raise ValueError("category labels must be unique")
    if set(names) != {
        "documentation",
        "config change",
        "bug fix",
        "feature",
    }:
        raise ValueError("category labels do not match the required set")
    return labels


def label_plan(existing_names: list[str]) -> dict[str, list[str]]:
    wanted = [label["name"] for label in load_labels()]
    return {
        "create": [name for name in wanted if name not in existing_names],
        "update": [name for name in wanted if name in existing_names],
        "delete": [name for name in existing_names if name not in wanted],
    }


def status_option_inputs(existing: list[dict[str, str]]) -> list[dict[str, str]]:
    """Replace Status options with the three kanban columns.

    Reuse an existing option id when renaming Todo or In Progress so items
    already on that option keep their value. Done is not reused.
    """
    options: list[dict[str, str]] = []
    for spec in STATUS_SPECS:
        entry = {
            "name": spec["name"],
            "color": spec["color"],
            "description": spec["description"],
        }
        for current in existing:
            if current.get("name") in spec["aliases"] and current.get("id"):
                entry["id"] = current["id"]
                break
        options.append(entry)
    return options


def workflows_to_disable(workflows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Default workflows set a Done column this board does not have."""
    selected = []
    for workflow in workflows:
        name = str(workflow.get("name", "")).casefold()
        if "reopen" in name:
            continue
        if "item closed" in name or "pull request merged" in name or name == "merged":
            selected.append(workflow)
    return selected


def protection_body() -> dict[str, Any]:
    return {
        "required_status_checks": None,
        "enforce_admins": True,
        "required_pull_request_reviews": {
            "dismiss_stale_reviews": True,
            "require_code_owner_reviews": True,
            "required_approving_review_count": 1,
        },
        "restrictions": None,
        "allow_force_pushes": False,
        "allow_deletions": False,
        "required_conversation_resolution": False,
    }


def issue_templates_use_known_labels() -> list[str]:
    """Return template labels that are not in the category set."""
    allowed = {label["name"] for label in load_labels()}
    unknown: list[str] = []
    template_dir = REPO_ROOT / ".github" / "ISSUE_TEMPLATE"
    for path in sorted(template_dir.glob("*.md")):
        text = path.read_text()
        match = re.search(r"^labels:\s*\[(.*)\]\s*$", text, re.MULTILINE)
        if not match:
            unknown.append(f"{path.name}: missing labels")
            continue
        for raw in match.group(1).split(","):
            name = raw.strip().strip("\"'")
            if name not in allowed:
                unknown.append(f"{path.name}: {name}")
    return unknown


class GithubError(RuntimeError):
    pass


class GithubSession:
    def __init__(self, token: str) -> None:
        self.token = token

    def request(
        self,
        method: str,
        url: str,
        body: dict[str, Any] | None = None,
        accept: str = "application/vnd.github+json",
    ) -> Any:
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(
            url,
            data=data,
            method=method,
            headers={
                "Authorization": f"Bearer {self.token}",
                "Accept": accept,
                "Content-Type": "application/json",
                "User-Agent": "bradfordly-games-project-setup",
                "X-GitHub-Api-Version": "2022-11-28",
            },
        )
        try:
            with urllib.request.urlopen(req) as response:
                raw = response.read().decode()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as error:
            detail = error.read().decode(errors="replace")
            raise GithubError(f"{method} {url} failed ({error.code}): {detail}") from error

    def graphql(self, query: str, variables: dict[str, Any]) -> dict[str, Any]:
        payload = self.request(
            "POST",
            "https://api.github.com/graphql",
            {"query": query, "variables": variables},
        )
        if payload.get("errors"):
            raise GithubError(json.dumps(payload["errors"]))
        return payload["data"]

    def rest(self, method: str, path: str, body: dict[str, Any] | None = None) -> Any:
        return self.request(method, f"https://api.github.com{path}", body)


def project_token_instructions(repo: str) -> str:
    """How to create the token that moves cards on a user-owned project.

    Fine-grained personal access tokens have no Projects account permission.
    That permission exists only for an organization, and a fine-grained token
    cannot access a project owned by a user account.
    """
    return "\n".join(
        [
            "Store a classic personal access token as the PROJECT_TOKEN secret.",
            "A fine-grained token cannot access this project: GitHub does not offer a Projects account permission for a user, and that permission exists only for an organization.",
            "Open Settings, Developer settings, Personal access tokens, Tokens (classic), then Generate new token (classic).",
            "Select the project scope, shown as Full control of projects.",
            "Select the public_repo scope, shown as Access public repositories. The repository is public, so the broader repo scope is unnecessary.",
            f"Then run: gh secret set PROJECT_TOKEN --repo {repo}",
        ]
    )


def missing_token_message() -> str:
    return (
        "No GitHub token is available. Log in with `gh auth login` as Bradfordly, "
        "or set GH_TOKEN to a classic personal access token with the project and "
        "public_repo scopes. A fine-grained token has no Projects account permission "
        "and cannot update a project owned by a user."
    )


def resolve_token(env: dict[str, str], read_gh_token) -> str:
    token = env.get("GH_TOKEN") or env.get("GITHUB_TOKEN")
    if token:
        return token
    try:
        output = read_gh_token()
    except (OSError, subprocess.CalledProcessError):
        output = ""
    if output and output.strip():
        return output.strip()
    raise GithubError(missing_token_message())


def read_gh_auth_token() -> str:
    completed = subprocess.run(
        ["gh", "auth", "token"],
        check=True,
        capture_output=True,
        text=True,
    )
    return completed.stdout


def token_from_environment() -> str:
    return resolve_token(os.environ, read_gh_auth_token)


def sync_labels(github: GithubSession, repo: str) -> None:
    existing = github.rest("GET", f"/repos/{repo}/labels?per_page=100")
    existing_names = [label["name"] for label in existing]
    plan = label_plan(existing_names)
    by_name = {label["name"]: label for label in load_labels()}
    for name in plan["create"]:
        github.rest("POST", f"/repos/{repo}/labels", by_name[name])
        print(f"created label {name}")
    for name in plan["update"]:
        github.rest("PATCH", f"/repos/{repo}/labels/{urllib.parse.quote(name)}", by_name[name])
        print(f"updated label {name}")
    for name in plan["delete"]:
        github.rest("DELETE", f"/repos/{repo}/labels/{urllib.parse.quote(name)}")
        print(f"deleted label {name}")


def find_or_create_project(github: GithubSession, owner: str, title: str) -> dict[str, Any]:
    data = github.graphql(
        """
        query($login: String!) {
          user(login: $login) {
            id
            projectsV2(first: 50) {
              nodes { id number title url }
            }
          }
        }
        """,
        {"login": owner},
    )
    user = data["user"]
    if user is None:
        raise GithubError(f"GitHub user {owner} was not found")
    for project in user["projectsV2"]["nodes"]:
        if project["title"] == title:
            print(f"using existing project {project['url']}")
            return project
    created = github.graphql(
        """
        mutation($ownerId: ID!, $title: String!) {
          createProjectV2(input: {ownerId: $ownerId, title: $title}) {
            projectV2 { id number title url }
          }
        }
        """,
        {"ownerId": user["id"], "title": title},
    )
    project = created["createProjectV2"]["projectV2"]
    print(f"created project {project['url']}")
    return project


def configure_project(github: GithubSession, project_id: str) -> None:
    github.graphql(
        """
        mutation($id: ID!, $title: String!, $summary: String!) {
          updateProjectV2(input: {
            projectId: $id
            public: true
            title: $title
            shortDescription: $summary
          }) { projectV2 { id } }
        }
        """,
        {"id": project_id, "title": PROJECT_TITLE, "summary": PROJECT_SUMMARY},
    )
    details = github.graphql(
        """
        query($id: ID!) {
          node(id: $id) {
            ... on ProjectV2 {
              field(name: "Status") {
                ... on ProjectV2SingleSelectField {
                  id
                  options { id name }
                }
              }
              views(first: 20) { nodes { id name layout } }
              workflows(first: 20) { nodes { id name } }
            }
          }
        }
        """,
        {"id": project_id},
    )
    project = details["node"]
    status_field = project["field"]
    if not status_field:
        raise GithubError("project has no Status field")
    github.graphql(
        """
        mutation($fieldId: ID!, $options: [ProjectV2SingleSelectFieldOptionInput!]!) {
          updateProjectV2Field(input: {
            fieldId: $fieldId
            name: "Status"
            singleSelectOptions: $options
          }) {
            projectV2Field { ... on ProjectV2SingleSelectField { id } }
          }
        }
        """,
        {
            "fieldId": status_field["id"],
            "options": status_option_inputs(status_field["options"]),
        },
    )
    print("status columns:", ", ".join(STATUS_NAMES))

    views = project["views"]["nodes"]
    board = next((view for view in views if view["layout"] == "BOARD_LAYOUT"), None)
    if board is None and views:
        board = views[0]
    if board is not None:
        github.graphql(
            """
            mutation($viewId: ID!) {
              updateProjectV2View(input: {
                viewId: $viewId
                name: "Board"
                layout: BOARD_LAYOUT
                filter: "is:issue is:open"
              }) { projectV2View { id } }
            }
            """,
            {"viewId": board["id"]},
        )
        print("board view shows open issues")

    for workflow in workflows_to_disable(project["workflows"]["nodes"]):
        github.graphql(
            """
            mutation($id: ID!) {
              deleteProjectV2Workflow(input: {workflowId: $id}) {
                deletedWorkflowId
              }
            }
            """,
            {"id": workflow["id"]},
        )
        print(f"removed built-in workflow {workflow['name']}")


def upsert_variable(github: GithubSession, repo: str, name: str, value: str) -> None:
    path = f"/repos/{repo}/actions/variables/{name}"
    try:
        github.rest("GET", path)
    except GithubError as error:
        if "failed (404)" not in str(error):
            raise
        github.rest(
            "POST",
            f"/repos/{repo}/actions/variables",
            {"name": name, "value": value},
        )
        print(f"set repository variable {name}")
        return
    github.rest("PATCH", path, {"name": name, "value": value})
    print(f"updated repository variable {name}")


def protect_main(github: GithubSession, repo: str) -> None:
    path = f"/repos/{repo}/branches/main/protection"
    body = protection_body()
    try:
        github.rest("PUT", path, body)
    except GithubError as error:
        if "Restrictions" not in str(error):
            raise
        body.pop("restrictions", None)
        github.rest("PUT", path, body)
    github.rest("PATCH", f"/repos/{repo}", {"allow_auto_merge": False})
    print("main requires a code-owner review from @Bradfordly and auto-merge is off")


def bootstrap(owner: str, repo: str, title: str, dry_run: bool) -> None:
    labels = load_labels()
    print(f"repository: {repo}")
    print(f"project owner: {owner}")
    print(f"project title: {title}")
    print("columns:", ", ".join(STATUS_NAMES))
    print("labels:", ", ".join(label["name"] for label in labels))
    print("pull request approval: @Bradfordly only")
    if dry_run:
        print("dry run: no GitHub changes were made")
        return
    github = GithubSession(token_from_environment())
    sync_labels(github, repo)
    project = find_or_create_project(github, owner, title)
    configure_project(github, project["id"])
    upsert_variable(github, repo, "PROJECT_OWNER", owner)
    upsert_variable(github, repo, "PROJECT_NUMBER", str(project["number"]))
    protect_main(github, repo)
    print()
    print(project_token_instructions(repo))
    url = project.get("url") or (
        f"https://github.com/users/{owner}/projects/{project['number']}"
    )
    print(f"Board: {url}")


class BoardClient:
    def __init__(self, github: GithubSession, repo: str, owner: str, project_number: int) -> None:
        self.github = github
        self.repo = repo
        self.owner = owner
        self.project_number = project_number
        self._project: dict[str, Any] | None = None

    def project(self) -> dict[str, Any]:
        if self._project is None:
            data = self.github.graphql(
                """
                query($login: String!, $number: Int!) {
                  user(login: $login) {
                    projectV2(number: $number) {
                      id
                      field(name: "Status") {
                        ... on ProjectV2SingleSelectField {
                          id
                          options { id name }
                        }
                      }
                    }
                  }
                }
                """,
                {"login": self.owner, "number": self.project_number},
            )
            project = data["user"]["projectV2"] if data["user"] else None
            if not project:
                raise GithubError(
                    f"project {self.project_number} was not found for {self.owner}"
                )
            self._project = project
        return self._project

    def ensure_item(self, content_id: str) -> str:
        project_id = self.project()["id"]
        data = self.github.graphql(
            """
            mutation($project: ID!, $content: ID!) {
              addProjectV2ItemById(input: {projectId: $project, contentId: $content}) {
                item { id }
              }
            }
            """,
            {"project": project_id, "content": content_id},
        )
        return data["addProjectV2ItemById"]["item"]["id"]

    def set_status(self, item_id: str, status: str) -> None:
        project = self.project()
        option_id = next(
            option["id"]
            for option in project["field"]["options"]
            if option["name"] == status
        )
        self.github.graphql(
            """
            mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
              updateProjectV2ItemFieldValue(input: {
                projectId: $project
                itemId: $item
                fieldId: $field
                value: {singleSelectOptionId: $option}
              }) { projectV2Item { id } }
            }
            """,
            {
                "project": project["id"],
                "item": item_id,
                "field": project["field"]["id"],
                "option": option_id,
            },
        )

    def archive(self, item_id: str) -> None:
        self.github.graphql(
            """
            mutation($project: ID!, $item: ID!) {
              archiveProjectV2Item(input: {projectId: $project, itemId: $item}) {
                projectV2Item { id }
              }
            }
            """,
            {"project": self.project()["id"], "item": item_id},
        )

    def issue(self, number: int) -> dict[str, Any]:
        owner, name = self.repo.split("/", 1)
        data = self.github.graphql(
            """
            query($owner: String!, $name: String!, $number: Int!) {
              repository(owner: $owner, name: $name) {
                issue(number: $number) {
                  id
                  number
                  state
                  assignees { totalCount }
                  closedByPullRequestsReferences(first: 20) {
                    nodes { state }
                  }
                }
              }
            }
            """,
            {"owner": owner, "name": name, "number": number},
        )
        issue = data["repository"]["issue"]
        if issue is None:
            raise GithubError(f"issue #{number} was not found")
        return issue

    def closing_issue_numbers(self, pr_number: int) -> list[int]:
        owner, name = self.repo.split("/", 1)
        data = self.github.graphql(
            """
            query($owner: String!, $name: String!, $number: Int!) {
              repository(owner: $owner, name: $name) {
                pullRequest(number: $number) {
                  closingIssuesReferences(first: 20) {
                    nodes { number }
                  }
                }
              }
            }
            """,
            {"owner": owner, "name": name, "number": pr_number},
        )
        pull_request = data["repository"]["pullRequest"]
        if pull_request is None:
            return []
        return [
            node["number"]
            for node in pull_request["closingIssuesReferences"]["nodes"]
        ]

    def close_issue(self, number: int) -> None:
        self.github.rest(
            "PATCH",
            f"/repos/{self.repo}/issues/{number}",
            {"state": "closed", "state_reason": "completed"},
        )


def place_issue(client: BoardClient, issue: dict[str, Any]) -> str:
    item_id = client.ensure_item(issue["id"])
    if issue["state"] == "CLOSED":
        client.archive(item_id)
        return "archived"
    open_prs = [
        pr
        for pr in issue["closedByPullRequestsReferences"]["nodes"]
        if pr["state"] == "OPEN"
    ]
    status = desired_status(
        assignee_count=issue["assignees"]["totalCount"],
        has_open_pr=bool(open_prs),
    )
    client.set_status(item_id, status)
    return status


def sync_event(payload: dict[str, Any], event_name: str, client: BoardClient) -> list[str]:
    results: list[str] = []
    if event_name == "issues":
        number = payload["issue"]["number"]
        issue = client.issue(number)
        if payload.get("action") == "closed":
            issue = {**issue, "state": "CLOSED"}
        outcome = place_issue(client, issue)
        results.append(f"#{number} {outcome}")
        return results

    if event_name == "pull_request":
        pull_request = payload["pull_request"]
        numbers = linked_issue_numbers(pull_request.get("body"))
        for number in client.closing_issue_numbers(pull_request["number"]):
            if number not in numbers:
                numbers.append(number)
        merged = bool(pull_request.get("merged"))
        for number in numbers:
            issue = client.issue(number)
            if merged:
                client.close_issue(number)
                issue = {**issue, "state": "CLOSED"}
            elif pull_request.get("state") == "open":
                issue = {
                    **issue,
                    "closedByPullRequestsReferences": {"nodes": [{"state": "OPEN"}]},
                }
            outcome = place_issue(client, issue)
            results.append(f"#{number} {outcome}")
        return results

    raise GithubError(f"unsupported event {event_name}")


def run_sync_event(event_name: str, event_path: str, owner: str, repo: str, project_number: str) -> None:
    if not project_number:
        print("PROJECT_NUMBER is unset; board sync skipped")
        return
    owner = owner or "Bradfordly"
    repo = repo or "Bradfordly/bradfordly-games"
    token = token_from_environment()
    payload = json.loads(Path(event_path).read_text())
    client = BoardClient(GithubSession(token), repo, owner, int(project_number))
    for line in sync_event(payload, event_name, client):
        print(line)


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    bootstrap_parser = sub.add_parser("bootstrap")
    bootstrap_parser.add_argument("--owner", default="Bradfordly")
    bootstrap_parser.add_argument("--repo", default="Bradfordly/bradfordly-games")
    bootstrap_parser.add_argument("--title", default=PROJECT_TITLE)
    bootstrap_parser.add_argument("--dry-run", action="store_true")

    sync_parser = sub.add_parser("sync-event")
    sync_parser.add_argument("--event-name", default=os.environ.get("GITHUB_EVENT_NAME", ""))
    sync_parser.add_argument("--event-path", default=os.environ.get("GITHUB_EVENT_PATH", ""))
    sync_parser.add_argument("--owner", default=os.environ.get("PROJECT_OWNER", "Bradfordly"))
    sync_parser.add_argument(
        "--repo",
        default=os.environ.get("GITHUB_REPOSITORY", "Bradfordly/bradfordly-games"),
    )
    sync_parser.add_argument("--project-number", default=os.environ.get("PROJECT_NUMBER", ""))
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    sys.stdout.reconfigure(line_buffering=True)
    args = parse_args(sys.argv[1:] if argv is None else argv)
    if args.command == "bootstrap":
        bootstrap(args.owner, args.repo, args.title, args.dry_run)
        return
    if not args.event_name or not args.event_path:
        raise GithubError("sync-event needs an event name and event path")
    run_sync_event(
        args.event_name,
        args.event_path,
        args.owner,
        args.repo,
        args.project_number,
    )


if __name__ == "__main__":
    try:
        main()
    except GithubError as error:
        print(f"error: {error}", file=sys.stderr)
        sys.exit(1)
