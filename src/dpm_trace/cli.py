from __future__ import annotations

import argparse
import json
import os
import re
import sys
import textwrap
import urllib.error
import urllib.request
from copy import deepcopy
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from uuid import uuid4


SCAN_UPDATE_PATH = "/v2/updates/{update_id}"
LEDGER_UPDATE_BY_ID_PATH = "/v2/updates/update-by-id"
LEDGER_ACTIVE_CONTRACTS_PATH = "/v2/state/active-contracts"
LEDGER_INTERACTIVE_PREPARE_PATH = "/v2/interactive-submission/prepare"
BUNDLE_SCHEMA = "dpm-trace/replay-bundle/v0"


@dataclass
class TraceEvent:
    event_id: str
    kind: str
    template: str | None = None
    contract_id: str | None = None
    choice: str | None = None
    consuming: bool | None = None
    acting_parties: list[str] = field(default_factory=list)
    witnesses: list[str] = field(default_factory=list)
    signatories: list[str] = field(default_factory=list)
    observers: list[str] = field(default_factory=list)
    child_event_ids: list[str] = field(default_factory=list)
    payload: Any = None
    argument: Any = None
    result: Any = None
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class NormalizedTrace:
    update_id: str
    source: str
    source_url: str | None
    projection: dict[str, Any]
    root_event_ids: list[str]
    events_by_id: dict[str, TraceEvent]
    record_time: str | None = None
    offset: str | None = None
    synchronizer_id: str | None = None
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class SourceLocation:
    path: str
    line: int
    label: str


@dataclass
class SourceLine:
    path: str
    line: int
    text: str


@dataclass
class ExpressionStep:
    line: SourceLine
    label: str
    expression: str
    variables: dict[str, Any] = field(default_factory=dict)
    result: Any = None
    note: str | None = None


def main(argv: list[str] | None = None) -> int:
    argv = list(sys.argv[1:] if argv is None else argv)
    if argv and argv[0] == "bundle":
        return bundle_main(argv[1:])
    if argv and argv[0] == "replay":
        return replay_main(argv[1:])
    if argv and argv[0] == "simulate":
        return simulate_main(argv[1:])

    parser = build_trace_parser()
    args = parser.parse_args(argv)
    return run_trace(args)


def build_trace_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dpm trace",
        description="POC for participant-scoped Canton transaction tracing and interactive stepping.",
        epilog=(
            "Subcommands: "
            "dpm trace bundle <update-id>, "
            "dpm trace replay <bundle>, "
            "dpm trace simulate <update-id>."
        ),
    )
    parser.add_argument("target", nargs="?", help="Update id or CantonScan update URL.")
    parser.add_argument("--interactive", action="store_true", help="Open the terminal stepper.")
    add_common_connection_args(parser)
    parser.add_argument("--print-json", action="store_true", help="Print normalized trace JSON and exit.")
    parser.add_argument("--explain-apis", action="store_true", help="Explain Scan API vs Ledger API.")
    parser.add_argument("--explain-replay", action="store_true", help="Explain what local replay needs.")
    return parser


def add_common_connection_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--scan-url", help="Scan API base URL, e.g. https://.../api/scan.")
    parser.add_argument("--ledger-url", help="Ledger JSON API base URL, e.g. http://localhost:7575.")
    parser.add_argument("--token-file", help="Bearer token file for Ledger JSON API.")
    parser.add_argument("--token", help="Bearer token for Ledger JSON API.")
    parser.add_argument("--read-as", action="append", default=[], help="Party to read as. Repeatable.")
    parser.add_argument("--party", action="append", default=[], help="Alias for --read-as.")
    parser.add_argument("--dar", action="append", default=[], help="Local DAR to attach as package/debug metadata. Repeatable.")
    parser.add_argument("--debug-info", action="append", default=[], help="Daml debug-info JSON sidecar. Repeatable.")
    parser.add_argument(
        "--config",
        help="Trace config JSON. Defaults to .dpm-trace.json found in the current directory or a parent.",
    )
    parser.add_argument(
        "--color",
        choices=["auto", "always", "never"],
        default="auto",
        help="Colorize pretty trace output. Defaults to auto.",
    )
    parser.add_argument("--source", choices=["auto", "scan", "ledger"], default="auto")


def bundle_main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(
        prog="dpm trace bundle",
        description="Create a participant-scoped replay/debug bundle for a committed update.",
    )
    parser.add_argument("target", help="Update id or CantonScan update URL.")
    parser.add_argument("--out", help="Output bundle path. Defaults to trace-<update-id>.bundle.json.")
    parser.add_argument("--active-at-offset", help="Override ACS snapshot offset.")
    parser.add_argument("--no-acs", action="store_true", help="Do not try to capture an ACS snapshot.")
    parser.add_argument("--include-raw", action="store_true", help="Include the raw update response in the bundle.")
    add_common_connection_args(parser)
    args = parser.parse_args(argv)
    return run_bundle(args)


def replay_main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(
        prog="dpm trace replay",
        description="Open a replay/debug bundle.",
    )
    parser.add_argument("bundle", help="Path to a dpm trace replay bundle.")
    parser.add_argument("--interactive", action="store_true", help="Step through the bundled trace.")
    parser.add_argument("--debug-info", action="append", default=[], help="Daml debug-info JSON sidecar. Repeatable.")
    parser.add_argument("--print-json", action="store_true", help="Print the bundle JSON and exit.")
    parser.add_argument("--color", choices=["auto", "always", "never"], default="auto")
    args = parser.parse_args(argv)
    return run_replay(args)


def simulate_main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(
        prog="dpm trace simulate",
        description="Re-simulate a committed update, or prepare an explicit command, without committing.",
    )
    parser.add_argument("target", nargs="?", help="Committed update id or CantonScan update URL.")
    add_common_connection_args(parser)
    parser.add_argument("--active-at-offset", help="Override ACS snapshot offset when target is an update id.")
    parser.add_argument("--no-acs", action="store_true", help="Do not capture/attach an ACS snapshot when target is an update id.")
    parser.add_argument("--act-as", action="append", default=[], help="Submitting party for simulation. Repeatable.")
    parser.add_argument("--template", help="Template id for an explicit command.")
    parser.add_argument("--choice", help="Choice name for an explicit exercise command.")
    parser.add_argument("--contract-id", help="Contract id for an explicit exercise command.")
    parser.add_argument("--args-json", help="JSON object/value to use as create arguments or choice argument.")
    parser.add_argument("--args-file", help="File containing JSON arguments.")
    parser.add_argument("--arg", action="append", default=[], help="Set one argument field, e.g. --arg count=1. Repeatable.")
    parser.add_argument(
        "--override",
        action="append",
        default=[],
        help="Override a reconstructed command argument field, e.g. --override amount=1000 or --override choiceArgument.amount=1000. Repeatable.",
    )
    parser.add_argument("--command-json", help="Raw JSON command envelope or commands array for PrepareSubmission.")
    parser.add_argument("--command-id", help="Command id. Defaults to dpm-trace-simulate-<uuid>.")
    parser.add_argument(
        "--no-disclosed-contracts",
        action="store_true",
        help="Do not attach disclosed contracts extracted from replay-context ACS snapshots.",
    )
    parser.add_argument(
        "--user-id",
        help="Ledger API user id for PrepareSubmission. Defaults to participant_admin when no bearer token is supplied.",
    )
    parser.add_argument("--out", help="Write the raw PrepareSubmission response to this path.")
    parser.add_argument("--print-json", action="store_true", help="Print the raw PrepareSubmission request and response.")
    explicit_ledger_url = has_cli_option(argv, "--ledger-url")
    explicit_read_as = has_cli_option(argv, "--read-as", "--party")
    args = parser.parse_args(argv)
    args._explicit_ledger_url = explicit_ledger_url
    args._explicit_read_as = explicit_read_as
    try:
        return run_simulate(args)
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


def run_trace(args: argparse.Namespace) -> int:
    try:
        apply_config_defaults(args, load_config(args.config))
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if args.explain_apis:
        print(explain_apis())
        if not args.target:
            return 0

    if args.explain_replay:
        print(explain_replay())
        if not args.target:
            return 0

    try:
        bundle = maybe_load_bundle_target(args.target)
        if bundle is not None:
            trace = trace_from_json(bundle["trace"])
        else:
            update_id = extract_update_id(args.target)
            parties = parse_parties(args.read_as + args.party)
            raw, source, source_url = load_update(args, update_id, parties)
            trace = normalize_trace(raw, source=source, source_url=source_url, parties=parties)
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if args.print_json:
        print(json.dumps(trace_to_json(trace), indent=2, sort_keys=True))
        return 0

    if args.interactive:
        Stepper(
            trace,
            bundle=bundle if bundle is not None else None,
            source_index=source_index_from_args(args, bundle),
            color=Color.from_mode(args.color),
        ).run()
        return 0

    print_pretty_trace(trace, color=Color.from_mode(args.color), source_index=source_index_from_args(args, bundle))
    return 0


def run_bundle(args: argparse.Namespace) -> int:
    try:
        apply_config_defaults(args, load_config(args.config))
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    update_id = extract_update_id(args.target)
    parties = parse_parties(args.read_as + args.party)
    try:
        raw, source, source_url = load_update(args, update_id, parties)
        trace = normalize_trace(raw, source=source, source_url=source_url, parties=parties)
        bundle = create_bundle(args, trace, raw if args.include_raw else None)
        out_path = Path(args.out) if args.out else default_bundle_path(trace.update_id)
        out_path.write_text(json.dumps(bundle, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(f"wrote replay bundle: {out_path}")
    print(bundle_summary(bundle))
    return 0


def run_replay(args: argparse.Namespace) -> int:
    try:
        apply_config_defaults(args, load_config(None))
        bundle = load_bundle(Path(args.bundle))
        trace = trace_from_json(bundle["trace"])
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if args.print_json:
        print(json.dumps(bundle, indent=2, sort_keys=True))
        return 0

    print(bundle_summary(bundle))
    if args.interactive:
        Stepper(trace, bundle=bundle, source_index=source_index_from_args(args, bundle), color=Color.from_mode(args.color)).run()
        return 0
    print_pretty_trace(trace, color=Color.from_mode(args.color), source_index=source_index_from_args(args, bundle))
    return 0


def run_simulate(args: argparse.Namespace) -> int:
    apply_config_defaults(args, load_config(args.config))
    bundle = load_or_create_simulation_bundle(args)
    apply_bundle_defaults(args, bundle)

    if bundle is None and not args.command_json and not args.template:
        raise ValueError("simulation needs an update id, --command-json, or --template")

    request = prepare_submission_request(args, bundle)
    ledger_url = simulation_ledger_url(args, bundle)
    token = args.token or read_token_file(args.token_file)
    url = join_url(ledger_url, LEDGER_INTERACTIVE_PREPARE_PATH)
    response = http_json("POST", url, body=request, token=token)

    if args.out:
        out_path = Path(args.out)
        out_path.write_text(json.dumps(response, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(f"wrote simulation response: {out_path}")

    if args.print_json:
        print(json.dumps({"request": request, "response": response}, indent=2, sort_keys=True))
        return 0

    print(simulation_success_report(request, response, url, bundle))
    return 0


def load_or_create_simulation_bundle(args: argparse.Namespace) -> dict[str, Any] | None:
    target = getattr(args, "target", None)
    if not target:
        return None

    existing = Path(target)
    if existing.exists() and existing.is_file():
        raise ValueError(
            "dpm trace simulate no longer accepts replay bundle paths. "
            "Use `dpm trace replay <bundle>` to inspect a saved bundle, or `dpm trace simulate <update-id>` to re-simulate a committed update."
        )

    update_id = extract_update_id(target)
    parties = parse_parties(args.read_as + args.party)
    update_args = args
    if getattr(args, "source", "auto") == "auto" and getattr(args, "ledger_url", None):
        update_args = argparse.Namespace(**vars(args))
        update_args.source = "ledger"
    raw, source, source_url = load_update(update_args, update_id, parties)
    trace = normalize_trace(raw, source=source, source_url=source_url, parties=parties)
    return create_bundle(update_args, trace, raw_update=None)


def apply_bundle_defaults(args: argparse.Namespace, bundle: dict[str, Any] | None) -> None:
    if bundle is None:
        return
    participant = bundle.get("participant") or {}
    if not getattr(args, "_explicit_ledger_url", False) and participant.get("ledgerUrl"):
        args.ledger_url = participant["ledgerUrl"]
    if not getattr(args, "_explicit_read_as", False) and participant.get("readAs"):
        args.read_as = []
        args.party = []


def prepare_submission_request(args: argparse.Namespace, bundle: dict[str, Any] | None) -> dict[str, Any]:
    command_id = args.command_id or f"dpm-trace-simulate-{uuid4().hex[:12]}"
    commands = apply_simulation_overrides(simulation_commands(args, bundle), args.override)
    act_as = simulation_act_as(args, bundle)
    read_as = [party for party in simulation_read_as(args, bundle) if party not in act_as]
    package_ids = simulation_package_ids(bundle)
    synchronizer_id = simulation_synchronizer_id(bundle)
    disclosed_contracts = [] if args.no_disclosed_contracts else disclosed_contracts_from_bundle(bundle)

    request: dict[str, Any] = {
        "commandId": command_id,
        "commands": commands,
        "actAs": act_as,
        "readAs": read_as,
        "disclosedContracts": disclosed_contracts,
        "synchronizerId": synchronizer_id or "",
        "packageIdSelectionPreference": package_ids,
        "verboseHashing": True,
    }
    user_id = simulation_user_id(args)
    if user_id:
        request["userId"] = user_id
    return request


def simulation_commands(args: argparse.Namespace, bundle: dict[str, Any] | None) -> list[dict[str, Any]]:
    if args.command_json:
        raw = parse_json_text(args.command_json, "--command-json")
        if isinstance(raw, dict) and isinstance(raw.get("commands"), list):
            return raw["commands"]
        if isinstance(raw, list):
            return raw
        if isinstance(raw, dict):
            return [raw]
        raise ValueError("--command-json must be a JSON object or array")

    if args.template:
        return [explicit_js_command(args)]

    command_context = (bundle or {}).get("command") or {}
    if not command_context.get("available"):
        reason = command_context.get("reason") or "bundle has no inferred command"
        raise ValueError(
            f"{reason}. Provide an explicit command with --template/--args-json or --command-json."
        )
    commands = command_context.get("commands")
    if not isinstance(commands, list) or not commands:
        raise ValueError("bundle command context is malformed: commands must be a non-empty array")
    return [bundle_command_to_js_command(command) for command in commands if isinstance(command, dict)]


def explicit_js_command(args: argparse.Namespace) -> dict[str, Any]:
    arguments = simulation_arguments(args)
    if args.contract_id or args.choice:
        if not args.contract_id:
            raise ValueError("--contract-id is required for an exercise simulation")
        if not args.choice:
            raise ValueError("--choice is required for an exercise simulation")
        return {
            "ExerciseCommand": {
                "templateId": args.template,
                "contractId": args.contract_id,
                "choice": args.choice,
                "choiceArgument": arguments,
            }
        }
    return {
        "CreateCommand": {
            "templateId": args.template,
            "createArguments": arguments,
        }
    }


def bundle_command_to_js_command(command: dict[str, Any]) -> dict[str, Any]:
    kind = command.get("kind")
    if kind == "create":
        template = command.get("template")
        if not template:
            raise ValueError("bundle create command is missing template")
        return {
            "CreateCommand": {
                "templateId": template,
                "createArguments": command.get("createArguments") or {},
            }
        }
    if kind == "exercise":
        template = command.get("template")
        contract_id = command.get("contractId")
        choice = command.get("choice")
        if not template or not contract_id or not choice:
            raise ValueError("bundle exercise command is missing template, contractId, or choice")
        return {
            "ExerciseCommand": {
                "templateId": template,
                "contractId": contract_id,
                "choice": choice,
                "choiceArgument": command.get("choiceArgument") or {},
            }
        }
    raise ValueError(f"unsupported bundle command kind for simulation: {kind!r}")


def apply_simulation_overrides(commands: list[dict[str, Any]], overrides: list[str]) -> list[dict[str, Any]]:
    if not overrides:
        return commands
    if len(commands) != 1:
        raise ValueError("--override currently supports exactly one reconstructed command")

    result = deepcopy(commands)
    command = result[0]
    for raw in overrides:
        path, value = parse_path_assignment(raw, "--override")
        apply_override_to_command(command, path, value)
    return result


def apply_override_to_command(command: dict[str, Any], path: list[str], value: Any) -> None:
    command_kind: str
    body: dict[str, Any]
    argument_key: str
    if isinstance(command.get("CreateCommand"), dict):
        command_kind = "create"
        body = command["CreateCommand"]
        argument_key = "createArguments"
    elif isinstance(command.get("ExerciseCommand"), dict):
        command_kind = "exercise"
        body = command["ExerciseCommand"]
        argument_key = "choiceArgument"
    else:
        raise ValueError("--override only supports CreateCommand and ExerciseCommand")

    if not path:
        raise ValueError("--override path cannot be empty")

    explicit_target = path[0]
    if explicit_target in ("createArgument", "createArguments"):
        if command_kind != "create":
            raise ValueError("--override createArguments.* can only be used with create commands")
        path = path[1:]
    elif explicit_target in ("choiceArgument", "choiceArguments"):
        if command_kind != "exercise":
            raise ValueError("--override choiceArgument.* can only be used with exercise commands")
        path = path[1:]

    if not path:
        body[argument_key] = value
        return

    arguments = body.get(argument_key)
    if arguments in (None, {}):
        arguments = {}
        body[argument_key] = arguments
    if not isinstance(arguments, dict):
        raise ValueError(f"--override needs {argument_key} to be a JSON object")
    set_json_path(arguments, path, value)


def parse_path_assignment(raw: str, option_name: str) -> tuple[list[str], Any]:
    if "=" not in raw:
        raise ValueError(f"{option_name} must use key=value syntax: {raw!r}")
    key, value = raw.split("=", 1)
    path = [part.strip() for part in key.strip().split(".") if part.strip()]
    if not path:
        raise ValueError(f"{option_name} key cannot be empty: {raw!r}")
    return path, parse_scalar(value.strip())


def set_json_path(target: dict[str, Any], path: list[str], value: Any) -> None:
    current = target
    for key in path[:-1]:
        existing = current.get(key)
        if existing is None:
            existing = {}
            current[key] = existing
        if not isinstance(existing, dict):
            raise ValueError(f"--override cannot descend into non-object field {key!r}")
        current = existing
    current[path[-1]] = value


def disclosed_contracts_from_bundle(bundle: dict[str, Any] | None) -> list[dict[str, Any]]:
    if bundle is None:
        return []
    acs = bundle.get("acsSnapshot") or {}
    if not acs.get("available"):
        return []
    synchronizer_id = simulation_synchronizer_id(bundle)
    contracts: dict[str, dict[str, Any]] = {}
    for created_event, local_synchronizer_id in iter_created_events(acs.get("response")):
        contract_id = pick(created_event, "contractId", "contract_id")
        blob = pick(created_event, "createdEventBlob", "created_event_blob")
        template_id = pick(created_event, "templateId", "template_id")
        if not contract_id or not blob or not template_id:
            continue
        contracts[str(contract_id)] = {
            "templateId": template_id,
            "contractId": contract_id,
            "createdEventBlob": blob,
            "synchronizerId": local_synchronizer_id or synchronizer_id or "",
        }
    return list(contracts.values())


def iter_created_events(value: Any, synchronizer_id: str | None = None):
    if isinstance(value, list):
        for item in value:
            yield from iter_created_events(item, synchronizer_id)
        return
    if not isinstance(value, dict):
        return

    local_synchronizer_id = pick(value, "synchronizerId", "synchronizer_id") or synchronizer_id
    created = pick(value, "createdEvent", "created_event", "created", "CreatedEvent", "createdEventValue")
    if isinstance(created, dict):
        yield created, local_synchronizer_id

    for child in value.values():
        if isinstance(child, (dict, list)):
            yield from iter_created_events(child, local_synchronizer_id)


def simulation_arguments(args: argparse.Namespace) -> Any:
    if args.args_json and args.args_file:
        raise ValueError("use only one of --args-json or --args-file")
    if args.args_json:
        base = parse_json_text(args.args_json, "--args-json")
    elif args.args_file:
        path = Path(args.args_file)
        base = parse_json_text(path.read_text(encoding="utf-8"), str(path))
    else:
        base = {}

    assignments = parse_arg_assignments(args.arg)
    if assignments:
        if base in (None, {}):
            base = {}
        if not isinstance(base, dict):
            raise ValueError("--arg can only be used when arguments are a JSON object")
        base.update(assignments)
    return base


def parse_arg_assignments(values: list[str]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for raw in values:
        if "=" not in raw:
            raise ValueError(f"--arg must use key=value syntax: {raw!r}")
        key, value = raw.split("=", 1)
        key = key.strip()
        if not key:
            raise ValueError(f"--arg key cannot be empty: {raw!r}")
        result[key] = parse_scalar(value.strip())
    return result


def parse_scalar(value: str) -> Any:
    if value == "null":
        return None
    if value == "true":
        return True
    if value == "false":
        return False
    if re.fullmatch(r"-?[0-9]+", value):
        try:
            return int(value)
        except ValueError:
            pass
    if re.fullmatch(r"-?[0-9]+\.[0-9]+", value):
        try:
            return float(value)
        except ValueError:
            pass
    if value.startswith(("{", "[")):
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            pass
    return value


def parse_json_text(value: str, source: str) -> Any:
    try:
        return json.loads(value)
    except json.JSONDecodeError as exc:
        raise ValueError(f"invalid JSON in {source}: {exc}") from exc


def simulation_ledger_url(args: argparse.Namespace, bundle: dict[str, Any] | None) -> str:
    ledger_url = args.ledger_url or ((bundle or {}).get("participant") or {}).get("ledgerUrl")
    if not ledger_url:
        raise ValueError("--ledger-url is required for engine-backed simulation")
    return str(ledger_url)


def simulation_user_id(args: argparse.Namespace) -> str | None:
    if args.user_id:
        return args.user_id
    if not args.token and not args.token_file:
        return "participant_admin"
    return None


def simulation_act_as(args: argparse.Namespace, bundle: dict[str, Any] | None) -> list[str]:
    explicit = parse_parties(args.act_as)
    if explicit:
        return explicit

    command_context = (bundle or {}).get("command") or {}
    parties: list[str] = []
    commands = command_context.get("commands")
    if isinstance(commands, list):
        for command in commands:
            if isinstance(command, dict):
                parties.extend(list_str(command.get("actAs") or []))
    if parties:
        return unique(parties)

    read_as = simulation_read_as(args, bundle)
    if read_as:
        return [read_as[0]]
    raise ValueError("--act-as is required for simulation")


def simulation_read_as(args: argparse.Namespace, bundle: dict[str, Any] | None) -> list[str]:
    explicit = parse_parties(args.read_as + args.party)
    if explicit:
        return explicit
    participant = (bundle or {}).get("participant") or {}
    return list_str(participant.get("readAs") or [])


def simulation_package_ids(bundle: dict[str, Any] | None) -> list[str]:
    packages = (bundle or {}).get("packages") or {}
    return list_str(packages.get("packageIds") or [])


def simulation_synchronizer_id(bundle: dict[str, Any] | None) -> str | None:
    time_context = (bundle or {}).get("time") or {}
    value = time_context.get("synchronizerId")
    return str(value) if value else None


def unique(values: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for value in values:
        if value not in seen:
            seen.add(value)
            result.append(value)
    return result


def simulation_success_report(
    request: dict[str, Any],
    response: Any,
    source_url: str,
    bundle: dict[str, Any] | None,
) -> str:
    source_update = ((bundle or {}).get("trace") or {}).get("updateId")
    lines = [
        "Historical transaction re-simulation prepared" if source_update else "Command preparation completed",
        "  source:       Ledger JSON API InteractiveSubmissionService.PrepareSubmission",
        f"  endpoint:     {source_url}",
        "  committed:    no",
        f"  command id:   {request.get('commandId')}",
        f"  act-as:       {', '.join(request.get('actAs') or [])}",
        f"  read-as:      {', '.join(request.get('readAs') or []) or '-'}",
        f"  commands:     {len(request.get('commands') or [])}",
        f"  disclosures:  {len(request.get('disclosedContracts') or [])}",
    ]
    if source_update:
        lines.append(f"  from update:  {source_update}")
    if isinstance(response, dict):
        prepared = response.get("preparedTransaction")
        tx_hash = response.get("preparedTransactionHash")
        hashing = response.get("hashingSchemeVersion")
        if tx_hash:
            lines.append(f"  tx hash:      {short(str(tx_hash), 80)}")
        if hashing:
            lines.append(f"  hashing:      {hashing}")
        if isinstance(prepared, str):
            lines.append(f"  prepared tx:  {len(prepared)} bytes/base64 chars returned")
        elif prepared is not None:
            lines.append("  prepared tx:  structured payload returned")
        if response.get("costEstimation") is not None:
            lines.append("  cost:         returned")
    lines.append("")
    lines.append("This is a non-committing participant prepare call, not an event-tree replay.")
    if source_update:
        lines.append("The command was reconstructed from the committed update and replay context available to this participant projection.")
    if request.get("disclosedContracts"):
        lines.append("Replay-context ACS contracts were attached as disclosed contracts, so consumed historical inputs can be resolved during preparation.")
    lines.append("Source-level stepping still needs a local LF engine adapter or participant-side debug events.")
    return "\n".join(lines)


def load_config(explicit_path: str | None) -> dict[str, Any]:
    path = find_config(explicit_path)
    if not path:
        return {}
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError:
        if explicit_path:
            raise ValueError(f"config file not found: {explicit_path}") from None
        return {}
    except json.JSONDecodeError as exc:
        raise ValueError(f"invalid JSON config {path}: {exc}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"config must be a JSON object: {path}")
    return data


def find_config(explicit_path: str | None) -> Path | None:
    if explicit_path:
        return Path(explicit_path)
    current = Path.cwd().resolve()
    for directory in (current, *current.parents):
        candidate = directory / ".dpm-trace.json"
        if candidate.exists():
            return candidate
    return None


def apply_config_defaults(args: argparse.Namespace, config: dict[str, Any]) -> None:
    set_default(args, "ledger_url", os.environ.get("DPM_TRACE_LEDGER_URL") or get_config(config, "ledgerUrl", "ledger_url"))
    set_default(args, "scan_url", os.environ.get("DPM_TRACE_SCAN_URL") or get_config(config, "scanUrl", "scan_url"))
    set_default(args, "token_file", os.environ.get("DPM_TRACE_TOKEN_FILE") or get_config(config, "tokenFile", "token_file"))
    set_default(args, "token", os.environ.get("DPM_TRACE_TOKEN") or get_config(config, "token"))

    if hasattr(args, "read_as") and hasattr(args, "party") and not args.read_as and not args.party:
        read_as = os.environ.get("DPM_TRACE_READ_AS") or get_config(config, "readAs", "read_as", "party")
        args.read_as = config_values(read_as)
    if hasattr(args, "dar") and not args.dar:
        dar_paths = os.environ.get("DPM_TRACE_DAR") or get_config(config, "darPaths", "dar_paths", "dar")
        args.dar = config_values(dar_paths)
    if hasattr(args, "debug_info") and not args.debug_info:
        debug_info_paths = os.environ.get("DPM_TRACE_DEBUG_INFO") or get_config(config, "debugInfoPaths", "debug_info_paths", "debugInfo", "debug_info")
        args.debug_info = config_values(debug_info_paths)


def set_default(args: argparse.Namespace, attr: str, value: Any) -> None:
    if not hasattr(args, attr):
        return
    if getattr(args, attr) is None and value not in (None, ""):
        setattr(args, attr, str(value))


def get_config(config: dict[str, Any], *keys: str) -> Any:
    for key in keys:
        if key in config:
            return config[key]
    return None


def config_values(value: Any) -> list[str]:
    if isinstance(value, list):
        return [str(item) for item in value if str(item).strip()]
    if value is None:
        return []
    return [str(value)]


def load_update(
    args: argparse.Namespace,
    update_id: str | None,
    parties: list[str],
) -> tuple[dict[str, Any], str, str | None]:
    if args.source == "scan" or (args.source == "auto" and args.scan_url):
        if not update_id:
            raise ValueError("an update id or CantonScan update URL is required for Scan API")
        if not args.scan_url:
            raise ValueError("--scan-url is required for Scan API fetches")
        url = join_url(args.scan_url, SCAN_UPDATE_PATH.format(update_id=update_id))
        return http_json("GET", url), "scan", url

    if args.source == "ledger" or (args.source == "auto" and args.ledger_url):
        if not update_id:
            raise ValueError("an update id or CantonScan update URL is required for Ledger API")
        if not args.ledger_url:
            raise ValueError("--ledger-url is required for Ledger API fetches")
        if not parties:
            raise ValueError("--read-as/--party is required for participant-scoped Ledger API fetches")
        token = args.token or read_token_file(args.token_file)
        url = join_url(args.ledger_url, LEDGER_UPDATE_BY_ID_PATH)
        body = ledger_update_by_id_body(update_id, parties)
        return http_json("POST", url, body=body, token=token), "ledger-json-api", url

    raise ValueError("choose a source: --scan-url BASE with target, or --ledger-url BASE with target")


def create_bundle(args: argparse.Namespace, trace: NormalizedTrace, raw_update: dict[str, Any] | None) -> dict[str, Any]:
    package_ids = sorted({
        package
        for ev in trace.events_by_id.values()
        for package in [package_from_template(ev.template)]
        if package
    })
    bundle = {
        "schema": BUNDLE_SCHEMA,
        "createdAt": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "mode": "participant-projection",
        "trace": trace_to_json(trace),
        "participant": {
            "ledgerUrl": getattr(args, "ledger_url", None),
            "scanUrl": getattr(args, "scan_url", None),
            "readAs": trace.projection.get("readAs") or [],
        },
        "time": {
            "recordTime": trace.record_time,
            "offset": trace.offset,
            "synchronizerId": trace.synchronizer_id,
            "status": "record time and update offset captured from the committed update",
        },
        "packages": package_metadata_context(getattr(args, "dar", []), getattr(args, "debug_info", []), package_ids),
        "command": infer_command_context(trace),
        "acsSnapshot": None,
        "engine": {
            "available": False,
            "required": "Daml-LF execution/debug adapter",
            "reason": "Trace and ACS data are sufficient for event stepping, not source-level execution stepping.",
        },
        "privacy": {
            "scope": trace.projection.get("note"),
            "readAs": trace.projection.get("readAs") or [],
            "missingPrivateDataPolicy": "private data outside this participant projection is not present in the bundle",
        },
    }
    if raw_update is not None:
        bundle["rawUpdate"] = raw_update

    if getattr(args, "no_acs", False):
        bundle["acsSnapshot"] = unavailable_snapshot("disabled by --no-acs")
    elif trace.source != "ledger-json-api":
        bundle["acsSnapshot"] = unavailable_snapshot("ACS snapshots require a participant Ledger JSON API endpoint")
    else:
        bundle["acsSnapshot"] = fetch_acs_snapshot(args, trace)
    return bundle


def fetch_acs_snapshot(args: argparse.Namespace, trace: NormalizedTrace) -> dict[str, Any]:
    if not args.ledger_url:
        return unavailable_snapshot("--ledger-url is required for ACS snapshot capture")
    active_at_offset = args.active_at_offset or pre_update_offset(trace.offset)
    if active_at_offset is None:
        return unavailable_snapshot(f"could not derive a snapshot offset from update offset {trace.offset!r}")

    url = join_url(args.ledger_url, LEDGER_ACTIVE_CONTRACTS_PATH)
    token = args.token or read_token_file(args.token_file)
    body = active_contracts_body(active_at_offset)
    try:
        response = http_json("POST", url, body=body, token=token)
    except Exception as exc:
        return unavailable_snapshot(str(exc), active_at_offset=active_at_offset, source_url=url)

    return {
        "available": True,
        "sourceUrl": url,
        "activeAtOffset": active_at_offset,
        "request": body,
        "response": response,
        "contractCount": active_contract_count(response),
        "note": "participant-visible ACS snapshot for the authorized read context",
    }


def active_contracts_body(active_at_offset: str | int) -> dict[str, Any]:
    wildcard = {
        "cumulative": [
            {
                "identifierFilter": {
                    "WildcardFilter": {
                        "value": {
                            "includeCreatedEventBlob": True,
                        }
                    }
                }
            }
        ]
    }
    return {
        "verbose": True,
        "eventFormat": None,
        "activeAtOffset": active_at_offset,
        "filter": {
            "filtersByParty": {},
            "filtersForAnyParty": wildcard,
        },
    }


def unavailable_snapshot(reason: str, active_at_offset: str | int | None = None, source_url: str | None = None) -> dict[str, Any]:
    result: dict[str, Any] = {
        "available": False,
        "reason": reason,
    }
    if active_at_offset is not None:
        result["activeAtOffset"] = active_at_offset
    if source_url is not None:
        result["sourceUrl"] = source_url
    return result


def pre_update_offset(offset: str | None) -> int | str | None:
    if not offset:
        return None
    try:
        value = int(offset)
    except ValueError:
        return offset
    return max(value - 1, 0)


def active_contract_count(response: Any) -> int | None:
    if isinstance(response, list):
        return len(response)
    if isinstance(response, dict):
        for key in ("activeContracts", "active_contracts", "createdEvents", "created_events"):
            value = response.get(key)
            if isinstance(value, list):
                return len(value)
    return None


def package_metadata_context(dar_paths: list[str], debug_info_paths: list[str], package_ids: list[str]) -> dict[str, Any]:
    found: list[str] = []
    missing: list[str] = []
    for path in dar_paths:
        resolved = str(Path(path).expanduser())
        if Path(resolved).exists():
            found.append(resolved)
        else:
            missing.append(resolved)
    found_debug_info: list[str] = []
    missing_debug_info: list[str] = []
    for path in debug_info_paths:
        resolved = str(Path(path).expanduser())
        if Path(resolved).exists():
            found_debug_info.append(resolved)
        else:
            missing_debug_info.append(resolved)
    return {
        "available": bool(found or found_debug_info),
        "packageIds": package_ids,
        "darPaths": found,
        "debugInfoPaths": found_debug_info,
        "missingDarPaths": missing,
        "missingDebugInfoPaths": missing_debug_info,
        "status": (
            "local DAR/debug-info metadata attached"
            if found or found_debug_info
            else "package ids captured; DAR/debug-info metadata must be supplied by local project or registry"
        ),
    }


def infer_command_context(trace: NormalizedTrace) -> dict[str, Any]:
    roots = [
        trace.events_by_id[event_id]
        for event_id in trace.root_event_ids
        if event_id in trace.events_by_id
    ]
    if len(roots) != 1:
        return {
            "available": False,
            "reason": "Cannot infer a single replay command from an update with zero or multiple root events. Capture the command at submission time or provide it manually.",
        }

    root = roots[0]
    if root.kind == "exercise" and root.contract_id and root.choice:
        return {
            "available": True,
            "source": "inferred-from-ledger-effects",
            "confidence": "partial",
            "warning": "This is not the original command envelope. Command id, deduplication, disclosed-contract context, and some submission metadata are not recoverable from a committed update.",
            "commands": [
                {
                    "kind": "exercise",
                    "template": root.template,
                    "contractId": root.contract_id,
                    "choice": root.choice,
                    "choiceArgument": simplify_lf_value(root.argument),
                    "actAs": root.acting_parties,
                }
            ],
        }

    if root.kind == "create" and root.template and root.payload is not None:
        act_as = root.signatories or trace.projection.get("readAs") or []
        return {
            "available": True,
            "source": "inferred-from-ledger-effects",
            "confidence": "partial",
            "warning": "This is not the original command envelope. The act-as parties are inferred from signatories/readAs and may need confirmation.",
            "commands": [
                {
                    "kind": "create",
                    "template": root.template,
                    "createArguments": simplify_lf_value(root.payload),
                    "actAs": act_as,
                }
            ],
        }

    return {
        "available": False,
        "reason": f"Cannot infer a replay command from root event kind {root.kind!r}. Capture the command at submission time or provide it manually.",
    }


def default_bundle_path(update_id: str) -> Path:
    safe = re.sub(r"[^A-Za-z0-9_.-]+", "-", update_id)
    return Path(f"trace-{safe[:24]}.bundle.json")


def maybe_load_bundle_target(target: str | None) -> dict[str, Any] | None:
    if not target:
        return None
    path = Path(target)
    if not path.exists() or not path.is_file():
        return None
    return load_bundle(path)


def load_bundle(path: Path) -> dict[str, Any]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"bundle must be a JSON object: {path}")
    if data.get("schema") != BUNDLE_SCHEMA:
        raise ValueError(f"unsupported bundle schema in {path}: {data.get('schema')!r}")
    if not isinstance(data.get("trace"), dict):
        raise ValueError(f"bundle is missing trace object: {path}")
    return data


def bundle_summary(bundle: dict[str, Any]) -> str:
    trace = bundle.get("trace") or {}
    acs = bundle.get("acsSnapshot") or {}
    command = bundle.get("command") or {}
    engine = bundle.get("engine") or {}
    packages = bundle.get("packages") or {}
    lines = [
        "Replay bundle",
        f"  schema:       {bundle.get('schema')}",
        f"  update:       {trace.get('updateId', '-')}",
        f"  mode:         {bundle.get('mode', '-')}",
        f"  ACS:          {snapshot_status(acs)}",
        f"  packages:     {'attached' if packages.get('available') else 'ids only'}",
        f"  command:      {command_status(command)}",
        f"  engine hooks: {'available' if engine.get('available') else 'missing'}",
    ]
    return "\n".join(lines)


def snapshot_status(acs: dict[str, Any]) -> str:
    if acs.get("available"):
        count = acs.get("contractCount")
        suffix = f", {count} active contracts" if count is not None else ""
        return f"captured at offset {acs.get('activeAtOffset')}{suffix}"
    return f"not captured ({acs.get('reason', 'unknown reason')})"


def command_status(command: dict[str, Any]) -> str:
    if command.get("available"):
        source = command.get("source")
        confidence = command.get("confidence")
        suffix = f" ({source}, {confidence})" if source or confidence else ""
        return "available" + suffix
    return "missing"


def simulation_readiness_report(bundle: dict[str, Any]) -> str:
    acs = bundle.get("acsSnapshot") or {}
    command = bundle.get("command") or {}
    engine = bundle.get("engine") or {}
    packages = bundle.get("packages") or {}
    time_context = bundle.get("time") or {}
    missing: list[str] = []
    if not acs.get("available"):
        missing.append("ACS snapshot")
    if not time_context.get("recordTime") and not time_context.get("offset"):
        missing.append("ledger time/offset context")
    if not packages.get("available"):
        missing.append("DAR/package metadata")
    if not command.get("available"):
        missing.append("command envelope")
    if not engine.get("available"):
        missing.append("Daml-LF execution/debug adapter")
    if missing:
        return (
            bundle_summary(bundle)
            + "\n\nSimulation readiness: not executable yet\n"
            + "\n".join(f"- missing {item}" for item in missing)
        )
    return bundle_summary(bundle) + "\n\nSimulation readiness: executable"


def http_json(method: str, url: str, body: dict[str, Any] | None = None, token: str | None = None) -> Any:
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token.strip()}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{method} {url} failed with HTTP {exc.code}: {detail}") from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"{method} {url} failed: {exc.reason}") from exc


def ledger_update_by_id_body(update_id: str, parties: list[str]) -> dict[str, Any]:
    filters_by_party = {
        party: {
            "cumulative": [
                {
                    "identifierFilter": {
                        "WildcardFilter": {
                            "value": {
                                "includeCreatedEventBlob": True,
                            }
                        }
                    }
                }
            ]
        }
        for party in parties
    }
    event_format = {
        "filtersByParty": filters_by_party,
        "verbose": True,
    }
    return {
        "updateId": update_id,
        "updateFormat": {
            "includeTransactions": {
                "eventFormat": event_format,
                "transactionShape": "TRANSACTION_SHAPE_LEDGER_EFFECTS",
            },
            "includeReassignments": event_format,
        },
    }


def normalize_trace(
    raw: dict[str, Any],
    source: str,
    source_url: str | None,
    parties: list[str],
) -> NormalizedTrace:
    tx = unwrap_transaction(raw)
    update_id = str(pick(tx, "update_id", "updateId", "id") or pick(raw, "update_id", "updateId") or "")
    if not update_id:
        raise ValueError("could not find update_id/updateId in response")

    events_raw = pick(tx, "events_by_id", "eventsById", "events") or {}
    events_by_id = normalize_events_map(events_raw)

    root_event_ids = list_str(pick(tx, "root_event_ids", "rootEventIds") or [])
    if not root_event_ids:
        root_event_ids = infer_roots(events_by_id)

    if source == "scan":
        note = "Public Scan projection. Event ids may be Scan-indexed and are not the same as a participant projection."
    else:
        note = "Authorized participant projection. Private data outside these party rights is not available."

    projection = {
        "source": source,
        "participantScoped": source != "scan",
        "readAs": parties,
        "notGlobal": source != "scan",
        "note": note,
    }

    return NormalizedTrace(
        update_id=update_id,
        source=source,
        source_url=source_url,
        projection=projection,
        root_event_ids=root_event_ids,
        events_by_id=events_by_id,
        record_time=pick(tx, "record_time", "recordTime"),
        offset=str(pick(tx, "offset") or "") or None,
        synchronizer_id=pick(tx, "synchronizer_id", "synchronizerId"),
        raw=raw,
    )


def unwrap_transaction(raw: dict[str, Any]) -> dict[str, Any]:
    data = raw.get("data", raw)
    for key in ("transaction", "Transaction", "TransactionTree", "update", "Update"):
        if isinstance(data, dict) and isinstance(data.get(key), dict):
            return unwrap_transaction(data[key])
    if isinstance(data, dict) and isinstance(data.get("value"), dict):
        return unwrap_transaction(data["value"])
    if isinstance(data, dict) and ("events_by_id" in data or "eventsById" in data):
        return data
    if isinstance(data, dict) and "events" in data:
        return data
    return data if isinstance(data, dict) else raw


def normalize_events_map(events_raw: Any) -> dict[str, TraceEvent]:
    result: dict[str, TraceEvent] = {}
    if isinstance(events_raw, dict):
        iterable = events_raw.items()
    elif isinstance(events_raw, list):
        iterable = []
        for i, item in enumerate(events_raw):
            if isinstance(item, dict) and "key" in item and "value" in item:
                iterable.append((str(item["key"]), item["value"]))
            else:
                iterable.append((str(i), item))
    else:
        iterable = []

    for event_id, event_raw in iterable:
        if not isinstance(event_raw, dict):
            continue
        event = normalize_event(str(event_id), event_raw)
        result[event.event_id] = event
    link_range_children(result)
    return result


def normalize_event(event_id: str, event_raw: dict[str, Any]) -> TraceEvent:
    variant = event_raw
    kind = "event"
    for candidate, normalized in (
        ("created", "create"),
        ("CreatedEvent", "create"),
        ("createdEvent", "create"),
        ("exercised", "exercise"),
        ("ExercisedEvent", "exercise"),
        ("exercisedEvent", "exercise"),
        ("archived", "archive"),
        ("ArchivedEvent", "archive"),
        ("archivedEvent", "archive"),
    ):
        if isinstance(event_raw.get(candidate), dict):
            variant = event_raw[candidate]
            kind = normalized
            break
    if kind == "event":
        explicit = str(pick(event_raw, "eventType", "event_type", "kind") or "").lower()
        if "create" in explicit:
            kind = "create"
        elif "exercise" in explicit:
            kind = "exercise"
        elif "archive" in explicit:
            kind = "archive"

    resolved_event_id = str(pick(variant, "event_id", "eventId", "node_id", "nodeId") or event_id)
    return TraceEvent(
        event_id=resolved_event_id,
        kind=kind,
        template=template_name(pick(variant, "template_id", "templateId")),
        contract_id=pick(variant, "contract_id", "contractId"),
        choice=pick(variant, "choice"),
        consuming=pick(variant, "consuming"),
        acting_parties=list_str(pick(variant, "acting_parties", "actingParties") or []),
        witnesses=list_str(pick(variant, "witness_parties", "witnessParties", "witnesses") or []),
        signatories=list_str(pick(variant, "signatories") or []),
        observers=list_str(pick(variant, "observers") or []),
        child_event_ids=list_str(pick(variant, "child_event_ids", "childEventIds") or []),
        payload=pick(variant, "create_arguments", "createArguments", "create_argument", "createArgument", "payload"),
        argument=pick(variant, "choice_argument", "choiceArgument", "exercise_argument", "exerciseArgument", "argument"),
        result=pick(variant, "exercise_result", "exerciseResult", "result"),
        raw=event_raw,
    )


def infer_roots(events_by_id: dict[str, TraceEvent]) -> list[str]:
    children = {child for ev in events_by_id.values() for child in ev.child_event_ids}
    roots = [event_id for event_id in events_by_id if event_id not in children]
    return roots or list(events_by_id.keys())


def link_range_children(events_by_id: dict[str, TraceEvent]) -> None:
    numeric_ids = sorted(
        [event_id for event_id in events_by_id if event_id.isdigit()],
        key=lambda value: int(value),
    )
    if not numeric_ids:
        return

    def last_descendant(event_id: str) -> int | None:
        ev = events_by_id[event_id]
        variant = event_variant(ev.raw)
        value = pick(variant, "last_descendant_node_id", "lastDescendantNodeId")
        try:
            return int(value)
        except (TypeError, ValueError):
            return None

    for event_id in numeric_ids:
        ev = events_by_id[event_id]
        if ev.child_event_ids:
            continue
        last = last_descendant(event_id)
        if last is None:
            continue
        current = int(event_id)
        child_ids: list[str] = []
        index = numeric_ids.index(event_id) + 1
        while index < len(numeric_ids):
            child_id = numeric_ids[index]
            child_node_id = int(child_id)
            if child_node_id > last:
                break
            child_ids.append(child_id)
            child_last = last_descendant(child_id)
            if child_last is not None and child_last > child_node_id:
                while index + 1 < len(numeric_ids) and int(numeric_ids[index + 1]) <= child_last:
                    index += 1
            index += 1
        ev.child_event_ids = child_ids


def event_variant(event_raw: dict[str, Any]) -> dict[str, Any]:
    for key in (
        "created",
        "CreatedEvent",
        "createdEvent",
        "exercised",
        "ExercisedEvent",
        "exercisedEvent",
        "archived",
        "ArchivedEvent",
        "archivedEvent",
    ):
        value = event_raw.get(key)
        if isinstance(value, dict):
            return value
    return event_raw


class Color:
    codes = {
        "reset": "\033[0m",
        "bold": "\033[1m",
        "dim": "\033[2m",
        "red": "\033[31m",
        "green": "\033[32m",
        "yellow": "\033[33m",
        "blue": "\033[34m",
        "magenta": "\033[35m",
        "cyan": "\033[36m",
        "gray": "\033[90m",
    }

    def __init__(self, enabled: bool) -> None:
        self.enabled = enabled

    @classmethod
    def from_mode(cls, mode: str) -> "Color":
        if mode == "always":
            return cls(True)
        if mode == "never":
            return cls(False)
        return cls(sys.stdout.isatty() and "NO_COLOR" not in os.environ)

    def apply(self, text: str, *styles: str) -> str:
        if not self.enabled or not styles:
            return text
        prefix = "".join(self.codes[style] for style in styles if style in self.codes)
        return f"{prefix}{text}{self.codes['reset']}"


def print_pretty_trace(trace: NormalizedTrace, color: Color, source_index: "SourceIndex | None" = None) -> None:
    ctx = RenderContext(trace)
    source_index = source_index or SourceIndex()
    print(color.apply("Canton trace", "bold"))
    print(f"  update:       {short(trace.update_id, 80)}")
    print(f"  source:       {trace.source} ({trace.source_url or '-'})")
    print(f"  offset:       {trace.offset or '-'}")
    print(f"  record time:  {trace.record_time or '-'}")
    print(f"  synchronizer: {short(trace.synchronizer_id, 80)}")
    if trace.projection.get("readAs"):
        print(f"  read-as:      {', '.join(ctx.party_with_full(party) for party in trace.projection['readAs'])}")
    print(f"  visibility:   {trace.projection['note']}")
    print(f"  events:       {state_diff_summary(trace, color)}")
    if source_index.has_sources():
        print(f"  source roots: {', '.join(source_index.roots)}")
    if ctx.party_aliases:
        print("  parties:")
        for party, alias in sorted(ctx.party_aliases.items(), key=lambda item: item[1]):
            print(f"    {alias} = {party}")
    print()

    if not trace.root_event_ids:
        print(color.apply("No events found.", "yellow"))
        return

    print(color.apply("Trace", "bold"))
    for index, event_id in enumerate(trace.root_event_ids):
        is_last = index == len(trace.root_event_ids) - 1
        print_event_tree(trace, event_id, prefix="", is_last=is_last, color=color, ctx=ctx, source_index=source_index)


def state_diff_summary(trace: NormalizedTrace, color: Color) -> str:
    counts = {"create": 0, "exercise": 0, "archive": 0, "event": 0}
    for ev in trace.events_by_id.values():
        counts[ev.kind if ev.kind in counts else "event"] += 1
    parts = [
        color.apply(f"+{counts['create']} create", "green"),
        color.apply(f">{counts['exercise']} exercise", "yellow"),
        color.apply(f"x{counts['archive']} archive", "red"),
    ]
    if counts["event"]:
        parts.append(color.apply(f"{counts['event']} other", "blue"))
    return ", ".join(parts)


def event_color(kind: str) -> str:
    return {
        "create": "green",
        "exercise": "yellow",
        "archive": "red",
    }.get(kind, "blue")


def print_event_tree(
    trace: NormalizedTrace,
    event_id: str,
    prefix: str,
    is_last: bool,
    color: Color,
    ctx: "RenderContext",
    source_index: "SourceIndex",
) -> None:
    ev = trace.events_by_id.get(event_id)
    if ev is None:
        return

    connector = "`-- " if is_last else "|-- "
    child_prefix = "    " if is_last else "|   "
    print(prefix + connector + event_title(ev, color))

    detail_lines = event_detail_lines(ev, color, ctx, source_index)
    for index, line in enumerate(detail_lines):
        detail_last = index == len(detail_lines) - 1 and not ev.child_event_ids
        detail_connector = "`-- " if detail_last else "|-- "
        print(prefix + child_prefix + detail_connector + line)

    next_prefix = prefix + child_prefix
    for index, child_id in enumerate(ev.child_event_ids):
        print_event_tree(
            trace,
            child_id,
            prefix=next_prefix,
            is_last=index == len(ev.child_event_ids) - 1,
            color=color,
            ctx=ctx,
            source_index=source_index,
        )


def event_title(ev: TraceEvent, color: Color) -> str:
    kind = ev.kind.upper()
    kind_style = {
        "create": "green",
        "exercise": "yellow",
        "archive": "red",
    }.get(ev.kind, "blue")
    target = event_target(ev)
    marker = {"create": "CREATE", "exercise": "EXERCISE", "archive": "ARCHIVE"}.get(ev.kind, kind)
    return (
        color.apply(f"[{ev.event_id}]", "gray")
        + " "
        + color.apply(marker, kind_style, "bold")
        + " "
        + color.apply(target, "bold")
    )


def event_target(ev: TraceEvent) -> str:
    template = short_template(ev.template) or "<unknown>"
    if ev.choice:
        return f"{template}.{ev.choice}"
    return template


def event_detail_lines(ev: TraceEvent, color: Color, ctx: "RenderContext", source_index: "SourceIndex | None" = None) -> list[str]:
    lines: list[str] = []
    if source_index is not None:
        loc = source_index.location_for_event(ev)
        if loc is not None:
            lines.append(label_value("source", f"{Path(loc.path).name}:{loc.line} ({loc.label})", color))
    if ev.contract_id:
        lines.append(label_value("contract", short(ev.contract_id, 66), color))
    if ev.consuming is not None and ev.kind == "exercise":
        lines.append(label_value("consuming", str(ev.consuming).lower(), color))
    if ev.acting_parties:
        lines.append(label_value("actors", ", ".join(ctx.party(party) for party in ev.acting_parties), color))
    if ev.signatories:
        lines.append(label_value("signatories", ", ".join(ctx.party(party) for party in ev.signatories), color))
    if ev.observers:
        lines.append(label_value("observers", ", ".join(ctx.party(party) for party in ev.observers), color))
    if ev.witnesses:
        lines.append(label_value("witnesses", ", ".join(ctx.party(party) for party in ev.witnesses), color))
    if ev.argument is not None:
        lines.extend(block_lines("argument", ev.argument, color, ctx))
    if ev.payload is not None:
        lines.extend(block_lines("payload", ev.payload, color, ctx))
    if ev.result is not None:
        lines.extend(block_lines("result", ev.result, color, ctx))
    return lines


def label_value(label: str, value: str, color: Color) -> str:
    return f"{color.apply(label + ':', 'cyan')} {value}"


def block_lines(label: str, value: Any, color: Color, ctx: "RenderContext") -> list[str]:
    rendered = render_pretty_value(value, ctx)
    if "\n" not in rendered:
        return [label_value(label, short(rendered, 120), color)]
    lines = [color.apply(label + ":", "cyan")]
    lines.extend("  " + line for line in rendered.splitlines())
    return lines


def render_pretty_value(value: Any, ctx: "RenderContext | None" = None) -> str:
    simplified = simplify_lf_value(value)
    if ctx is not None:
        simplified = ctx.render_value(simplified)
    if isinstance(simplified, dict):
        if not simplified:
            return "{}"
        items = ", ".join(f"{key}: {format_scalar(val, ctx)}" for key, val in simplified.items())
        if len(items) <= 100 and all("\n" not in str(val) for val in simplified.values()):
            return "{ " + items + " }"
    if isinstance(simplified, list):
        items = ", ".join(format_scalar(item, ctx) for item in simplified)
        if len(items) <= 100:
            return "[" + items + "]"
    if not isinstance(simplified, (dict, list)):
        return format_scalar(simplified, ctx)
    return json.dumps(simplified, indent=2, sort_keys=True)


def simplify_lf_value(value: Any) -> Any:
    if isinstance(value, dict):
        if "sum" in value and len(value) == 1:
            return simplify_lf_value(value["sum"])
        if "fields" in value and isinstance(value["fields"], list):
            return {
                str(field.get("label", index)): simplify_lf_value(field.get("value"))
                for index, field in enumerate(value["fields"])
                if isinstance(field, dict)
            }
        if "record" in value and isinstance(value["record"], dict):
            return simplify_lf_value(value["record"])
        for scalar_key in ("party", "int64", "numeric", "text", "contract_id", "contractId", "timestamp", "date", "bool"):
            if scalar_key in value and len(value) == 1:
                return value[scalar_key]
        if "list" in value and isinstance(value["list"], dict):
            return simplify_lf_value(value["list"].get("elements", []))
        if "optional" in value and isinstance(value["optional"], dict):
            optional = value["optional"]
            if "value" not in optional:
                return None
            return simplify_lf_value(optional["value"])
        if "variant" in value and isinstance(value["variant"], dict):
            variant = value["variant"]
            constructor = pick(variant, "constructor", "variant") or "variant"
            return {str(constructor): simplify_lf_value(variant.get("value"))}
        if "enum" in value and isinstance(value["enum"], dict):
            return pick(value["enum"], "constructor", "value") or value["enum"]
        return {key: simplify_lf_value(val) for key, val in value.items() if key not in ("record_id", "recordId")}
    if isinstance(value, list):
        return [simplify_lf_value(item) for item in value]
    return value


class RenderContext:
    def __init__(self, trace: NormalizedTrace) -> None:
        self.party_aliases = build_party_aliases(trace)

    def party(self, value: str) -> str:
        return self.party_aliases.get(value, value)

    def party_with_full(self, value: str) -> str:
        alias = self.party_aliases.get(value)
        if not alias:
            return value
        return f"{alias} ({short_party(value)})"

    def render_value(self, value: Any) -> Any:
        if isinstance(value, str):
            return self.party(value)
        if isinstance(value, list):
            return [self.render_value(item) for item in value]
        if isinstance(value, dict):
            return {key: self.render_value(val) for key, val in value.items()}
        return value


def build_party_aliases(trace: NormalizedTrace) -> dict[str, str]:
    parties: set[str] = set()
    parties.update(trace.projection.get("readAs") or [])

    for ev in trace.events_by_id.values():
        parties.update(ev.acting_parties)
        parties.update(ev.witnesses)
        parties.update(ev.signatories)
        parties.update(ev.observers)
        collect_party_ids(ev.payload, parties)
        collect_party_ids(ev.argument, parties)
        collect_party_ids(ev.result, parties)

    names: dict[str, list[str]] = {}
    for party in parties:
        parsed = split_party_id(party)
        if parsed is None:
            continue
        name, _fingerprint = parsed
        names.setdefault(name, []).append(party)

    aliases: dict[str, str] = {}
    for name, party_ids in names.items():
        sorted_parties = sorted(set(party_ids))
        if len(sorted_parties) == 1:
            aliases[sorted_parties[0]] = name
        else:
            for party in sorted_parties:
                _name, fingerprint = split_party_id(party) or (name, "")
                aliases[party] = f"{name}@{fingerprint[:8]}"
    return aliases


def collect_party_ids(value: Any, parties: set[str]) -> None:
    if isinstance(value, str):
        if split_party_id(value) is not None:
            parties.add(value)
        return
    if isinstance(value, list):
        for item in value:
            collect_party_ids(item, parties)
        return
    if isinstance(value, dict):
        for item in value.values():
            collect_party_ids(item, parties)


def split_party_id(value: str) -> tuple[str, str] | None:
    if "::" not in value:
        return None
    name, fingerprint = value.split("::", 1)
    if not name or not fingerprint:
        return None
    if not re.fullmatch(r"[0-9a-fA-F]{16,}", fingerprint):
        return None
    return name, fingerprint


def short_party(value: str) -> str:
    parsed = split_party_id(value)
    if parsed is None:
        return short(value, 80)
    name, fingerprint = parsed
    return f"{name}::{fingerprint[:8]}...{fingerprint[-6:]}"


def format_scalar(value: Any, ctx: RenderContext | None = None) -> str:
    if isinstance(value, str):
        if ctx is not None:
            value = ctx.party(value)
        return value
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    return json.dumps(value, sort_keys=True)


def short_template(template: str | None) -> str | None:
    if not template:
        return None
    parts = template.split(":")
    if len(parts) >= 3:
        return ":".join(parts[1:])
    return template


class SourceIndex:
    def __init__(self, debug_info_paths: list[str] | None = None) -> None:
        self.roots: list[str] = []
        self.templates: dict[str, SourceLocation] = {}
        self.choices: dict[str, SourceLocation] = {}
        self.files: dict[str, list[str]] = {}
        for path in debug_info_paths or []:
            self._load_debug_info(Path(path).expanduser())

    def _load_debug_info(self, path: Path) -> None:
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            return
        if not isinstance(data, dict):
            return
        package_id = data.get("packageId")
        if not isinstance(package_id, str) or not package_id:
            return
        source_root = Path(str(data.get("sourceRoot") or path.parent)).expanduser()
        if not source_root.is_absolute():
            source_root = (path.parent / source_root).resolve()
        source_root_str = str(source_root)
        if source_root_str not in self.roots:
            self.roots.append(source_root_str)
        for file_info in data.get("files") or []:
            if not isinstance(file_info, dict):
                continue
            raw_file_path = file_info.get("path")
            if not raw_file_path:
                continue
            file_path = Path(str(raw_file_path)).expanduser()
            if not file_path.is_absolute():
                file_path = source_root / file_path
            if file_path.exists():
                try:
                    self.files[str(file_path)] = file_path.read_text(encoding="utf-8").splitlines()
                except UnicodeDecodeError:
                    self.files[str(file_path)] = file_path.read_text(errors="replace").splitlines()
            for entity in file_info.get("entities") or []:
                if not isinstance(entity, dict):
                    continue
                key = entity.get("qualifiedName")
                kind = entity.get("kind")
                line = entity.get("startLine")
                if not isinstance(key, str) or not isinstance(kind, str) or not isinstance(line, int):
                    continue
                package_key = f"{package_id}:{key}"
                loc = SourceLocation(str(file_path), line, key)
                if kind == "template":
                    self.templates[package_key] = loc
                elif kind == "choice":
                    self.choices[package_key] = loc

    def location_for_event(self, ev: TraceEvent) -> SourceLocation | None:
        parsed = parse_template_ref(ev.template)
        if parsed is None:
            return None
        package_id, module, entity = parsed
        if ev.choice:
            return self.choices.get(f"{package_id}:{module}:{entity}.{ev.choice}")
        return self.templates.get(f"{package_id}:{module}:{entity}")

    def snippet(self, loc: SourceLocation, radius: int = 2) -> str:
        lines = self.files.get(loc.path)
        if not lines:
            return f"{loc.path}:{loc.line}"
        start = max(loc.line - radius, 1)
        end = min(loc.line + radius, len(lines))
        rendered: list[str] = []
        width = len(str(end))
        for line_no in range(start, end + 1):
            marker = ">" if line_no == loc.line else " "
            rendered.append(f"{marker} {line_no:{width}d} | {lines[line_no - 1]}")
        return f"{loc.path}:{loc.line}\n" + "\n".join(rendered)

    def body_lines(self, loc: SourceLocation) -> list[SourceLine]:
        lines = self.files.get(loc.path)
        if not lines:
            return []

        result: list[SourceLine] = []
        choice_indent = leading_spaces(lines[loc.line - 1]) if loc.line - 1 < len(lines) else 0
        for idx in range(loc.line + 1, len(lines) + 1):
            text = lines[idx - 1]
            stripped = text.strip()
            if not stripped:
                result.append(SourceLine(loc.path, idx, text))
                continue
            indent = leading_spaces(text)
            if indent <= choice_indent and re.match(r"(choice|template)\s+\S+", stripped):
                break
            if indent <= choice_indent and stripped.startswith("-- |"):
                break
            result.append(SourceLine(loc.path, idx, text))
        return result

    def has_sources(self) -> bool:
        return bool(self.templates or self.choices or self.files)


def leading_spaces(value: str) -> int:
    return len(value) - len(value.lstrip(" "))


def source_index_from_args(args: argparse.Namespace, bundle: dict[str, Any] | None = None) -> SourceIndex:
    debug_info_paths: list[str] = []
    debug_info_paths.extend(getattr(args, "debug_info", []) or [])
    packages = (bundle or {}).get("packages") or {}
    debug_info_paths.extend(list_str(packages.get("debugInfoPaths") or []))
    return SourceIndex(unique([path for path in debug_info_paths if path]))


def parse_template_ref(template: str | None) -> tuple[str, str, str] | None:
    if not template:
        return None
    parts = template.split(":")
    if len(parts) < 3:
        return None
    return parts[0], parts[1], parts[2]


def input_contract_payload(bundle: dict[str, Any] | None, contract_id: str | None) -> Any:
    if bundle is None or not contract_id:
        return None
    acs = bundle.get("acsSnapshot") or {}
    return find_contract_payload(acs.get("response"), contract_id)


def find_contract_payload(value: Any, contract_id: str) -> Any:
    if isinstance(value, list):
        for item in value:
            found = find_contract_payload(item, contract_id)
            if found is not None:
                return found
        return None
    if not isinstance(value, dict):
        return None

    created = pick(value, "createdEvent", "created_event", "created", "CreatedEvent")
    if isinstance(created, dict):
        cid = pick(created, "contractId", "contract_id")
        if cid == contract_id:
            payload = pick(created, "createArgument", "create_arguments", "createArguments", "payload")
            return simplify_lf_value(payload)

    cid = pick(value, "contractId", "contract_id")
    if cid == contract_id:
        payload = pick(value, "createArgument", "create_arguments", "createArguments", "payload")
        if payload is not None:
            return simplify_lf_value(payload)

    for child in value.values():
        if isinstance(child, (dict, list)):
            found = find_contract_payload(child, contract_id)
            if found is not None:
                return found
    return None


def child_create_payload(trace: NormalizedTrace, ev: TraceEvent) -> Any:
    for child_id in ev.child_event_ids:
        child = trace.events_by_id.get(child_id)
        if child is not None and child.kind == "create" and child.payload is not None:
            return simplify_lf_value(child.payload)
    return None


def expression_environment(ev: TraceEvent, bundle: dict[str, Any] | None) -> dict[str, Any]:
    env: dict[str, Any] = {}
    input_contract = input_contract_payload(bundle, ev.contract_id)
    if isinstance(input_contract, dict):
        env.update(input_contract)
        env["this"] = input_contract
    if ev.argument is not None:
        argument = simplify_lf_value(ev.argument)
        env["choiceArgument"] = argument
        if isinstance(argument, dict):
            env.update(argument)
    return env


def expression_steps_for_event(
    trace: NormalizedTrace,
    bundle: dict[str, Any] | None,
    source_index: SourceIndex,
    ev: TraceEvent,
) -> list[ExpressionStep]:
    loc = source_index.location_for_event(ev)
    if loc is None:
        return []
    source_lines = source_index.body_lines(loc)
    if not source_lines:
        return []

    env = expression_environment(ev, bundle)
    output_payload = child_create_payload(trace, ev)
    steps: list[ExpressionStep] = [
        ExpressionStep(
            line=SourceLine(loc.path, loc.line, source_index.files.get(loc.path, [""])[loc.line - 1]),
            label=f"enter {ev.choice or event_target(ev)}",
            expression=ev.choice or event_target(ev),
            variables=env.copy(),
            result=None,
            note="source-linked replay step",
        )
    ]

    for line in source_lines:
        stripped = line.text.strip()
        if not stripped or stripped.startswith("--"):
            continue
        if stripped.startswith("controller "):
            expr = stripped.removeprefix("controller ").strip()
            result = eval_daml_expression(expr, env)
            steps.append(ExpressionStep(line, "authorize controller", expr, env.copy(), result))
            continue
        if stripped == "do":
            steps.append(ExpressionStep(line, "enter do block", stripped, env.copy(), None))
            continue
        if stripped.startswith("create this with "):
            assignment_text = stripped.removeprefix("create this with ").strip()
            assignments = parse_record_update_assignments(assignment_text)
            if not assignments:
                steps.append(ExpressionStep(line, "create", stripped, env.copy(), output_payload))
                continue
            for field, expr in assignments:
                result = eval_daml_expression(expr, env)
                steps.append(
                    ExpressionStep(
                        line=line,
                        label=f"evaluate {field}",
                        expression=expr,
                        variables=env.copy(),
                        result=result,
                    )
                )
                env[field] = result
            steps.append(
                ExpressionStep(
                    line=line,
                    label="create contract",
                    expression=stripped,
                    variables=env.copy(),
                    result=output_payload if output_payload is not None else env.get("this"),
                )
            )
            continue
        if stripped.startswith("create "):
            steps.append(ExpressionStep(line, "create", stripped, env.copy(), output_payload))
            continue
        if "<-" in stripped:
            name, expr = [part.strip() for part in stripped.split("<-", 1)]
            result = eval_daml_expression(expr, env)
            steps.append(ExpressionStep(line, f"bind {name}", expr, env.copy(), result))
            if result is not None:
                env[name] = result
            continue
        steps.append(ExpressionStep(line, "evaluate", stripped, env.copy(), eval_daml_expression(stripped, env)))

    return steps


def parse_record_update_assignments(value: str) -> list[tuple[str, str]]:
    assignments: list[tuple[str, str]] = []
    for part in split_top_level(value, ";"):
        if "=" not in part:
            continue
        field, expr = part.split("=", 1)
        field = field.strip()
        expr = expr.strip()
        if field and expr:
            assignments.append((field, expr))
    return assignments


def split_top_level(value: str, delimiter: str) -> list[str]:
    parts: list[str] = []
    depth = 0
    start = 0
    for idx, char in enumerate(value):
        if char in "([{":
            depth += 1
        elif char in ")]}":
            depth = max(depth - 1, 0)
        elif char == delimiter and depth == 0:
            parts.append(value[start:idx].strip())
            start = idx + 1
    parts.append(value[start:].strip())
    return [part for part in parts if part]


def eval_daml_expression(expr: str, env: dict[str, Any]) -> Any:
    expr = expr.strip()
    if not expr:
        return None
    if expr in env:
        return env[expr]
    if re.fullmatch(r"-?[0-9]+", expr):
        return int(expr)
    if expr.startswith('"') and expr.endswith('"'):
        return expr[1:-1]
    if expr.startswith("(") and expr.endswith(")"):
        return eval_daml_expression(expr[1:-1], env)
    for op in ("+", "-"):
        left, right = split_binary(expr, op)
        if left is not None and right is not None:
            left_value = eval_daml_expression(left, env)
            right_value = eval_daml_expression(right, env)
            if op == "+":
                return coerce_int(left_value) + coerce_int(right_value)
            return coerce_int(left_value) - coerce_int(right_value)
    return None


def split_binary(expr: str, operator: str) -> tuple[str | None, str | None]:
    depth = 0
    for idx in range(len(expr) - 1, -1, -1):
        char = expr[idx]
        if char in ")]}":
            depth += 1
        elif char in "([{":
            depth = max(depth - 1, 0)
        elif char == operator and depth == 0 and idx > 0:
            return expr[:idx].strip(), expr[idx + 1 :].strip()
    return None, None


def coerce_int(value: Any) -> int:
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, int):
        return value
    if isinstance(value, str) and re.fullmatch(r"-?[0-9]+", value):
        return int(value)
    raise ValueError(f"cannot evaluate integer expression from {value!r}")


@dataclass
class Breakpoint:
    spec: str

    def matches(self, step: int, event_id: str, ev: TraceEvent, loc: SourceLocation | None) -> bool:
        spec = self.spec.strip()
        if not spec:
            return False
        lowered = spec.lower()
        if spec == event_id or lowered == f"#{event_id}".lower() or spec == str(step + 1):
            return True
        target = event_target(ev).lower()
        if lowered == target or lowered in target:
            return True
        if loc is None:
            return False
        if lowered == loc.label.lower() or lowered in loc.label.lower():
            return True
        file_part, sep, line_part = spec.rpartition(":")
        if sep and line_part.isdigit():
            try:
                line = int(line_part)
            except ValueError:
                return False
            if line != loc.line:
                return False
            file_part = file_part.strip()
            return not file_part or loc.path.endswith(file_part)
        return loc.path.endswith(spec)


class Stepper:
    def __init__(
        self,
        trace: NormalizedTrace,
        bundle: dict[str, Any] | None = None,
        source_index: SourceIndex | None = None,
        color: Color | None = None,
    ) -> None:
        self.trace = trace
        self.bundle = bundle
        self.source_index = source_index or SourceIndex()
        self.color = color or Color(False)
        self.breakpoints: list[Breakpoint] = []
        self.order = self._preorder()
        self.index = 0
        self.expression_event_id: str | None = None
        self.expression_index = 0

    def run(self) -> None:
        print_summary(self.trace)
        print("\n" + self.color.apply("Interactive commands:", "bold") + " n/next, p/prev, s/source, expr, si/step-in, vars, b <spec>, c/continue, tree, context, json, q")
        if self.source_index.has_sources():
            print(self.color.apply("source roots:", "cyan"), ", ".join(self.source_index.roots))
        if not self.order:
            print("No events found.")
            return
        self.show_current()
        while True:
            try:
                cmd = input("dpm-trace> ").strip()
            except (EOFError, KeyboardInterrupt):
                print()
                return
            if cmd in ("q", "quit", "exit"):
                return
            if cmd in ("", "n", "next"):
                self.index = min(self.index + 1, len(self.order) - 1)
                self.show_current()
            elif cmd in ("p", "prev"):
                self.index = max(self.index - 1, 0)
                self.show_current()
            elif cmd.startswith("j "):
                self.jump(cmd)
            elif cmd in ("s", "src", "source"):
                self.show_source()
            elif cmd in ("expr", "expressions"):
                self.show_expression_steps()
            elif cmd in ("si", "step-in", "stepi"):
                self.step_expression()
            elif cmd in ("vars", "locals"):
                self.show_variables()
            elif cmd.startswith("b "):
                self.add_breakpoint(cmd)
            elif cmd in ("bp", "breakpoints"):
                self.list_breakpoints()
            elif cmd.startswith("clear"):
                self.clear_breakpoints(cmd)
            elif cmd in ("c", "continue"):
                self.continue_to_breakpoint()
            elif cmd == "tree":
                self.show_tree()
            elif cmd == "context":
                print(debug_context_report(self.trace))
            elif cmd == "replay":
                print(explain_replay())
            elif cmd == "json":
                event = self.trace.events_by_id[self.order[self.index]]
                print(json.dumps(event_to_json(event), indent=2, sort_keys=True))
            elif cmd == "help":
                print("n/next, p/prev, j <index>, s/source, expr, si/step-in, vars, b <spec>, bp, clear [n], c/continue, tree, context, replay, json, q")
            else:
                print("unknown command; try `help`")

    def _preorder(self) -> list[str]:
        seen: set[str] = set()
        order: list[str] = []

        def visit(event_id: str) -> None:
            if event_id in seen or event_id not in self.trace.events_by_id:
                return
            seen.add(event_id)
            order.append(event_id)
            for child in self.trace.events_by_id[event_id].child_event_ids:
                visit(child)

        for root in self.trace.root_event_ids:
            visit(root)
        for event_id in self.trace.events_by_id:
            visit(event_id)
        return order

    def show_current(self) -> None:
        ctx = RenderContext(self.trace)
        event_id = self.order[self.index]
        if self.expression_event_id != event_id:
            self.expression_event_id = event_id
            self.expression_index = 0
        ev = self.trace.events_by_id[event_id]
        color = self.color
        print("\n" + color.apply("-" * 72, "gray"))
        print(color.apply(f"Step {self.index + 1}/{len(self.order)}", "bold"), color.apply(ev.kind.upper(), event_color(ev.kind), "bold"), color.apply(ev.event_id, "gray"))
        print(label_value("template", ev.template or "-", color))
        loc = self.source_index.location_for_event(ev)
        if loc is not None:
            print(label_value("source", f"{loc.path}:{loc.line}  ({loc.label})", color))
        print(label_value("contract", short(ev.contract_id), color))
        if ev.choice:
            print(label_value("choice", f"{ev.choice}  consuming={ev.consuming}", color))
        if ev.acting_parties:
            print(label_value("actors", ", ".join(ctx.party(party) for party in ev.acting_parties), color))
        if ev.witnesses:
            print(label_value("witness", ", ".join(ctx.party(party) for party in ev.witnesses), color))
        if ev.signatories or ev.observers:
            signatories = [ctx.party(party) for party in ev.signatories]
            observers = [ctx.party(party) for party in ev.observers]
            print(label_value("stakeholders", f"signatories={signatories or []} observers={observers or []}", color))
        if ev.payload is not None:
            print(color.apply("payload:", "cyan"))
            print(indent_text(render_pretty_value(ev.payload, ctx)))
        if ev.argument is not None:
            print(color.apply("choice argument:", "cyan"))
            print(indent_text(render_pretty_value(ev.argument, ctx)))
        if ev.result is not None:
            print(color.apply("choice result:", "cyan"))
            print(indent_text(render_pretty_value(ev.result, ctx)))
        if ev.child_event_ids:
            print(label_value("children", ", ".join(ev.child_event_ids), color))

    def show_source(self) -> None:
        ev = self.trace.events_by_id[self.order[self.index]]
        loc = self.source_index.location_for_event(ev)
        if loc is None:
            print(self.color.apply("no source location available for this step; provide matching --debug-info or registry metadata", "yellow"))
            return
        print(self.render_source_snippet(loc))

    def render_source_snippet(self, loc: SourceLocation, radius: int = 2) -> str:
        lines = self.source_index.files.get(loc.path)
        if not lines:
            return f"{loc.path}:{loc.line}"
        start = max(loc.line - radius, 1)
        end = min(loc.line + radius, len(lines))
        width = len(str(end))
        rendered = [self.color.apply(f"{loc.path}:{loc.line}", "cyan")]
        for line_no in range(start, end + 1):
            marker = ">" if line_no == loc.line else " "
            prefix = f"{marker} {line_no:{width}d} | "
            line = lines[line_no - 1]
            if line_no == loc.line:
                rendered.append(self.color.apply(prefix + line, "yellow", "bold"))
            else:
                rendered.append(self.color.apply(prefix, "gray") + line)
        return "\n".join(rendered)

    def show_variables(self) -> None:
        event_id = self.order[self.index]
        ev = self.trace.events_by_id[event_id]
        ctx = RenderContext(self.trace)
        variables = self.step_variables(ev, ctx)
        print(self.color.apply("variables", "bold"))
        if not variables:
            print("  -")
            return
        for key, value in variables.items():
            rendered = render_pretty_value(value, ctx)
            if "\n" in rendered:
                print(f"  {self.color.apply(key + ':', 'cyan')}")
                print(indent_text(rendered))
            else:
                print(f"  {self.color.apply(key + ':', 'cyan')} {rendered}")

    def show_expression_steps(self) -> None:
        steps = self.current_expression_steps()
        if not steps:
            print(self.color.apply("no expression steps available; provide source metadata and a replay bundle with visible inputs", "yellow"))
            return
        for idx, step in enumerate(steps, start=1):
            print(f"{self.color.apply(str(idx) + '.', 'gray')} {self.color.apply(step.label, 'bold')}  {self.color.apply(Path(step.line.path).name + ':' + str(step.line.line), 'cyan')}")
            print(f"   {self.color.apply('source:', 'cyan')} {step.line.text.strip()}")
            if step.expression:
                print(f"   {self.color.apply('expr:', 'cyan')}   {step.expression}")
            if step.result is not None:
                print(f"   {self.color.apply('result:', 'green')} {render_pretty_value(step.result, RenderContext(self.trace))}")
            if step.note:
                print(f"   {self.color.apply('note:', 'gray')}   {step.note}")

    def step_expression(self) -> None:
        steps = self.current_expression_steps()
        if not steps:
            print(self.color.apply("no expression steps available", "yellow"))
            return
        if self.expression_index >= len(steps):
            self.expression_index = len(steps) - 1
        step = steps[self.expression_index]
        self.print_expression_step(step)
        if self.expression_index < len(steps) - 1:
            self.expression_index += 1

    def current_expression_steps(self) -> list[ExpressionStep]:
        event_id = self.order[self.index]
        ev = self.trace.events_by_id[event_id]
        return expression_steps_for_event(self.trace, self.bundle, self.source_index, ev)

    def print_expression_step(self, step: ExpressionStep) -> None:
        ctx = RenderContext(self.trace)
        color = self.color
        print("\n" + color.apply("-" * 72, "gray"))
        print(color.apply(f"Expression {self.expression_index + 1}", "bold"), color.apply(step.label, "magenta", "bold"))
        print(label_value("source", f"{step.line.path}:{step.line.line}", color))
        print(f"  {color.apply(step.line.text.strip(), 'bold')}")
        if step.expression:
            print(label_value("expr", step.expression, color))
        if step.variables:
            compact_vars = {
                key: value
                for key, value in step.variables.items()
                if key in ("this", "owner", "count", "amount", "choiceArgument")
            }
            if compact_vars:
                print(color.apply("vars:", "cyan"))
                for key, value in compact_vars.items():
                    print(f"  {color.apply(key + ':', 'cyan')} {render_pretty_value(value, ctx)}")
        if step.result is not None:
            print(f"{color.apply('result:', 'green')} {render_pretty_value(step.result, ctx)}")
        if step.note:
            print(f"{color.apply('note:', 'gray')}   {step.note}")

    def step_variables(self, ev: TraceEvent, ctx: RenderContext) -> dict[str, Any]:
        variables: dict[str, Any] = {
            "eventId": ev.event_id,
            "kind": ev.kind,
        }
        if ev.template:
            variables["template"] = ev.template
        if ev.contract_id:
            variables["contractId"] = ev.contract_id
        if ev.choice:
            variables["choice"] = ev.choice
        if ev.acting_parties:
            variables["actors"] = ev.acting_parties
        if ev.witnesses:
            variables["witnesses"] = ev.witnesses
        if ev.signatories:
            variables["signatories"] = ev.signatories
        if ev.payload is not None:
            variables["createPayload"] = simplify_lf_value(ev.payload)
        if ev.argument is not None:
            variables["choiceArgument"] = simplify_lf_value(ev.argument)
        if ev.result is not None:
            variables["choiceResult"] = simplify_lf_value(ev.result)
        input_contract = input_contract_payload(self.bundle, ev.contract_id)
        if input_contract is not None:
            variables["inputContract"] = input_contract
        return {key: ctx.render_value(value) for key, value in variables.items()}

    def add_breakpoint(self, cmd: str) -> None:
        _head, _sep, spec = cmd.partition(" ")
        spec = spec.strip()
        if not spec:
            print(self.color.apply("usage: b <event-id|template.choice|file:line>", "yellow"))
            return
        self.breakpoints.append(Breakpoint(spec))
        print(f"{self.color.apply('breakpoint', 'magenta', 'bold')} {len(self.breakpoints)} set: {spec}")

    def list_breakpoints(self) -> None:
        if not self.breakpoints:
            print(self.color.apply("no breakpoints", "yellow"))
            return
        for index, breakpoint in enumerate(self.breakpoints, start=1):
            print(f"{self.color.apply(str(index) + ':', 'gray')} {breakpoint.spec}")

    def clear_breakpoints(self, cmd: str) -> None:
        _head, _sep, value = cmd.partition(" ")
        value = value.strip()
        if not value:
            self.breakpoints.clear()
            print(self.color.apply("cleared all breakpoints", "yellow"))
            return
        try:
            index = int(value) - 1
        except ValueError:
            print(self.color.apply("usage: clear [breakpoint-number]", "yellow"))
            return
        if index < 0 or index >= len(self.breakpoints):
            print(self.color.apply(f"breakpoint must be between 1 and {len(self.breakpoints)}", "yellow"))
            return
        removed = self.breakpoints.pop(index)
        print(self.color.apply("cleared breakpoint:", "yellow"), removed.spec)

    def continue_to_breakpoint(self) -> None:
        if not self.breakpoints:
            print(self.color.apply("no breakpoints set", "yellow"))
            return
        if not self.order:
            print(self.color.apply("no events", "yellow"))
            return
        start = self.index + 1
        for idx in range(start, len(self.order)):
            event_id = self.order[idx]
            ev = self.trace.events_by_id[event_id]
            loc = self.source_index.location_for_event(ev)
            if any(bp.matches(idx, event_id, ev, loc) for bp in self.breakpoints):
                self.index = idx
                self.show_current()
                return
        print(self.color.apply("no later breakpoint hit", "yellow"))

    def show_tree(self) -> None:
        def visit(event_id: str, depth: int) -> None:
            ev = self.trace.events_by_id.get(event_id)
            if not ev:
                return
            marker = {"create": "+", "exercise": ">", "archive": "x"}.get(ev.kind, "-")
            label = ev.choice if ev.choice else ev.template
            print(f"{'  ' * depth}{marker} {ev.event_id} {ev.kind} {label or ''}")
            for child in ev.child_event_ids:
                visit(child, depth + 1)

        for root in self.trace.root_event_ids:
            visit(root, 0)

    def jump(self, cmd: str) -> None:
        _, _, value = cmd.partition(" ")
        try:
            idx = int(value) - 1
        except ValueError:
            print("usage: j <step-number>")
            return
        if idx < 0 or idx >= len(self.order):
            print(f"step must be between 1 and {len(self.order)}")
            return
        self.index = idx
        self.show_current()


def print_summary(trace: NormalizedTrace) -> None:
    print(f"update:      {trace.update_id}")
    print(f"source:      {trace.source} ({trace.source_url or '-'})")
    print(f"record time: {trace.record_time or '-'}")
    print(f"offset:      {trace.offset or '-'}")
    print(f"synchronizer:{trace.synchronizer_id or '-'}")
    print(f"projection:  {trace.projection['note']}")
    if trace.projection.get("readAs"):
        print(f"read-as:     {', '.join(trace.projection['readAs'])}")
    print(f"events:      {len(trace.events_by_id)}")


def debug_context_report(trace: NormalizedTrace) -> str:
    package_ids = sorted({
        package
        for ev in trace.events_by_id.values()
        for package in [package_from_template(ev.template)]
        if package
    })
    present = [
        "participant-visible transaction tree",
        "event order and parent/child links" if trace.root_event_ids else "flat event list",
        "choice arguments and create payloads where exposed",
        "party/witness labels where exposed",
        "source breakpoints and event/input-contract variables when matching debug-info metadata and replay bundle are available",
    ]
    if package_ids:
        present.append(f"package ids referenced by events: {', '.join(package_ids[:5])}")

    missing = [
        "verified DAR/source metadata unless provided by a registry",
        "full original command envelope unless captured at submission time",
        "private subtransactions outside this projection",
        "negative key lookups/fetch/no-such-key interpreter details not emitted in UpdateService trees",
        "exact replay state unless ACS/related contracts are captured at the right offset",
        "Daml-LF interpreter step hooks for expression-level local variables and step-into-choice-body debugging",
    ]
    return (
        "\nDebug context assessment\n"
        "------------------------\n"
        "Present in this trace:\n"
        + "\n".join(f"- {item}" for item in present)
        + "\n\nNeeded for true replay/step debugging:\n"
        + "\n".join(f"- {item}" for item in missing)
        + "\n\nConclusion: this POC supports event/source-level stepping now. LF expression stepping still needs engine instrumentation."
    )


def explain_apis() -> str:
    return textwrap.dedent(
        f"""
        Scan API vs Ledger API
        ----------------------
        Scan API:
        - Public/indexed network data from Super Validator Scan services.
        - Useful for CantonScan-like flows and public update lookup.
        - Endpoint used by this POC: GET {SCAN_UPDATE_PATH}
        - Does not prove access to a bank/private participant projection.

        Ledger JSON API:
        - Authenticated participant/validator API.
        - Requires participant URL, bearer token, and read-as/party context.
        - Endpoint used by this POC: POST {LEDGER_UPDATE_BY_ID_PATH}
        - Returns the participant-visible projection; it is not a global trace.

        In proposal terms:
        - Scan is the public entry point.
        - Ledger API is the private/richer debugging entry point.
        """
    ).strip()


def explain_replay() -> str:
    return textwrap.dedent(
        """
        What local step-by-step replay needs
        -----------------------------------
        Event/source stepping needs the transaction tree plus optional local source metadata.
        Expression-level execution replay needs more:

        - update id and participant projection
        - package ids plus verified DAR/source metadata
        - visible input contracts and ACS snapshot at the right offset
        - command/choice arguments and act-as/read-as context
        - ledger time and disclosed-contract context
        - Daml-LF engine instrumentation to pause/step inside choice bodies

        Likely extension points:
        - source/package registry for verified DAR/source metadata
        - replay bundle format that captures participant-visible state
        - Daml-LF engine trace/debug adapter for expression-level stepping

        What should not be claimed:
        - direct access to global Canton state
        - replay of private subtransactions outside the connected participant projection
        - direct access to participant Postgres databases as a product interface
        """
    ).strip()


def trace_to_json(trace: NormalizedTrace) -> dict[str, Any]:
    return {
        "updateId": trace.update_id,
        "source": trace.source,
        "sourceUrl": trace.source_url,
        "projection": trace.projection,
        "recordTime": trace.record_time,
        "offset": trace.offset,
        "synchronizerId": trace.synchronizer_id,
        "rootEventIds": trace.root_event_ids,
        "eventsById": {key: event_to_json(ev) for key, ev in trace.events_by_id.items()},
    }


def trace_from_json(data: dict[str, Any]) -> NormalizedTrace:
    events_json = data.get("eventsById") or {}
    if not isinstance(events_json, dict):
        raise ValueError("trace.eventsById must be an object")
    events_by_id = {
        str(event_id): event_from_json(event)
        for event_id, event in events_json.items()
        if isinstance(event, dict)
    }
    return NormalizedTrace(
        update_id=str(data.get("updateId") or ""),
        source=str(data.get("source") or "bundle"),
        source_url=data.get("sourceUrl"),
        projection=data.get("projection") if isinstance(data.get("projection"), dict) else {},
        root_event_ids=list_str(data.get("rootEventIds") or []),
        events_by_id=events_by_id,
        record_time=data.get("recordTime"),
        offset=str(data.get("offset") or "") or None,
        synchronizer_id=data.get("synchronizerId"),
        raw={},
    )


def event_to_json(ev: TraceEvent) -> dict[str, Any]:
    return {
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
    }


def event_from_json(data: dict[str, Any]) -> TraceEvent:
    return TraceEvent(
        event_id=str(data.get("eventId") or ""),
        kind=str(data.get("kind") or "event"),
        template=data.get("template"),
        contract_id=data.get("contractId"),
        choice=data.get("choice"),
        consuming=data.get("consuming"),
        acting_parties=list_str(data.get("actingParties") or []),
        witnesses=list_str(data.get("witnesses") or []),
        signatories=list_str(data.get("signatories") or []),
        observers=list_str(data.get("observers") or []),
        child_event_ids=list_str(data.get("childEventIds") or []),
        payload=data.get("payload"),
        argument=data.get("argument"),
        result=data.get("result"),
        raw={},
    )


def extract_update_id(target: str | None) -> str | None:
    if not target:
        return None
    match = re.search(r"/update/([^/?#]+)", target)
    if match:
        return match.group(1)
    return target


def parse_parties(values: list[str]) -> list[str]:
    parties: list[str] = []
    for value in values:
        for part in value.split(","):
            stripped = part.strip()
            if stripped:
                parties.append(stripped)
    return parties


def has_cli_option(argv: list[str], *names: str) -> bool:
    prefixes = tuple(name + "=" for name in names)
    return any(arg in names or arg.startswith(prefixes) for arg in argv)


def read_token_file(path: str | None) -> str | None:
    if not path:
        return None
    return Path(path).read_text(encoding="utf-8").strip()


def join_url(base: str, path: str) -> str:
    return base.rstrip("/") + "/" + path.lstrip("/")


def pick(obj: dict[str, Any], *keys: str) -> Any:
    for key in keys:
        if key in obj:
            return obj[key]
    return None


def list_str(value: Any) -> list[str]:
    if isinstance(value, list):
        return [str(item) for item in value]
    if value is None:
        return []
    return [str(value)]


def template_name(value: Any) -> str | None:
    if value is None:
        return None
    if isinstance(value, str):
        return value
    if isinstance(value, dict):
        package = pick(value, "package_id", "packageId", "packageName")
        module = pick(value, "module_name", "moduleName")
        entity = pick(value, "entity_name", "entityName")
        parts = [str(part) for part in (package, module, entity) if part]
        return ":".join(parts) if parts else json.dumps(value, sort_keys=True)
    return str(value)


def package_from_template(template: str | None) -> str | None:
    if not template or ":" not in template:
        return None
    return template.split(":", 1)[0]


def indent_json(value: Any) -> str:
    return textwrap.indent(json.dumps(value, indent=2, sort_keys=True), "  ")


def indent_text(value: str) -> str:
    return textwrap.indent(value, "  ")


def short(value: str | None, max_len: int = 32) -> str:
    if not value:
        return "-"
    if len(value) <= max_len:
        return value
    return value[: max_len - 3] + "..."


if __name__ == "__main__":
    raise SystemExit(main())
