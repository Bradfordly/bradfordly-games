Feature: Invite-only panel at games.bradfordly.com
  The control plane requires GitHub login and an SSM allowlist.
  GET /healthz is the only operational route that skips authentication.

  Scenario: Healthz does not require a session
    When an anonymous client gets "/healthz"
    Then the response status is 200

  Scenario: Unauthenticated HTML redirects to login
    When an anonymous client gets "/"
    Then the response status is 302
    And the Location header is "/login"

  Scenario: Unauthenticated API returns 401
    When an anonymous client gets "/api/worlds"
    Then the response status is 401

  Scenario: Login page offers GitHub
    When an anonymous client gets "/login"
    Then the response status is 200
    And the page contains "Sign in with GitHub"

  Scenario: Authenticated user not on the allowlist sees Denied
    Given GitHub will authenticate "outsider@example.com" with subject "99"
    And the allowlist contains "owner@example.com"
    When the user signs in with GitHub
    Then they land on "/denied"
    When they get "/denied"
    Then the page contains "Denied"

  Scenario: Allowlisted user sees world list fields
    Given GitHub will authenticate "owner@example.com" with subject "1"
    And the allowlist contains "owner@example.com"
    And a world "survival" exists with allocation "survival.games.bradfordly.com:25565"
    And world "survival" is in gateway state "asleep"
    When the user signs in with GitHub
    And they get "/"
    Then the page contains "survival.games.bradfordly.com:25565"
    And the page contains "asleep — join the game to start"

  Scenario: World detail shows operations the panel must not hide
    Given an allowlisted signed-in user
    And a world "survival" exists with allocation "survival.games.bradfordly.com:25565"
    And world "survival" is failed with 2 players, wake "manual", a forced stop, and 4.5 hours online
    When they get "/worlds/survival"
    Then the page contains "failed"
    And the page contains "manual"
    And the page contains "4.5"
    And the page contains "data-confirm-online"

  Scenario: Manual start forwards to the gateway
    Given an allowlisted signed-in user
    And a world "survival" exists with allocation "survival.games.bradfordly.com:25565"
    When they post power action "start" to "/api/worlds/survival/power"
    Then the response status is 200
    And the gateway received power "start" for "survival"

  Scenario: Session cookie is Secure, HttpOnly, and SameSite=Lax
    Given GitHub will authenticate "owner@example.com" with subject "1"
    And the allowlist contains "owner@example.com"
    When the user signs in with GitHub
    Then the session cookie is Secure, HttpOnly, and SameSite=Lax

  Scenario: File manager and billing routes are absent
    Given an allowlisted signed-in user
    When they get "/files"
    Then the response status is 404
    When they get "/billing"
    Then the response status is 404
