from __future__ import annotations

import tempfile
from pathlib import Path

from behave import given, then, when

from host.bootstrap import (
    DATA_LABEL,
    FORBIDDEN_HOST_UNITS,
    HTTPS_PORT,
    PANEL_HOSTNAME,
    PANEL_LISTEN_PORT,
    apply_layout,
    layout_paths,
    render_bootstrap_script,
    render_caddyfile,
    render_compose,
    render_user_data,
    systemd_host_units,
)


@given("a temporary data volume mount")
def given_temporary_mount(context) -> None:
    context._tmpdir = tempfile.TemporaryDirectory()
    context.data_root = Path(context._tmpdir.name) / "games-data"


@when("the host bootstrap applies the data-directory layout")
def when_apply_layout(context) -> None:
    context.created = apply_layout(context.data_root)
    context.paths = layout_paths(context.data_root)


@when("the host bootstrap renders the Caddyfile")
def when_render_caddyfile(context) -> None:
    context.caddyfile = render_caddyfile()


@when("the host bootstrap renders compose and user-data")
def when_render_stack(context) -> None:
    context.compose = render_compose()
    context.user_data = render_user_data()
    context.script = render_bootstrap_script()


@then("the panel state directory exists")
def then_panel_dir(context) -> None:
    assert context.paths["panel"].is_dir()


@then("the world save directory exists")
def then_worlds_dir(context) -> None:
    assert context.paths["worlds"].is_dir()


@then("Caddy certificate storage exists on the data volume")
def then_caddy_data(context) -> None:
    assert context.paths["caddy_data"].is_dir()
    assert context.paths["caddy_config"].is_dir()


@then("Caddy serves games.bradfordly.com")
def then_caddy_hostname(context) -> None:
    assert PANEL_HOSTNAME in context.caddyfile


@then("Caddy enables Let's Encrypt without opening port 80")
def then_caddy_tls_alpn(context) -> None:
    assert "auto_https disable_redirects" in context.caddyfile
    assert "http://" not in context.caddyfile
    assert "80:80" not in context.caddyfile


@then("Caddy reverse proxies to the panel container")
def then_caddy_proxy(context) -> None:
    assert f"reverse_proxy panel:{PANEL_LISTEN_PORT}" in context.caddyfile


@then("compose defines caddy, panel, and gateway services")
def then_compose_services(context) -> None:
    for name in ("caddy:", "panel:", "gateway:"):
        assert name in context.compose


@then("Caddy publishes TCP 443 only")
def then_caddy_443(context) -> None:
    assert f'"{HTTPS_PORT}:{HTTPS_PORT}"' in context.compose
    assert "80:80" not in context.compose


@then("no systemd unit runs the panel or gateway on the host")
def then_no_host_units(context) -> None:
    assert systemd_host_units() == ("docker.service",)
    for unit in FORBIDDEN_HOST_UNITS:
        assert unit not in context.user_data
        assert unit not in context.script
        assert f"systemctl enable --now {unit.removesuffix('.service')}" not in context.script


@then("user-data installs Docker Engine")
def then_user_data_docker(context) -> None:
    assert context.user_data.startswith("#cloud-config")
    assert "- docker" in context.user_data
    assert "dnf install -y docker" in context.script
    assert DATA_LABEL in context.script


@then("user-data starts only the Caddy container")
def then_start_caddy_only(context) -> None:
    assert "up -d caddy" in context.script
    assert "up -d panel" not in context.script
    assert "up -d gateway" not in context.script
