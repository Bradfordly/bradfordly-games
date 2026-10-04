Feature: v1 backups for the games data volume
  Scheduled EBS snapshots cover the 50 GB data volume from #57.
  Destroying a save takes one on-demand snapshot first.
  Restore is a new world pointed at a restored directory.

  Scenario: weekly snapshots target the tagged data volume
    Then a DLM policy retains weekly snapshots of volumes tagged Role=data-volume
    And the backups module does not create the EC2 host or data volume

  Scenario: destroying a save directory snapshots first
    Given a world save directory exists on the data volume
    And the data volume id is configured
    When the operator deletes the world and asks to destroy the save
    Then an on-demand snapshot of the data volume is created
    And the save directory is removed only after the snapshot is requested

  Scenario: a failed snapshot blocks destroy
    Given a world save directory exists on the data volume
    And snapshot creation will fail
    When the operator deletes the world and asks to destroy the save
    Then the save directory is still present

  Scenario: keeping a save does not snapshot
    Given a world save directory exists on the data volume
    When the operator deletes the world and keeps the save
    Then no snapshot is created
    And the save directory is still present

  Scenario: restore points a new world at a restored directory
    Given a restored save directory exists
    When the operator creates a new world pointed at that directory
    Then the world volume is the restored directory
    And no world files are copied through the panel API
