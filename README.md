# Remotely

Lightweight ad-hoc remote access (join.me / TeamViewer-style). No accounts, no installation for viewers. Host runs a native agent, shares a 9-digit key, viewer connects via browser.

---

## Architecture

```
  ┌──────────────────────┐   WebSocket (signaling)   ┌──────────────────────┐
  │     Host Agent       │ ── host:create ──────────► │  Signaling Server    │
  │  host-agent-go/      │ ◄─ host:created ─────────  │  server/server.js    │
  │  Remotely.exe        │                            │  Port 3000 (default) │
  │                      │ ◄─ viewer:connected ──────  │                      │
  │  FFmpeg (gdigrab)    │                            │                      │
  │  user32 SendInput    │ ── rtc:offer ────────────► │ ── rtc:offer ──────► │
  │                      │ ◄─ rtc:answer ───────────  │ ◄─ rtc:answer ─────  │
  │                      │ ◄─►  rtc:ice  ───────────► │ ◄─►  rtc:ice  ─────► │
  └──────────────────────┘                            └──────────────────────┘
            │                                                    │
            │  WebRTC P2P video (VP8, DTLS-SRTP)                 │ WebSocket
            │  ◄── input:mouse / keyboard / scroll ──            │
            │                                          ┌─────────┴────────────┐
            └──────────────────────────────────────►   │  Web Viewer          │
                                                       │  web/public/index.html│
                                                       │  Browser WebRTC API  │
                                                       └──────────────────────┘
```

### Data flow

1. Host agent connects to signaling server → receives 9-digit session key
2. Viewer enters key on website → server introduces both peers
3. WebRTC negotiated (offer/answer/ICE relayed through signaling server)
4. Video streams P2P (VP8, DTLS-SRTP encrypted); input events travel back via WebSocket relay
5. Session deleted when host disconnects

---

## Project structure

```
/server/
  server.js          WebSocket signaling server (session keys, SDP/ICE relay)
  package.json
  setup.sh           One-time Ubuntu 24.04 deployment script

/host-agent-go/      Go native host agent (single binary, no runtime required)
  main.go            Entry point, signaling loop, reconnect
  capture.go         FFmpeg subprocess, IVF/VP8 parsing, Pion track feed
  webrtc.go          Pion PeerConnection, offer/answer, ICE
  input_windows.go   Windows input injection via user32.dll SendInput
  input_stub.go      No-op stubs for non-Windows builds
  ffmpeg_windows.go  gdigrab capture args for Windows
  ffmpeg_stub.go     x11grab args for Linux
  build.bat          One-click Windows build → dist\Remotely.exe
  go.mod / go.sum

/web/public/
  index.html         Single-file SPA (landing page + viewer UI)

README.md
```

---

## Setup & run

### Prerequisites

- **FFmpeg** installed and in `PATH` on the host machine — `winget install ffmpeg`

### Signaling server

```bash
cd server && npm install
node server.js                   # port 3000
PORT=8080 node server.js         # custom port
```

The server also serves the viewer SPA at `http://localhost:3000`.

### Host agent (Windows)

Download `Remotely.exe` from the [latest release](https://github.com/remyseven/tools-remote-access/releases/latest) and run it:

```powershell
.\Remotely.exe                                    # connects to wss://remote.logicnode.us
.\Remotely.exe --server wss://your-server.com     # custom server
.\Remotely.exe --quality 60 --fps 10              # lower bandwidth
```

On first run, the agent prints a session key like `284-731-095`. Share it with the viewer.

### Web viewer

Open `https://remote.logicnode.us` (or `http://localhost:3000` for local dev) in a browser.
Enter the 9-digit key and click **Connect**.

---

## Build from source (Windows)

Requires [Go 1.21+](https://go.dev/dl/).

```powershell
cd host-agent-go
go mod tidy
.\build.bat          # produces dist\Remotely.exe (~9 MB)
```

Cross-compile from Linux/macOS:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/Remotely.exe .
```

---

## Test signaling locally

```bash
# Terminal 1 — start server
node server/server.js

# Terminal 2 — simulate a host
node -e "
const WebSocket = require('ws');
const ws = new WebSocket('ws://localhost:3000');
ws.on('open', () => ws.send(JSON.stringify({ type: 'host:create', hostInfo: {} })));
ws.on('message', d => console.log(JSON.parse(d)));
"
```

---

## Session key format

- 9 raw digits internally: `284731095`
- Display format: `284-731-095`
- Generated with `crypto.randomInt` (cryptographically random)
- Always strip non-digits before storing or comparing: `cleanKey(key)`
- Format for display: `formatKey(key)` — both defined in `server.js`

---

## WebRTC signaling protocol

All WebSocket messages are JSON.

| Type | Direction | Purpose |
|------|-----------|---------|
| `host:create` | Host → Server | Create session, get key |
| `host:created` | Server → Host | Returns `key` and `displayKey` |
| `viewer:join` | Viewer → Server | Join session by key |
| `viewer:joined` | Server → Viewer | Confirms session found |
| `viewer:connected` | Server → Host | New viewer arrived |
| `rtc:offer` | Host → Server → Viewer | WebRTC offer SDP |
| `rtc:answer` | Viewer → Server → Host | WebRTC answer SDP |
| `rtc:ice` | Both → Server → Both | ICE candidates |
| `input:mouse` | Viewer → Server → Host | Mouse move/click/down/up |
| `input:keyboard` | Viewer → Server → Host | Key down/up |
| `input:scroll` | Viewer → Server → Host | Scroll delta |
| `clipboard:sync` | Either → Server → Other | Clipboard text sync |
| `session:ended` | Server → Viewer | Host disconnected |
| `ping` / `pong` | Viewer ↔ Server | Latency measurement |

---

## Security notes

- Session keys are cryptographically random (`crypto.randomInt`); deleted when the host disconnects
- WebRTC DTLS-SRTP: the relay server never decodes the video stream
- Input injection (`user32.dll SendInput`) runs with the agent's OS user privileges — inform users before deploying
- Rate limiting on `host:create`: 5 sessions per IP per minute
- Input payloads sanitized on the server before relay
- WebSocket messages capped at 64 KB (`maxPayload`)
- Max 10 viewers per session
- **Before public deployment:** add TURN server credentials, review rate limits, consider a host-side accept/deny prompt

---

## Known limitations / TODO

- No TURN server configured — P2P may fail behind symmetric NAT
- Multi-monitor support not implemented (primary display only)
- No audio forwarding
- No file transfer
- Viewer-only (no-input) mode not implemented
- Clipboard sync on host side requires additional implementation
