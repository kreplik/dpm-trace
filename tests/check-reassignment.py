import io
import json
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


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-reassignment.py <repo-root>", file=sys.stderr)
        return 2

    repo_root = Path(sys.argv[1]).resolve()
    sys.path.insert(0, str(repo_root / "src"))
    fixtures = repo_root / "tests" / "fixtures" / "reassignment"

    from dpm_trace.cli import (
        Color,
        event_kind_label,
        normalize_trace,
        print_pretty_trace,
        state_diff_counts,
        trace_from_json,
        trace_to_json,
    )

    alice = "Alice::aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa000001"
    bob = "Bob::bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb000002"

    def load(name: str, parties: list[str]):
        raw = json.loads((fixtures / name).read_text(encoding="utf-8"))
        return normalize_trace(raw, "ledger-json-api", None, parties)

    # ── unassign (source synchronizer update) ─────────────────────────────────
    section("unassign normalization")
    unassign = load("unassign-update.json", [alice])
    check("update id", unassign.update_id == "update-reassign-unassign-0001", unassign.update_id)
    check("one event", len(unassign.events_by_id) == 1, len(unassign.events_by_id))
    ev = list(unassign.events_by_id.values())[0]
    check("kind is unassign", ev.kind == "unassign", ev.kind)
    check("node id 0 kept", ev.event_id == "0", ev.event_id)
    check("template", ev.template == "pkg1aabb:Asset:Asset", ev.template)
    check("contract id", (ev.contract_id or "").startswith("00aabbcc"), ev.contract_id)
    check("source synchronizer", (ev.source_synchronizer or "").startswith("sync-a::"), ev.source_synchronizer)
    check("target synchronizer", (ev.target_synchronizer or "").startswith("sync-b::"), ev.target_synchronizer)
    check("reassignment id", ev.reassignment_id == "reassign-0001", ev.reassignment_id)
    check("counter is int", ev.reassignment_counter == 1, ev.reassignment_counter)
    check("submitter", ev.submitter == alice, ev.submitter)
    check("witnesses", ev.witnesses == [alice], ev.witnesses)
    check("root event inferred", unassign.root_event_ids == ["0"], unassign.root_event_ids)

    # ── assign (target synchronizer update) ───────────────────────────────────
    section("assign normalization")
    assign = load("assign-update.json", [bob])
    check("update id", assign.update_id == "update-reassign-assign-0002", assign.update_id)
    ev = list(assign.events_by_id.values())[0]
    check("kind is assign", ev.kind == "assign", ev.kind)
    check("template from nested created", ev.template == "pkg1aabb:Asset:Asset", ev.template)
    check("contract from nested created", (ev.contract_id or "").startswith("00aabbcc"), ev.contract_id)
    check("payload from nested created", isinstance(ev.payload, dict) and ev.payload.get("name") == "GOLD", ev.payload)
    check("signatories from nested created", ev.signatories == [alice], ev.signatories)
    check("observers from nested created", ev.observers == [bob], ev.observers)
    check("source synchronizer", (ev.source_synchronizer or "").startswith("sync-a::"), ev.source_synchronizer)
    check("target synchronizer", (ev.target_synchronizer or "").startswith("sync-b::"), ev.target_synchronizer)
    check("counter", ev.reassignment_counter == 1, ev.reassignment_counter)

    # ── single-event reassignment shape ───────────────────────────────────────
    section("single-event shape")
    raw = json.loads((fixtures / "assign-update.json").read_text(encoding="utf-8"))
    single_raw = {"reassignment": dict(raw["reassignment"])}
    single_raw["reassignment"]["event"] = single_raw["reassignment"].pop("events")[0]
    single = normalize_trace(single_raw, "ledger-json-api", None, [bob])
    check("one event", len(single.events_by_id) == 1, len(single.events_by_id))
    check("kind is assign", list(single.events_by_id.values())[0].kind == "assign")

    # ── real Canton captures ──────────────────────────────────────────────────
    # Recorded from Canton 3.5 with two synchronizers and an explicit
    # unassign/assign. The wrapper keys and envelope differ from the synthetic
    # fixtures above, which is why both shapes are covered: an earlier version
    # of this parser handled only the synthetic names and produced untyped
    # events against a real ledger while every test still passed.
    section("real Canton capture: unassign")
    real_un = load("real-unassign-update.json", [])
    run = list(real_un.events_by_id.values())[0]
    check("kind is unassign", run.kind == "unassign", run.kind)
    check("template", (run.template or "").endswith(":Iou:Iou"), run.template)
    check("contract id", bool(run.contract_id), run.contract_id)
    check("source is sync-a", (run.source_synchronizer or "").startswith("sync-a::"), run.source_synchronizer)
    check("target is sync-b", (run.target_synchronizer or "").startswith("sync-b::"), run.target_synchronizer)
    check("counter", run.reassignment_counter == 1, run.reassignment_counter)
    check("submitter", (run.submitter or "").startswith("Alice::"), run.submitter)
    check("reassignment id", bool(run.reassignment_id), run.reassignment_id)
    check("top-level synchronizer is source",
          (real_un.synchronizer_id or "").startswith("sync-a::"), real_un.synchronizer_id)

    section("real Canton capture: assign")
    real_as = load("real-assign-update.json", [])
    ras = list(real_as.events_by_id.values())[0]
    check("kind is assign", ras.kind == "assign", ras.kind)
    check("template from nested created", (ras.template or "").endswith(":Iou:Iou"), ras.template)
    check("payload from nested created", isinstance(ras.payload, dict) and "amount" in ras.payload, ras.payload)
    check("signatories from nested created", len(ras.signatories) == 1, ras.signatories)
    check("observers from nested created", len(ras.observers) == 1, ras.observers)
    check("counter", ras.reassignment_counter == 1, ras.reassignment_counter)
    check("top-level synchronizer is target",
          (real_as.synchronizer_id or "").startswith("sync-b::"), real_as.synchronizer_id)

    # ── labels and counts ─────────────────────────────────────────────────────
    section("labels and counts")
    check("assign label", event_kind_label("assign") == "ASSIGN", event_kind_label("assign"))
    check("unassign label", event_kind_label("unassign") == "UNASSIGN", event_kind_label("unassign"))
    counts = state_diff_counts(assign)
    check("assign counted, not other", counts["assign"] == 1 and counts["other"] == 0, counts)
    counts = state_diff_counts(unassign)
    check("unassign counted, not other", counts["unassign"] == 1 and counts["other"] == 0, counts)

    # ── rendering ─────────────────────────────────────────────────────────────
    section("rendering")
    color = Color(False)
    text = capture(lambda: print_pretty_trace(unassign, color=color))
    check("UNASSIGN marker", "UNASSIGN Asset:Asset" in text)
    check("summary counts unassign", "1 unassign" in text)
    check("synchronizer arrow", "reassignment:" in text and "sync-a::" in text and "->" in text)
    check("counter line", "counter: 1" in text)
    check("submitter line", "submitter:" in text)
    print()
    print(text)

    text = capture(lambda: print_pretty_trace(assign, color=color))
    check("ASSIGN marker", "ASSIGN Asset:Asset" in text)
    check("summary counts assign", "1 assign" in text)
    check("payload rendered", "GOLD" in text)
    print()
    print(text)

    # ── artifact round-trip ───────────────────────────────────────────────────
    section("artifact round-trip")
    restored = trace_from_json(json.loads(json.dumps(trace_to_json(assign))))
    rev = list(restored.events_by_id.values())[0]
    check("kind survives", rev.kind == "assign", rev.kind)
    check("source survives", rev.source_synchronizer == ev.source_synchronizer, rev.source_synchronizer)
    check("target survives", rev.target_synchronizer == ev.target_synchronizer, rev.target_synchronizer)
    check("counter survives", rev.reassignment_counter == 1, rev.reassignment_counter)
    check("reassignment id survives", rev.reassignment_id == "reassign-0001", rev.reassignment_id)
    check("submitter survives", rev.submitter == alice, rev.submitter)

    print()
    if _errors:
        print("dpm trace reassignment checks FAILED:")
        for err in _errors:
            print(f"  - {err}")
        return 1
    print("dpm trace reassignment checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
