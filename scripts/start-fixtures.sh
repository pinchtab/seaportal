#!/bin/bash
# Start the fixtures server for local development/testing
PORT="${1:-8099}"
DIR="$(cd "$(dirname "$0")/../tests/e2e/fixtures" && pwd)"
PIDFILE="/tmp/seaportal-fixtures.pid"

if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
  echo "Fixtures server already running (pid $(cat "$PIDFILE")) on port $PORT"
  exit 0
fi

python3 -m http.server "$PORT" --directory "$DIR" &>/dev/null &
echo $! > "$PIDFILE"
echo "Fixtures server started on http://localhost:$PORT (pid $!)"
