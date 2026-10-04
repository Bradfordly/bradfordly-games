Feature: Stop a world container after the idle timeout
  The gateway frees RAM by stopping the game container. It does not stop the EC2 instance.

  Background:
    Given a Minecraft world "survival" in container "mc-survival"
    And idle_timeout is 20ms
    And stop_timeout is 20ms

  Scenario: zero play connections stop the container after idle_timeout
    Given the world is online
    And the adapter reports 0 play connections
    When the gateway ticks once
    Then the world state is "idle_wait"
    When the idle timer elapses
    And the gateway ticks once
    Then the world becomes "asleep"
    And the runtime received SIGTERM for "mc-survival"
    And the panel does not show a forced stop
    And the host instance was not stopped

  Scenario: a player returning cancels idle_wait
    Given the world is online
    And the adapter reports 0 play connections
    When the gateway ticks once
    Then the world state is "idle_wait"
    When the adapter reports 1 play connection
    And the gateway ticks once
    Then the world state is "online"
    When the idle timer elapses
    And the gateway ticks once
    Then the world state is "online"
    And the container was not stopped

  Scenario: unknown activity does not start the idle timer
    Given a Valheim world "vale" in container "vale"
    And the world is online
    When the gateway ticks once
    Then the world state is "online"
    When the idle timer elapses
    And the gateway ticks once
    Then the world state is "online"
    And the container was not stopped

  Scenario: a hard kill after stop_timeout is a forced stop
    Given the world is online
    And the container ignores SIGTERM
    And the adapter reports 0 play connections
    When the gateway ticks once
    And the idle timer elapses
    And the gateway ticks once
    Then the world becomes "asleep"
    And the runtime received SIGKILL for "mc-survival"
    And the panel shows a forced stop
    And the host instance was not stopped
