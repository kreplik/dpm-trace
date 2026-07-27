"""Checks for command-building helpers.

Tests parse_scalar, parse_arg_assignments, command_arguments, normalize_commands_json,
explicit_js_command, and prepare_user_id. No network calls are made.
"""

from __future__ import annotations

import argparse
import io
import sys
from contextlib import redirect_stderr
from pathlib import Path

_LABEL_W = 52
_errors: list[str] = []
_section = ""


def section(title: str) -> None:
    global _section
    _section = title
    print(f"\n{title}")
    print("─" * max(len(title), 40))


def check(
    label: str, condition: bool, actual=None, *, fail_msg: str | None = None
) -> None:
    status = "PASS" if condition else "FAIL"
    suffix = f"  got={actual!r}" if actual is not None else ""
    print(f"  {label:<{_LABEL_W}} {status}{suffix}")
    if not condition:
        _errors.append(fail_msg or f"{_section} / {label}")


def check_raises(label: str, fn, exc_type=Exception) -> None:
    try:
        fn()
        check(label, False, "no exception raised")
    except exc_type:
        check(label, True)
    except Exception as exc:
        check(label, False, repr(exc))


def capture_stderr(fn) -> str:
    buf = io.StringIO()
    with redirect_stderr(buf):
        fn()
    return buf.getvalue()


def ns(**kwargs) -> argparse.Namespace:
    """Build a minimal Namespace for command_arguments / explicit_js_command."""
    defaults = dict(
        args_json=None,
        args_file=None,
        arg=[],
        template=None,
        contract_id=None,
        choice=None,
        user_id=None,
        token=None,
        token_file=None,
        ledger_url=None,
    )
    defaults.update(kwargs)
    return argparse.Namespace(**defaults)


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-command-build.py <repo-root>", file=sys.stderr)
        return 2

    repo_root = Path(sys.argv[1]).resolve()
    sys.path.insert(0, str(repo_root / "src"))

    from dpm_trace.cli import (
        command_arguments,
        explicit_js_command,
        normalize_commands_json,
        parse_arg_assignments,
        parse_scalar,
        prepare_user_id,
    )

    # ── parse_scalar ──────────────────────────────────────────────────────────
    section("parse_scalar")
    check("null → None", parse_scalar("null") is None)
    check("true → True", parse_scalar("true") is True)
    check("false → False", parse_scalar("false") is False)
    check("42 → int 42", parse_scalar("42") == 42, parse_scalar("42"))
    check("-7 → int -7", parse_scalar("-7") == -7, parse_scalar("-7"))
    check("3.14 → float", abs(parse_scalar("3.14") - 3.14) < 1e-9, parse_scalar("3.14"))
    check("JSON object", parse_scalar('{"a":1}') == {"a": 1}, parse_scalar('{"a":1}'))
    check("JSON array", parse_scalar("[1,2]") == [1, 2], parse_scalar("[1,2]"))
    check("plain string", parse_scalar("hello") == "hello", parse_scalar("hello"))
    check(
        "mixed str not int", parse_scalar("123abc") == "123abc", parse_scalar("123abc")
    )

    # ── parse_arg_assignments ─────────────────────────────────────────────────
    section("parse_arg_assignments")
    result = parse_arg_assignments(["count=5", "name=Alice", "flag=true"])
    check("count coerced to int", result.get("count") == 5, result.get("count"))
    check("name stays string", result.get("name") == "Alice", result.get("name"))
    check("flag coerced to bool", result.get("flag") is True, result.get("flag"))
    check_raises("missing = raises", lambda: parse_arg_assignments(["badvalue"]))
    check_raises("empty key raises", lambda: parse_arg_assignments(["=val"]))

    # ── command_arguments ─────────────────────────────────────────────────────
    section("command_arguments")
    check("no flags → empty dict", command_arguments(ns()) == {})

    a = command_arguments(ns(args_json='{"owner":"Alice","count":0}'))
    check("--args-json parsed", a == {"owner": "Alice", "count": 0}, a)

    b = command_arguments(ns(args_json='{"owner":"Alice"}', arg=["count=7"]))
    check("--arg overlays --args-json key", b.get("count") == 7, b)
    check("--arg preserves base keys", b.get("owner") == "Alice", b)

    c = command_arguments(ns(arg=["x=null", "y=false"]))
    check("--arg only: null coercion", c.get("x") is None, c)
    check("--arg only: false coercion", c.get("y") is False, c)

    check_raises(
        "--args-json + --args-file raises",
        lambda: command_arguments(ns(args_json="{}", args_file="f.json")),
    )
    check_raises(
        "--arg on non-dict base raises",
        lambda: command_arguments(ns(args_json='"scalar"', arg=["k=1"])),
    )

    # ── normalize_commands_json ───────────────────────────────────────────────
    section("normalize_commands_json")
    cmd = {"CreateCommand": {"templateId": "T:T", "createArguments": {}}}

    r1 = normalize_commands_json([cmd])
    check("list passthrough", r1 == [cmd], len(r1))

    r2 = normalize_commands_json(cmd)
    check("bare dict wrapped in list", r2 == [cmd], len(r2))

    r3 = normalize_commands_json({"commands": [cmd]})
    check("wrapper object unwrapped", r3 == [cmd], len(r3))

    check_raises("scalar raises", lambda: normalize_commands_json("bad"))
    check_raises("int raises", lambda: normalize_commands_json(42))

    # ── explicit_js_command ───────────────────────────────────────────────────
    section("explicit_js_command")

    create_cmd = explicit_js_command(
        ns(template="Pkg:Mod:T", args_json='{"owner":"Alice"}')
    )
    check("create: top-level key", "CreateCommand" in create_cmd, list(create_cmd))
    check(
        "create: templateId", create_cmd["CreateCommand"]["templateId"] == "Pkg:Mod:T"
    )
    check(
        "create: createArguments from --args-json",
        create_cmd["CreateCommand"]["createArguments"] == {"owner": "Alice"},
    )

    ex_cmd = explicit_js_command(
        ns(
            template="Pkg:Mod:T",
            contract_id="#1:0",
            choice="Transfer",
            args_json='{"amount":10}',
        )
    )
    check("exercise: top-level key", "ExerciseCommand" in ex_cmd, list(ex_cmd))
    check(
        "exercise: templateId", ex_cmd["ExerciseCommand"]["templateId"] == "Pkg:Mod:T"
    )
    check("exercise: contractId", ex_cmd["ExerciseCommand"]["contractId"] == "#1:0")
    check("exercise: choice", ex_cmd["ExerciseCommand"]["choice"] == "Transfer")
    check(
        "exercise: choiceArgument",
        ex_cmd["ExerciseCommand"]["choiceArgument"] == {"amount": 10},
    )

    check_raises(
        "exercise without --contract-id raises",
        lambda: explicit_js_command(ns(template="T:T", choice="Foo")),
    )
    check_raises(
        "exercise without --choice raises",
        lambda: explicit_js_command(ns(template="T:T", contract_id="#1:0")),
    )

    # ── prepare_user_id ───────────────────────────────────────────────────────
    section("prepare_user_id")

    uid = prepare_user_id(ns(ledger_url="http://localhost:7575"))
    check("localhost → participant_admin, no warning", uid == "participant_admin", uid)

    uid2 = prepare_user_id(ns(ledger_url="http://127.0.0.1:7575"))
    check(
        "127.0.0.1 → participant_admin, no warning", uid2 == "participant_admin", uid2
    )

    stderr_out = capture_stderr(
        lambda: prepare_user_id(ns(ledger_url="http://remote.example.com:9999"))
    )
    check("remote host emits warning", "warning:" in stderr_out, None)
    check("warning names the host", "remote.example.com" in stderr_out, None)
    check(
        "remote still returns participant_admin",
        prepare_user_id(ns(ledger_url="http://remote.example.com:9999"))
        == "participant_admin",
    )

    uid3 = prepare_user_id(
        ns(user_id="my-user", ledger_url="http://remote.example.com:9999")
    )
    check("explicit --user-id returned as-is", uid3 == "my-user", uid3)

    uid4 = prepare_user_id(ns(token="tok", ledger_url="http://remote.example.com:9999"))
    check("token present → None (no user-id injected)", uid4 is None, uid4)

    uid5 = prepare_user_id(
        ns(token_file="/tmp/t", ledger_url="http://remote.example.com:9999")
    )
    check("token_file present → None", uid5 is None, uid5)

    # ── result ────────────────────────────────────────────────────────────────
    print()
    if _errors:
        print("dpm trace command-build checks FAILED:")
        for err in _errors:
            print(f"  - {err}")
        return 1
    print("dpm trace command-build checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
