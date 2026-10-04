Feature: Store worlds on disk and create a stopped Minecraft container

  A local panel with a data-volume SQLite file and a fake Docker Engine
  exercises issue #60. The data directory and Docker daemon are assumed
  to exist on the G host (#57/#58); this environment substitutes them.

  Scenario: Creating a world writes SQLite, a save directory, and a stopped container
    Given a local panel with a data volume and Docker API
    When an operator creates a minecraft-java world named "Survival"
    Then a SQLite world row exists for "Survival"
    And a save directory exists on the data volume
    And a stopped itzg/minecraft-server container exists with EULA=TRUE
    And the container bind-mounts the save directory on /data
    And the container does not publish RCON
    And allocation.host is a name under games.bradfordly.com

  Scenario: Creating a non-minecraft world is rejected
    Given a local panel with a data volume and Docker API
    When an operator creates a valheim world named "Mead"
    Then the request is rejected

  Scenario: PATCH updates settings without recreating the save directory
    Given a local panel with a data volume and Docker API
    And an existing world named "Survival"
    When an operator patches idle_timeout to "30m" and env DIFFICULTY=hard
    Then the SQLite row has idle_timeout "30m0s" and DIFFICULTY hard
    And the container env includes DIFFICULTY=hard
    And the save directory is the same path

  Scenario: DELETE drains, removes the container, and keeps the save
    Given a local panel with a data volume and Docker API
    And an existing world named "Survival"
    When an operator deletes the world
    Then the gateway was asked to drain
    And the container is gone
    And the save directory still exists
    And the SQLite row is gone

  Scenario: DELETE with destroy removes the save directory
    Given a local panel with a data volume and Docker API
    And an existing world named "Survival"
    When an operator deletes the world and asks to destroy the save
    Then the save directory is gone
