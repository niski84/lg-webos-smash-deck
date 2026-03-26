#!/usr/bin/env bash
# Install e2e deps, run Playwright to capture frames, build docs/demo.gif with ffmpeg.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p docs e2e/screenshots/frames

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "[generate-demo-gif] ffmpeg not found — install it (e.g. apt install ffmpeg) to build the GIF."
  exit 1
fi

echo "[generate-demo-gif] npm install (e2e)..."
(cd e2e && npm install)

echo "[generate-demo-gif] playwright chromium..."
(cd e2e && npx playwright install chromium)

echo "[generate-demo-gif] running demo-gif test..."
(cd e2e && npx playwright test demo-gif)

FRAMES=(e2e/screenshots/frames/frame-*.png)
if [[ ! -e "${FRAMES[0]:-}" ]]; then
  echo "[generate-demo-gif] no frames in e2e/screenshots/frames/"
  exit 1
fi

# Input framerate sets dwell time per frame (~1.5s each at 0.65 fps).
echo "[generate-demo-gif] assembling docs/demo.gif ..."
ffmpeg -y \
  -framerate 0.65 \
  -pattern_type glob -i 'e2e/screenshots/frames/frame-*.png' \
  -vf "scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=128:stats_mode=single[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3" \
  -loop 0 \
  docs/demo.gif

echo "[generate-demo-gif] done → docs/demo.gif ($(du -h docs/demo.gif | cut -f1))"
