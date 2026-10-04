from __future__ import annotations


def after_scenario(context, scenario) -> None:
    tmp = getattr(context, "_tmpdir", None)
    if tmp is not None:
        tmp.cleanup()
