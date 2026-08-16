#!/usr/bin/env bash
#
# Start the local dev stack: backend on :$BACKEND_PORT, Vite on :$FRONTEND_PORT.
# Invoked by `make dev`; run it directly if you prefer.
#
# Lives in a script rather than the Makefile recipe because the shutdown needs
# `wait -n` and a signal-interruptible wait. A recipe that runs Vite in the
# foreground under POSIX `sh` defers its EXIT trap until that foreground
# command returns, so anything that kills the recipe non-interactively leaves
# the backend holding its port — the phantom listener this is meant to prevent.
#
# Both processes are backgrounded here and reaped together: whichever exits
# first takes the other down with it.
set -uo pipefail

BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
BINARY="${BINARY:-./whatomate}"
# Matches the Makefile's CONFIG default: checked in, no secrets, localhost
# hosts, so running this straight from a fresh clone works.
CONFIG="${CONFIG:-dev/config.toml}"

cd "$(dirname "$0")/.."

# Job control, so each background job below leads its own process group and can
# be killed as a group. `npm run dev` spawns Vite as a *grandchild*; signalling
# just the npm pid leaves Vite holding :$FRONTEND_PORT.
set -m

BACKEND_PID=""
FRONTEND_PID=""

kill_tree() {
  [ -n "$1" ] || return 0
  # Negative pid = the whole process group. Fall back to the bare pid if the
  # group is already gone.
  kill -- "-$1" 2>/dev/null || kill "$1" 2>/dev/null
}

cleanup() {
  trap - INT TERM EXIT
  kill_tree "$FRONTEND_PID"
  kill_tree "$BACKEND_PID"
  wait 2>/dev/null
}
trap cleanup INT TERM EXIT

WHATOMATE_SERVER__PORT="$BACKEND_PORT" "$BINARY" server -config "$CONFIG" -migrate &
BACKEND_PID=$!

echo "Waiting for backend on :${BACKEND_PORT} ..."
for _ in $(seq 1 60); do
  curl -sf "http://localhost:${BACKEND_PORT}/health" >/dev/null 2>&1 && break
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "Backend exited during startup (see its log above)." >&2
    exit 1
  fi
  sleep 1
done

if ! curl -sf "http://localhost:${BACKEND_PORT}/health" >/dev/null 2>&1; then
  echo "Backend never became healthy on :${BACKEND_PORT} — are Postgres and Redis up?" >&2
  exit 1
fi

cat <<EOF

  API      http://localhost:${BACKEND_PORT}   (also serves the last frontend build)
  App      http://localhost:${FRONTEND_PORT}   <-- open this

EOF

(cd frontend && npm run dev) &
FRONTEND_PID=$!

# Return as soon as either side exits; the trap reaps the survivor.
wait -n "$BACKEND_PID" "$FRONTEND_PID"
