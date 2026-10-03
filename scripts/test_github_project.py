import unittest
from pathlib import Path

from scripts.github_project import (
    STATUS_BACKLOG,
    STATUS_IN_PROGRESS,
    STATUS_REVIEW,
    GithubError,
    desired_status,
    issue_templates_use_known_labels,
    label_plan,
    linked_issue_numbers,
    load_labels,
    missing_token_message,
    project_token_instructions,
    protection_body,
    resolve_token,
    status_option_inputs,
    sync_event,
    workflows_to_disable,
)

ROOT = Path(__file__).resolve().parents[1]


class Rules(unittest.TestCase):
    def test_category_labels_are_exactly_the_required_set(self) -> None:
        self.assertEqual(
            [label["name"] for label in load_labels()],
            ["documentation", "config change", "bug fix", "feature"],
        )

    def test_issue_templates_use_those_labels(self) -> None:
        self.assertEqual(issue_templates_use_known_labels(), [])

    def test_agents_cannot_approve_pull_requests(self) -> None:
        rules = (ROOT / "AGENTS.md").read_text()
        self.assertIn("Do not approve a pull request.", rules)
        self.assertIn("Do not enable auto-merge.", rules)
        self.assertIn("@Bradfordly", rules)

    def test_codeowners_is_only_the_repository_owner(self) -> None:
        owners = (ROOT / ".github" / "CODEOWNERS").read_text()
        self.assertIn("* @Bradfordly", owners)
        self.assertNotIn("[bot]", owners)

    def test_project_token_uses_a_classic_scope_instead_of_an_account_permission(self) -> None:
        instructions = project_token_instructions("Bradfordly/bradfordly-games")
        missing = missing_token_message()
        for text in (instructions, missing):
            self.assertNotIn("Account permission", text)
            self.assertNotIn("fine-grained personal access token as the PROJECT_TOKEN", text)
            self.assertIn("classic", text.casefold())
        self.assertIn("Tokens (classic)", instructions)
        self.assertIn("project scope", instructions)
        self.assertIn("public_repo", instructions)
        self.assertIn("does not offer a Projects account permission", instructions)
        self.assertIn("gh secret set PROJECT_TOKEN --repo Bradfordly/bradfordly-games", instructions)

    def test_token_resolution_prefers_the_environment_then_gh(self) -> None:
        self.assertEqual(resolve_token({"GH_TOKEN": "from-env"}, lambda: "from-gh"), "from-env")
        self.assertEqual(resolve_token({}, lambda: "from-gh\n"), "from-gh")

        def unavailable() -> str:
            raise OSError("gh is not installed")

        with self.assertRaises(GithubError) as raised:
            resolve_token({}, unavailable)
        self.assertIn("project", str(raised.exception))
        self.assertNotIn("Account permission", str(raised.exception))

    def test_bot_approval_workflow_dismisses_bots_only(self) -> None:
        workflow = (ROOT / ".github" / "workflows" / "dismiss-bot-approval.yml").read_text()
        self.assertIn("github.event.review.user.login != 'Bradfordly'", workflow)
        self.assertIn("github.event.review.user.type == 'Bot'", workflow)
        self.assertIn("dismissals", workflow)


class BoardRules(unittest.TestCase):
    def test_status_follows_assignment_and_pull_requests(self) -> None:
        self.assertEqual(
            desired_status(assignee_count=0, has_open_pr=False),
            STATUS_BACKLOG,
        )
        self.assertEqual(
            desired_status(assignee_count=1, has_open_pr=False),
            STATUS_IN_PROGRESS,
        )
        self.assertEqual(
            desired_status(assignee_count=1, has_open_pr=True),
            STATUS_REVIEW,
        )
        self.assertEqual(
            desired_status(assignee_count=0, has_open_pr=True),
            STATUS_REVIEW,
        )

    def test_status_columns_replace_the_github_defaults(self) -> None:
        options = status_option_inputs(
            [
                {"id": "todo", "name": "Todo"},
                {"id": "doing", "name": "In Progress"},
                {"id": "done", "name": "Done"},
            ]
        )
        self.assertEqual(
            [(option["name"], option.get("id")) for option in options],
            [
                (STATUS_BACKLOG, "todo"),
                (STATUS_IN_PROGRESS, "doing"),
                (STATUS_REVIEW, None),
            ],
        )
        self.assertNotIn("done", {option.get("id") for option in options})

    def test_done_workflows_are_removed(self) -> None:
        selected = workflows_to_disable(
            [
                {"id": "1", "name": "Item added to project"},
                {"id": "2", "name": "Item closed"},
                {"id": "3", "name": "Pull request merged"},
                {"id": "4", "name": "Item reopened"},
            ]
        )
        self.assertEqual([item["id"] for item in selected], ["2", "3"])

    def test_closing_keywords_find_issue_numbers(self) -> None:
        body = "Closes #12\nAlso fixes #12 and Resolves #4."
        self.assertEqual(linked_issue_numbers(body), [12, 4])

    def test_default_labels_outside_the_category_set_are_removed(self) -> None:
        plan = label_plan(["bug", "documentation", "enhancement", "question"])
        self.assertEqual(plan["delete"], ["bug", "enhancement", "question"])
        self.assertIn("feature", plan["create"])
        self.assertIn("documentation", plan["update"])

    def test_branch_protection_requires_the_owner_review(self) -> None:
        body = protection_body()
        reviews = body["required_pull_request_reviews"]
        self.assertTrue(reviews["require_code_owner_reviews"])
        self.assertEqual(reviews["required_approving_review_count"], 1)
        self.assertTrue(reviews["dismiss_stale_reviews"])
        self.assertTrue(body["enforce_admins"])
        self.assertFalse(body["allow_force_pushes"])


class FakeBoard:
    def __init__(self) -> None:
        self.issues: dict[int, dict] = {}
        self.statuses: dict[str, str] = {}
        self.archived: list[str] = []
        self.closed: list[int] = []
        self.prs: dict[int, list[int]] = {}

    def issue(self, number: int) -> dict:
        return self.issues[number]

    def ensure_item(self, content_id: str) -> str:
        return f"item-{content_id}"

    def set_status(self, item_id: str, status: str) -> None:
        self.statuses[item_id] = status

    def archive(self, item_id: str) -> None:
        self.archived.append(item_id)

    def close_issue(self, number: int) -> None:
        self.closed.append(number)

    def closing_issue_numbers(self, pr_number: int) -> list[int]:
        return list(self.prs.get(pr_number, []))


def issue(number: int, assignees: int, prs: list[str] | None = None, state: str = "OPEN") -> dict:
    return {
        "id": f"issue-{number}",
        "number": number,
        "state": state,
        "assignees": {"totalCount": assignees},
        "closedByPullRequestsReferences": {
            "nodes": [{"state": pr_state} for pr_state in (prs or [])]
        },
    }


class Sync(unittest.TestCase):
    def test_opened_issue_without_an_assignee_stays_in_backlog(self) -> None:
        board = FakeBoard()
        board.issues[7] = issue(7, 0)
        result = sync_event(
            {"action": "opened", "issue": {"number": 7}},
            "issues",
            board,  # type: ignore[arg-type]
        )
        self.assertEqual(result, ["#7 Backlog"])
        self.assertEqual(board.statuses["item-issue-7"], STATUS_BACKLOG)

    def test_assignment_moves_the_issue_in_progress(self) -> None:
        board = FakeBoard()
        board.issues[7] = issue(7, 1)
        sync_event(
            {"action": "assigned", "issue": {"number": 7}},
            "issues",
            board,  # type: ignore[arg-type]
        )
        self.assertEqual(board.statuses["item-issue-7"], STATUS_IN_PROGRESS)

    def test_open_pull_request_moves_the_issue_to_review(self) -> None:
        board = FakeBoard()
        board.issues[7] = issue(7, 1)
        result = sync_event(
            {
                "action": "opened",
                "pull_request": {"number": 3, "body": "Closes #7", "state": "open", "merged": False},
            },
            "pull_request",
            board,  # type: ignore[arg-type]
        )
        self.assertEqual(result, ["#7 Review"])
        self.assertEqual(board.closed, [])

    def test_merged_pull_request_closes_and_archives_the_issue(self) -> None:
        board = FakeBoard()
        board.issues[7] = issue(7, 1, prs=["OPEN"])
        board.prs[3] = [7]
        result = sync_event(
            {
                "action": "closed",
                "pull_request": {
                    "number": 3,
                    "body": "Closes #7",
                    "state": "closed",
                    "merged": True,
                },
            },
            "pull_request",
            board,  # type: ignore[arg-type]
        )
        self.assertEqual(board.closed, [7])
        self.assertEqual(board.archived, ["item-issue-7"])
        self.assertEqual(result, ["#7 archived"])

    def test_closed_unmerged_pull_request_returns_assigned_work_to_in_progress(self) -> None:
        board = FakeBoard()
        board.issues[7] = issue(7, 1, prs=[])
        sync_event(
            {
                "action": "closed",
                "pull_request": {
                    "number": 3,
                    "body": "Closes #7",
                    "state": "closed",
                    "merged": False,
                },
            },
            "pull_request",
            board,  # type: ignore[arg-type]
        )
        self.assertEqual(board.statuses["item-issue-7"], STATUS_IN_PROGRESS)
        self.assertEqual(board.closed, [])


if __name__ == "__main__":
    unittest.main()
