#!/usr/bin/env bash
# Live demo: boots a throwaway Canton, runs five workflows through it, and
# shows what `dpm trace` makes of each one.
#
#   ./examples/demo.sh
#
# Needs `dpm` (for `dpm sandbox`) and the dpm-trace binary. Everything else --
# the Daml package, the DAR, the parties -- is set up here and thrown away on
# exit. Nothing touches your DPM home or an existing participant.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
LEDGER="http://127.0.0.1:6864"   # dpm sandbox's fixed JSON Ledger API port

# The binary: next to this script's parent (release archive), built in the repo,
# or on PATH.
if [[ -n "${DPM_TRACE_BIN:-}" ]]; then
  DPM=("$DPM_TRACE_BIN")
elif [[ -x "$ROOT/dpm-trace" ]]; then
  DPM=("$ROOT/dpm-trace")
elif command -v dpm-trace >/dev/null 2>&1; then
  DPM=(dpm-trace)
else
  echo "error: no dpm-trace binary found (set DPM_TRACE_BIN)" >&2
  exit 2
fi

command -v dpm >/dev/null 2>&1 || { echo "error: dpm not found on PATH" >&2; exit 2; }

DAR="$HERE/asset/.daml/dist/asset-tests-1.0.0.dar"
if [[ ! -f "$DAR" ]]; then
  echo "building the example DAR ..."
  (cd "$HERE/asset" && dpm build >/dev/null)
fi

# Config discovery walks up from the working directory, so a .dpm-trace.json in
# any ancestor of wherever this is run from would silently supply its values --
# a stale ledger url, or a placeholder party that the ledger rejects with
# "non expected character 0x2e in Daml-LF Party". Every path used below is
# absolute, so run from a directory that has no such ancestor.
cd "$(mktemp -d)"

SANDBOX_LOG="$(mktemp -t dpm-trace-demo)"
SANDBOX_PID=""
cleanup() {
  [[ -n "$SANDBOX_PID" ]] && kill "$SANDBOX_PID" 2>/dev/null || true
  # dpm sandbox runs Canton in a child JVM; killing the parent leaves it behind.
  pkill -f "canton.*sandbox" 2>/dev/null || true
  rm -f "$SANDBOX_LOG"
}
trap cleanup EXIT

step() { printf '\n\033[1m── %s\033[0m\n\n' "$1"; }

step "Booting dpm sandbox"
dpm sandbox --no-tty >"$SANDBOX_LOG" 2>&1 &
SANDBOX_PID=$!
# /v2/version answers before the participant has connected to its
# synchronizer, and uploading a DAR before then fails with
# PACKAGE_SERVICE_CANNOT_AUTODETECT_SYNCHRONIZER. Wait for the connection.
ready() {
  curl -sf "$LEDGER/v2/state/connected-synchronizers" 2>/dev/null \
  | python3 -c 'import sys,json; sys.exit(0 if json.load(sys.stdin).get("connectedSynchronizers") else 1)' 2>/dev/null
}
for _ in $(seq 1 120); do
  ready && break
  kill -0 "$SANDBOX_PID" 2>/dev/null || { cat "$SANDBOX_LOG" >&2; exit 1; }
  sleep 1
done
ready || { echo "sandbox did not become ready" >&2; cat "$SANDBOX_LOG" >&2; exit 1; }
echo "sandbox up on $LEDGER"

curl -sf -X POST "$LEDGER/v2/packages" \
  -H 'Content-Type: application/octet-stream' --data-binary "@$DAR" >/dev/null
echo "uploaded $(basename "$DAR")"

party() {
  curl -sf -X POST "$LEDGER/v2/parties" -H 'Content-Type: application/json' \
    -d "{\"partyIdHint\":\"$1\"}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["partyDetails"]["party"])'
}
ISSUER="$(party Issuer)"
ALICE="$(party Alice)"
BOB="$(party Bob)"
echo "allocated Issuer, Alice, Bob"

trace() { "${DPM[@]}" "$1" --submitter "$LEDGER" --read-as "$ISSUER" --color auto; }
submit() {
  local id
  id="$("${DPM[@]}" submit --submitter "$LEDGER" "$@")"
  case "$id" in
    1220*) printf '%s' "$id" ;;
    *) echo "submit failed:" >&2; echo "$id" >&2; exit 1 ;;
  esac
}
first_create() {
  "${DPM[@]}" "$1" --submitter "$LEDGER" --read-as "$ISSUER" --print-json \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(next(e["contractId"] for e in d["eventsById"].values() if e.get("kind")=="create"))'
}

step "1/5  Create — one event, with signatory, observer and payload"
CREATE_ID="$(submit --act-as "$ISSUER" --template '#asset-tests:Asset:Asset' \
  --arg issuer="$ISSUER" --arg owner="$ALICE" --arg name=GOLD --arg quantity=100)"
trace "$CREATE_ID"
CID="$(first_create "$CREATE_ID")"

step "2/5  Exercise with child creates — Split 100 into 60 and 40"
SPLIT_ID="$(submit --act-as "$ALICE" --template '#asset-tests:Asset:Asset' \
  --choice Split --contract-id "$CID" --arg splitQuantity=40)"
trace "$SPLIT_ID"
SPLIT_CID="$(first_create "$SPLIT_ID")"

step "3/5  Consuming exercise — Burn archives without creating anything"
BURN_ID="$(submit --act-as "$ALICE" --template '#asset-tests:Asset:Asset' \
  --choice Burn --contract-id "$SPLIT_CID")"
trace "$BURN_ID"

step "4/5  A rejected submission, mapped back to the Daml that rejected it"
FAIL_ID="$(submit --act-as "$ISSUER" --template '#asset-tests:Asset:Asset' \
  --arg issuer="$ISSUER" --arg owner="$ALICE" --arg name=SILVER --arg quantity=100)"
FAIL_CID="$(first_create "$FAIL_ID")"
COMPLETION="$(mktemp -t dpm-trace-demo-completion)"
"${DPM[@]}" submit --submitter "$LEDGER" --act-as "$ALICE" \
  --template '#asset-tests:Asset:Asset' --choice Withdraw \
  --contract-id "$FAIL_CID" --arg amount=500 \
  --allow-fail --print-json >"$COMPLETION"
"${DPM[@]}" --completion-file "$COMPLETION" --daml-yaml "$HERE/asset/daml.yaml" --color auto
rm -f "$COMPLETION"

step "5/5  Prepared vs committed — did the ledger do what was asked?"
PREPARED="$(mktemp -t dpm-trace-demo-prepared)"
"${DPM[@]}" prepare --submitter "$LEDGER" --act-as "$ALICE" --read-as "$BOB" \
  --template '#asset-tests:Asset:Asset' --choice Transfer \
  --contract-id "$FAIL_CID" --arg newOwner="$BOB" --export "$PREPARED" >/dev/null
TRANSFER_ID="$(submit --act-as "$ALICE" --read-as "$BOB" \
  --template '#asset-tests:Asset:Asset' --choice Transfer \
  --contract-id "$FAIL_CID" --arg newOwner="$BOB")"
TRACE_OUT="$(mktemp -t dpm-trace-demo-trace)"
"${DPM[@]}" "$TRANSFER_ID" --submitter "$LEDGER" --read-as "$ISSUER" --export "$TRACE_OUT" >/dev/null
"${DPM[@]}" compare --prepared "$PREPARED" --update "$TRACE_OUT" --full --color auto
rm -f "$PREPARED" "$TRACE_OUT"

step "Done"
cat <<EOF
The sandbox is being shut down; nothing was installed and nothing persists.

The same five outputs are committed as artifacts, so they can be replayed with
no ledger at all:

  ${DPM[*]} open examples/create.trace.json
  ${DPM[*]} compare --prepared examples/transfer.prepared.json \\
    --update examples/transfer.trace.json --full

See examples/README.md.
EOF
