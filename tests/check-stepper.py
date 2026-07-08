import io
import sys
from contextlib import redirect_stdout
from pathlib import Path

_LABEL_W = 48
_errors: list[str] = []
_section = ""


def section(title: str) -> None:
    global _section
    _section = title
    print(f"\n{title}")
    print("─" * max(len(title), 40))


def check(label: str, condition: bool, actual=None, *, fail_msg: str | None = None) -> None:
    status = "PASS" if condition else "FAIL"
    suffix = f"  {actual}" if actual is not None else ""
    print(f"  {label:<{_LABEL_W}} {status}{suffix}")
    if not condition:
        _errors.append(fail_msg or f"{_section} / {label}")


def capture(fn) -> str:
    buf = io.StringIO()
    with redirect_stdout(buf):
        fn()
    return buf.getvalue()


# ── fixture ───────────────────────────────────────────────────────────────────

def make_trace():
    from dpm_trace.cli import NormalizedTrace, TraceEvent

    events = {
        "ev-create": TraceEvent(
            event_id="ev-create",
            kind="create",
            template="pkg1aabb:Asset:Asset",
            contract_id="#1:0",
            signatories=["Alice"],
            payload={"owner": "Alice", "balance": 0},
        ),
        "ev-exercise": TraceEvent(
            event_id="ev-exercise",
            kind="exercise",
            template="pkg2ccdd:Token:Token",
            choice="Transfer",
            consuming=True,
            acting_parties=["Bob"],
            argument={"newOwner": "Bob"},
            child_event_ids=["ev-child"],
        ),
        "ev-child": TraceEvent(
            event_id="ev-child",
            kind="create",
            template="pkg2ccdd:Token:Token",
            contract_id="#2:1",
        ),
    }
    return NormalizedTrace(
        update_id="update-stepper-test-001",
        source="scan",
        source_url=None,
        projection={},
        root_event_ids=["ev-create", "ev-exercise"],
        events_by_id=events,
    )

def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-stepper.py <repo-root>", file=sys.stderr)
        return 2

    repo_root = Path(sys.argv[1]).resolve()
    sys.path.insert(0, str(repo_root / "src"))

    trace = make_trace()

    from dpm_trace.cli import Breakpoint, Color, RenderContext, SourceLocation, Stepper

    stepper = Stepper(trace, color=Color(False))

    # ── _preorder ──────────────────────────────────────────────────────────
    section("_preorder")
    order = stepper.order
    check("length", len(order) == 3, len(order))
    check("order[0] == ev-create",   order[0] == "ev-create",   order[0] if order else None)
    check("order[1] == ev-exercise", order[1] == "ev-exercise", order[1] if len(order) > 1 else None)
    check("order[2] == ev-child",    order[2] == "ev-child",    order[2] if len(order) > 2 else None)

    # ── show_tree ──────────────────────────────────────────────────────────
    section("show_tree")

    stepper.index = 0
    lines0 = capture(stepper.show_tree).strip().splitlines()
    check("3 lines rendered",                len(lines0) == 3, len(lines0))
    check("cursor on ev-create (idx 0)",     "=>" in lines0[0] and "ev-create"   in lines0[0], lines0[0] if lines0 else None)
    check("no cursor on ev-exercise",        "=>" not in lines0[1] and "ev-exercise" in lines0[1], lines0[1] if len(lines0) > 1 else None)
    check("no cursor on ev-child",           "=>" not in lines0[2] and "ev-child"    in lines0[2], lines0[2] if len(lines0) > 2 else None)

    stepper.index = 2  # ev-child
    lines2 = capture(stepper.show_tree).strip().splitlines()
    check("cursor on ev-child (idx 2)",      "=>" in lines2[2] and "ev-child"    in lines2[2], lines2[2] if len(lines2) > 2 else None)
    check("no cursor on ev-exercise (idx 2)","=>" not in lines2[1],                             lines2[1] if len(lines2) > 1 else None)

    # ── Breakpoint.matches ─────────────────────────────────────────────────
    section("Breakpoint.matches")
    ev_ex = trace.events_by_id["ev-exercise"]
    loc = SourceLocation(path="/daml/Token.daml", line=42, label="Transfer")

    # id and step
    check("exact event_id",          Breakpoint("ev-exercise").matches(1, "ev-exercise", ev_ex, None))
    check("#event_id prefix",         Breakpoint("#ev-exercise").matches(1, "ev-exercise", ev_ex, None))
    check("step number (2)",          Breakpoint("2").matches(1, "ev-exercise", ev_ex, None))
    check("wrong step (99) → False",  not Breakpoint("99").matches(1, "ev-exercise", ev_ex, None))
    check("wrong id → False",         not Breakpoint("ev-create").matches(1, "ev-exercise", ev_ex, None))

    # event_target substring (template.choice)
    check("choice name 'Transfer'",   Breakpoint("Transfer").matches(1, "ev-exercise", ev_ex, None))
    check("template 'Token:Token'",   Breakpoint("Token:Token").matches(1, "ev-exercise", ev_ex, None))
    check("unrelated 'Asset' → F",  not Breakpoint("Asset").matches(1, "ev-exercise", ev_ex, None))

    # loc-dependent
    check("loc label 'transfer'",     Breakpoint("transfer").matches(1, "ev-exercise", ev_ex, loc))
    check("file:line match",          Breakpoint("Token.daml:42").matches(1, "ev-exercise", ev_ex, loc))
    check("file:line wrong line → F", not Breakpoint("Token.daml:99").matches(1, "ev-exercise", ev_ex, loc))
    check("path suffix",              Breakpoint("Token.daml").matches(1, "ev-exercise", ev_ex, loc))
    check("wrong path → False",       not Breakpoint("Other.daml").matches(1, "ev-exercise", ev_ex, loc))

    # ── step_variables ────────────────────────────────────────────────────
    section("step_variables")
    ctx = RenderContext(trace)

    ev_cr = trace.events_by_id["ev-create"]
    vc = stepper.step_variables(ev_cr, ctx)
    check("create: eventId present",       "eventId"       in vc)
    check("create: kind present",          "kind"          in vc)
    check("create: template present",      "template"      in vc)
    check("create: contractId present",    "contractId"    in vc)
    check("create: signatories present",   "signatories"   in vc)
    check("create: createPayload present", "createPayload" in vc)
    check("create: no choiceArgument",     "choiceArgument" not in vc)
    check("create: no actors",             "actors" not in vc)

    ve = stepper.step_variables(ev_ex, ctx)
    check("exercise: eventId present",     "eventId"        in ve)
    check("exercise: choice present",      "choice"         in ve)
    check("exercise: actors present",      "actors"         in ve)
    check("exercise: choiceArgument",      "choiceArgument" in ve)
    check("exercise: no createPayload",    "createPayload"  not in ve)
    check("exercise: no signatories",      "signatories"    not in ve)

    # ── result ────────────────────────────────────────────────────────────────
    print()
    if _errors:
        print("dpm trace stepper checks FAILED:")
        for err in _errors:
            print(f"  - {err}")
        return 1
    print("dpm trace stepper checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
