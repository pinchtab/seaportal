#!/bin/bash
# Stop the fixtures server
PIDFILE="/tmp/seaportal-fixtures.pid"

if [ -f "$PIDFILE" ]; then
  PID=$(cat "$PIDFILE")
  if kill -0 "$PID" 2>/dev/null; then
    kill "$PID"
    echo "Fixtures server stopped (pid $PID)"
  else
    echo "Fixtures server not running (stale pid)"
  fi
  rm -f "$PIDFILE"
else
  echo "No fixtures server running"
fi
