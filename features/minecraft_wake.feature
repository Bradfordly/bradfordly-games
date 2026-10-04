Feature: Answer Minecraft status pings and start on login
  The public listener is itzg/mc-router. This gateway owns docker start,
  occupy, wake_whitelist, rate limits, and the localhost admin port.

  Scenario: status ping does not start a container
    Given an asleep world "survival" on host "survival.games.bradfordly.com"
    When mc-router classifies a status ping for "survival"
    Then docker start is not called
    And the world "survival" is "asleep"

  Scenario: login starts the matched world
    Given an asleep world "survival" on host "survival.games.bradfordly.com"
    When mc-router classifies a login for player "Steve" on "survival"
    Then docker start is called for "survival"
    And the world "survival" is "starting"
    And occupy is kick with a starting message

  Scenario: wake whitelist rejects an unknown player
    Given an asleep world "survival" on host "survival.games.bradfordly.com"
    And wake_whitelist is "Steve"
    When mc-router classifies a login for player "Alex" on "survival"
    Then docker start is not called
    And the world "survival" is "asleep"

  Scenario: one start per world per 30 seconds
    Given an asleep world "survival" on host "survival.games.bradfordly.com"
    When mc-router classifies a login for player "Steve" on "survival"
    And the world returns to asleep immediately
    And another login for player "Steve" arrives on "survival" within 30 seconds
    Then the second start is rate-limited

  Scenario: do not start a second world while another is running
    Given an asleep world "survival" on host "survival.games.bradfordly.com"
    And an asleep world "creative" on host "creative.games.bradfordly.com"
    When mc-router classifies a login for player "Steve" on "survival"
    And mc-router classifies a login for player "Alex" on "creative"
    Then docker start is called for "survival"
    And docker start is not called for "creative"

  Scenario: healthz and worlds on the localhost admin port
    Given an asleep world "survival" on host "survival.games.bradfordly.com"
    When an operator calls GET /healthz
    Then the admin response is 200
    When an operator calls GET /worlds
    Then the admin response is 200
    And the worlds list includes "survival" as "asleep"
