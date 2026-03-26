#!/usr/bin/env bash
# Build and restart the LG WebOS Smash Deck server.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "[reload] Building lg-webos-smash-deck..."
go build -o lgdeck ./cmd/lgdeck/

echo "[reload] Stopping any running instance..."
pkill -f "./lgdeck" 2>/dev/null || pkill -f "lgdeck" 2>/dev/null || true
sleep 0.5

PORT="${PORT:-8088}"
echo "[reload] Starting on port $PORT ..."
nohup ./lgdeck > lgdeck.log 2>&1 &
echo "[reload] PID=$!"
echo "[reload] Waiting for server..."
for i in $(seq 1 20); do
  if curl -s "http://localhost:$PORT/api/health" | grep -q '"success":true'; then
    echo "[reload] Server is up → http://localhost:$PORT"
    exit 0
  fi
  sleep 0.3
done
echo "[reload] WARNING: server did not respond after 6s — check lgdeck.log"
exit 1
