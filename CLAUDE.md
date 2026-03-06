# Remotely — Claude Code Project Prompt

## Project Overview
Remotely is a lightweight, ad-hoc remote access utility (join.me / TeamViewer-style).
No accounts required. Host runs a native agent, gets a 9-digit key, shares it with
the viewer who connects via browser. Peer-to-peer WebRTC when possible, relay fallback
via signaling server.

## Architecture
```
/server/server.js        — WebSocket signaling server (session keys, WebRTC relay)
/host-agent/agent.js     — Native Node.js agent (screen capture via FFmpeg, input via robotjs)
/web/public/index.html   — Single-file SPA (landing + download + viewer UI)
README.md                — Full architecture diagram and deployment guide
```

### Data Flow
1. Host agent connects to signaling server → receives 9-digit session key
2. Viewer enters key on website → signaling server introduces both peers
3. WebRTC negotiated (offer/answer/ICE relayed through server)
4. Video streams P2P (DTLS-SRTP encrypted); input events sent back via data channel
5. Session ends when host disconnects or quits agent

## Tech Stack
- **Signaling server**: Node.js, `ws` (WebSocket), plain HTTP — no framework
- **Host agent**: Node.js, `node-webrtc`, `@jitsi/robotjs`, FFmpeg (system dependency)
- **Web viewer**: Vanilla JS, WebRTC browser APIs, single HTML file (no build step)
- **Packaging**: `pkg` to bundle agent into standalone `.exe` / `.dmg` / `.AppImage`

## Key Constraints
- **No accounts, no persistence** — sessions are ephemeral, deleted on disconnect
- **Single HTML file** — `web/public/index.html` must stay self-contained, no separate CSS/JS files
- **Cross-platform agent** — agent.js must support Windows (GDI), macOS (AVFoundation), Linux (X11)
- **Minimal dependencies** — server has only `ws`; keep it that way unless there's strong reason
- **WebRTC encrypted** — relay server must never decode video; only relay SDP/ICE

## Code Style
- Plain JavaScript (no TypeScript) — keeps it accessible and easy to bundle with `pkg`
- `const` / `let` only, no `var`
- Async/await for all async operations, no raw `.then()` chains
- Single-responsibility functions, keep files under ~300 lines
- Comments on non-obvious logic only; self-documenting names preferred
- Error handling: non-fatal errors (input injection, screen capture glitches) should log and continue, never crash the agent

## Commands

### Server
```bash
cd server && npm install
node server.js                   # default port 3000
PORT=8080 node server.js         # custom port
```

### Host Agent
```bash
cd host-agent && npm install
node agent.js                                        # connects to localhost:3000
node agent.js --server wss://your-server.com         # production server
node agent.js --quality 60 --fps 10                  # lower bandwidth
```

### Build distributable binaries
```bash
cd host-agent
npm install -g pkg
npm run build:win    # → dist/Remotely-Setup.exe
npm run build:mac    # → dist/Remotely-Mac
npm run build:linux  # → dist/remotely-linux
```

### Test signaling server locally
```bash
# Terminal 1
node server/server.js

# Terminal 2 — simulate a host connecting
node -e "
const WebSocket = require('ws');
const ws = new WebSocket('ws://localhost:3000');
ws.on('open', () => ws.send(JSON.stringify({ type: 'host:create', hostInfo: {} })));
ws.on('message', d => console.log(JSON.parse(d)));
"
```

## Session Key Format
- 9 raw digits internally: `284731095`
- Display format with dashes: `284-731-095`
- Always strip non-digits before storing or comparing keys
- Use `cleanKey(key)` / `formatKey(key)` helpers in server.js

## WebRTC Signaling Message Protocol
All WebSocket messages are JSON. Key message types:

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
| `input:keyboard` | Viewer → Server → Host | Key down/up events |
| `input:scroll` | Viewer → Server → Host | Scroll delta |
| `clipboard:sync` | Either → Server → Other | Clipboard text sync |
| `session:ended` | Server → Viewer | Host disconnected |
| `ping` / `pong` | Viewer ↔ Server | Latency measurement |

## Common Tasks

### Adding a new input event type
1. Add handler in `server.js` under the relevant `case` (relay pattern matches existing input types)
2. Emit from viewer in `web/public/index.html` inside `setupInput()`
3. Handle in `host-agent/agent.js` inside `handleInput()`

### Adding a new toolbar button (viewer UI)
- Add button HTML in `.viewer-toolbar` div in `index.html`
- Style using existing `.toolbar-btn` class
- Wire up click handler in the `<script>` block

### Changing session key length or format
- Modify `generateKey()` in `server.js`
- Update `formatKey()` / `cleanKey()` in both `server.js` and `index.html`
- Update `maxlength` on `#keyInput` in `index.html`

### Adding TURN server support
- Add credentials to `CONFIG.iceServers` in `host-agent/agent.js`
- Add same credentials to `config.iceServers` in the `handleOffer()` function in `index.html`

## Security Considerations
- Session keys are single-use per connection and deleted on host disconnect
- WebRTC DTLS-SRTP: signaling server cannot read video stream
- Do NOT add persistent storage or user accounts without security review
- Input injection (robotjs) runs with the agent's OS user privileges — document this clearly
- Rate-limit `host:create` messages per IP before deploying publicly
- Consider adding a host-side accept/deny dialog before viewer gains control

## Known Limitations & TODO
- FFmpeg must be pre-installed on the host machine (not bundled in pkg build yet)
- `node-webrtc` has platform-specific native binaries — test pkg build on each target OS
- No TURN server configured by default — P2P may fail behind symmetric NAT
- Multi-monitor support not yet implemented (FFmpeg captures primary display only)
- No audio forwarding
- No file transfer
- Viewer-only (no-input) mode not yet implemented
