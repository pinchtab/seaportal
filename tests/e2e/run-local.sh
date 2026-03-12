#!/bin/bash
# Run E2E tests locally
# Usage: ./run-local.sh [--no-docker]

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "🌊 SeaPortal E2E Tests (Local)"
echo ""

# Build the binary
echo "Building seaportal..."
cd ../..
go build -o seaportal ./cmd/seaportal
cd tests/e2e
echo "✓ Binary built"
echo ""

if [ "${1:-}" = "--no-docker" ]; then
  # Run without docker - start local server for fixtures
  echo "Running without Docker..."
  echo ""
  
  PORT=18765
  
  # Start a simple HTTP server for fixtures
  echo "Starting fixtures server on :$PORT..."
  cd fixtures
  python3 -m http.server $PORT > /dev/null 2>&1 &
  FIXTURES_PID=$!
  cd ..
  
  trap "kill $FIXTURES_PID 2>/dev/null" EXIT
  sleep 1
  
  # Run tests
  export FIXTURES_URL="http://localhost:$PORT"
  export PATH="$SCRIPT_DIR/../..:$PATH"
  export RESULTS_DIR="$SCRIPT_DIR/results"
  
  mkdir -p "$RESULTS_DIR"
  
  ./scenarios/run-all.sh
else
  # Run with docker compose
  echo "Running with Docker..."
  
  # Copy binary to runner context
  cp ../../seaportal runner/seaportal
  
  # Try docker compose (v2) first, fall back to docker-compose (v1)
  if command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
  elif docker compose version &> /dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
  else
    echo "Docker Compose not found. Use --no-docker to run without Docker."
    exit 1
  fi
  
  $COMPOSE_CMD up --build --abort-on-container-exit --exit-code-from runner
  $COMPOSE_CMD down
fi
