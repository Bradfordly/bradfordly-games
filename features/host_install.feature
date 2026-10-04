Feature: Option G host bootstrap
  The #57 public EC2 host attaches this cloud-init. Docker, Caddy, and the
  data-volume layout must exist. Panel and gateway are containers, not
  host systemd units.

  Scenario: Data volume layout for panel state and world saves
    Given a temporary data volume mount
    When the host bootstrap applies the data-directory layout
    Then the panel state directory exists
    And the world save directory exists
    And Caddy certificate storage exists on the data volume

  Scenario: Caddy terminates HTTPS for the panel on 443 only
    When the host bootstrap renders the Caddyfile
    Then Caddy serves games.bradfordly.com
    And Caddy enables Let's Encrypt without opening port 80
    And Caddy reverse proxies to the panel container

  Scenario: Panel and gateway run as Docker containers
    When the host bootstrap renders compose and user-data
    Then compose defines caddy, panel, and gateway services
    And Caddy publishes TCP 443 only
    And no systemd unit runs the panel or gateway on the host
    And user-data installs Docker Engine
    And user-data starts only the Caddy container
