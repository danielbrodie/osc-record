#!/usr/bin/env bash
# test-record-cycle.sh
# End-to-end test of osc-record without hardware — uses avfoundation + webcam.
# Starts the daemon in --no-tui mode, fires record/stop via OSC, checks output file.
#
# Usage: ./scripts/test-record-cycle.sh [duration_seconds]
#   duration_seconds: how long to record (default: 5)
#
# Prerequisites: osc-record configured with record/stop addresses, webcam available.

set -euo pipefail

DURATION=${1:-5}
RECORD_ADDR=$(osc-record config 2>/dev/null | grep record_address | awk '{print $3}' | tr -d '"')
STOP_ADDR=$(osc-record config 2>/dev/null | grep stop_address | awk '{print $3}' | tr -d '"')
PORT=8000
HOST=127.0.0.1

if [[ -z "$RECORD_ADDR" || -z "$STOP_ADDR" ]]; then
  echo "Error: record_address or stop_address not configured."
  echo "Run: osc-record capture record && osc-record capture stop"
  exit 1
fi

echo "=== osc-record test cycle ==="
echo "Record: $RECORD_ADDR | Stop: $STOP_ADDR | Duration: ${DURATION}s"
echo ""

# Start daemon in background (no-tui, plaintext mode)
osc-record run --no-tui &
DAEMON_PID=$!
echo "Daemon PID: $DAEMON_PID"
sleep 2  # let it bind the port

# Send record trigger
echo "→ Sending record trigger: $RECORD_ADDR"
python3 "$(dirname "$0")/osc-send.py" "$RECORD_ADDR" --host "$HOST" --port "$PORT"

# Wait
echo "  Recording for ${DURATION}s..."
sleep "$DURATION"

# Send stop trigger
echo "→ Sending stop trigger: $STOP_ADDR"
python3 "$(dirname "$0")/osc-send.py" "$STOP_ADDR" --host "$HOST" --port "$PORT"
sleep 2  # let it finalize

# Kill daemon
kill "$DAEMON_PID" 2>/dev/null || true
wait "$DAEMON_PID" 2>/dev/null || true

echo ""
echo "=== Output files ==="
OUTPUT_DIR=$(osc-record config 2>/dev/null | grep output_dir | awk '{print $3}' | tr -d '"' | sed "s|~|$HOME|")
ls -lh "$OUTPUT_DIR"/*.mp4 2>/dev/null | tail -5 || echo "No .mp4 files found in $OUTPUT_DIR"

echo ""
echo "=== Done ==="
