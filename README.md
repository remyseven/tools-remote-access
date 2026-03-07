# Remotely

Lightweight ad-hoc remote access (join.me / TeamViewer-style). No accounts, no installation for viewers. Host runs a native agent, shares a 9-digit key, viewer connects via browser.

---

## Architecture

```
  ┌──────────────────────┐   WebSocket (signaling)   ┌──────────────────────┐
  │     Host Agent       │ ── host:create ──────────► │  Signaling Server    │
  │   host-agent/agent.js│ ◄─ host:created ─────────  │  server/server.js    │
  │                      │                            │  Port 3000 (default) │
  │  FFmpeg (capture)    │ ◄─ viewer:connected ──────  │                      │
  │  robotjs (input)     │                            │                      │
  │                      │ ── rtc:offer ────────────► │ ── rtc:offer ──────► │
  │                      │ ◄─ rtc:answer ───────────  │ ◄─ rtc:answer ─────  │
  │                      │ ◄─►  rtc:ice  ───────────► │ ◄─►  rtc:ice  ─────► │
  └──────────────────────┘                            └──────────────────────┘
            │                                                    │
            │  WebRTC P2P video (DTLS-SRTP)                      │ WebSocket
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
4. Video streams P2P (DTLS-SRTP encrypted); input events travel back via WebSocket relay
5. Session deleted when host disconnects

---

## Project structure

```
/server/
  server.js          WebSocket signaling server (session keys, SDP/ICE relay)
  package.json
  setup.sh           One-time Ubuntu 24.04 deployment script

/host-agent/
  agent.js           Native Node.js agent (FFmpeg capture, robotjs input injection)
  Remotely.bat       Windows launcher (double-click to run)
  package.json

/web/public/
  index.html         Single-file SPA (landing page + viewer UI)

README.md
```

---

## Setup & run

### Prerequisites

- **Node.js 18+**
- **FFmpeg** installed and in `PATH` (host machine only)

### Signaling server

```bash
cd server && npm install
node server.js                   # port 3000
PORT=8080 node server.js         # custom port
```

The server also serves the viewer SPA at `http://localhost:3000`.

### Host agent

```bash
cd host-agent && npm install
node agent.js                                              # connects to wss://remote.logicnode.us
node agent.js --server wss://your-server.com               # custom server
node agent.js --quality 60 --fps 10                        # lower bandwidth
```

On first run, the agent prints a session key like `284-731-095`. Share it with the viewer.

### Web viewer

Open `https://remote.logicnode.us` (or `http://localhost:3000` for local dev) in a browser.
Enter the 9-digit key and click **Connect**.

---

## Build distributable (Windows)

The Windows distribution is a portable zip — `pkg` cannot load native `.node` binaries (robotjs, wrtc) from its virtual snapshot filesystem, so we bundle a portable `node.exe` instead.

```powershell
cd host-agent

# Download portable Node 18
Invoke-WebRequest -Uri "https://nodejs.org/dist/v18.20.4/node-v18.20.4-win-x64.zip" -OutFile node18.zip
Expand-Archive node18.zip -DestinationPath node18-tmp
New-Item -ItemType Directory -Force dist | Out-Null
Copy-Item node18-tmp\node-v18.20.4-win-x64\node.exe dist\node.exe
Remove-Item -Recurse node18-tmp, node18.zip

# Copy app files
Copy-Item agent.js dist\
Copy-Item Remotely.bat dist\
Copy-Item -Recurse node_modules dist\

# Zip
Compress-Archive -Path dist\* -DestinationPath dist\Remotely-Windows.zip -Force
```

Users extract the zip and double-click `Remotely.bat`. A terminal window stays open showing the session key.

> **Note:** FFmpeg must be installed separately on the host machine (`winget install ffmpeg`).

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

- Session keys are single-use; deleted when the host disconnects
- WebRTC DTLS-SRTP: the relay server never decodes the video stream
- Input injection (robotjs) runs with the agent's OS user privileges — inform users before deploying
- Rate limiting on `host:create`: 5 sessions per IP per minute
- Input payloads are sanitized on the server before relay
- **Before public deployment:** add TURN server credentials, review rate limits, consider a host-side accept/deny prompt

---

## Known limitations / TODO

- FFmpeg must be pre-installed (not bundled in pkg build)
- No TURN server configured — P2P may fail behind symmetric NAT
- Multi-monitor support not implemented (primary display only)
- No audio forwarding
- No file transfer
- Viewer-only (no-input) mode not implemented
- Clipboard sync on host side requires additional native library
