"""Dump normalized traces as canonical JSON, for differential testing.

Both implementations run their own normalizer over the same fixtures and print
this exact structure; the Go side compares the two byte for byte
(internal/model/differential_test.go). Emitting JSON rather than formatted text
is deliberate -- an earlier ad-hoc comparison reported a difference that was
only Python's repr quoting list elements.

    python tests/dump-model.py <fixture.json> [...]

Delete this once the Python implementation is retired.
"""

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "src"))

from dpm_trace.cli import normalize_trace  # noqa: E402


def dump(path: Path) -> dict:
    raw = json.loads(path.read_text(encoding="utf-8"))
    # Trace artifacts nest the update under "trace"; raw API responses do not.
    if isinstance(raw.get("trace"), dict):
        raw = raw["trace"]
    trace = normalize_trace(raw, "ledger-json-api", None, [])
    return {
        "fixture": path.name,
        "updateId": trace.update_id,
        "offset": trace.offset,
        "recordTime": trace.record_time,
        "synchronizerId": trace.synchronizer_id,
        "rootEventIds": trace.root_event_ids,
        "events": [
            {
                "eventId": ev.event_id,
                "kind": ev.kind,
                "template": ev.template,
                "contractId": ev.contract_id,
                "choice": ev.choice,
                "consuming": ev.consuming,
                "actingParties": ev.acting_parties,
                "witnesses": ev.witnesses,
                "signatories": ev.signatories,
                "observers": ev.observers,
                "childEventIds": ev.child_event_ids,
                "payload": ev.payload,
                "argument": ev.argument,
                "result": ev.result,
                "sourceSynchronizer": ev.source_synchronizer,
                "targetSynchronizer": ev.target_synchronizer,
                "reassignmentId": ev.reassignment_id,
                "reassignmentCounter": ev.reassignment_counter,
                "submitter": ev.submitter,
            }
            for _, ev in sorted(trace.events_by_id.items())
        ],
    }


def main() -> int:
    paths = [Path(arg) for arg in sys.argv[1:]]
    if not paths:
        print("usage: dump-model.py <fixture.json> [...]", file=sys.stderr)
        return 2
    print(json.dumps([dump(path) for path in paths], indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
