# LG WebOS Smash Deck

A web dashboard and REST API for controlling LG TVs over the network using LG's Network IP Control protocol.

Part of the [Smash Deck](https://github.com/niski84/smash-deck-catalog) family - self-hosted dashboards built in Go for the homelab.

## What It Does

Speaks LG's Network IP Control protocol over TCP port 9761 (with AES encryption when an 8-character keycode is configured) to drive a TV without depending on the official mobile app. The dashboard exposes a full virtual remote: D-pad, volume slider (sequential key presses so HDMI CEC and soundbars track correctly), playback controls, channels, color buttons, and a number pad.

Beyond the remote it can launch streaming apps by ID, switch HDMI and tuner inputs, change picture mode, mute the screen, toggle energy saving, send Wake-on-LAN, and power the TV off. Every action is logged to a local activity log.

The same binary serves the web UI, a JSON REST API, and a CLI - useful for scripting or wiring the TV into other home automation.

## Tech Stack

- Go (single binary, no runtime dependencies)
- `golang.org/x/crypto` for AES keycode encryption
- Embedded vanilla HTML, CSS, and JavaScript (no framework)

## Running

```bash
go build -o lgdeck ./cmd/lgdeck
./lgdeck
```

Configure via environment variables (`PORT`, `TV_IP`, `TV_MAC`, `TV_KEYCODE`, `DATA_DIR`) or through the Settings tab in the UI. Default port is 8088. Settings persist to `DATA_DIR/lgdeck-settings.json`.

## Status

Active development.

## License

MIT
