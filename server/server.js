'use strict';

const crypto = require('crypto');
const fs = require('fs');
const http = require('http');
const path = require('path');
const { WebSocketServer, WebSocket } = require('ws');

const PORT = process.env.PORT || 3000;

// Rate limiting: max 5 host:create per IP per minute
const RATE_LIMIT_MAX = 5;
const RATE_LIMIT_WINDOW_MS = 60_000;
const rateLimitMap = new Map(); // ip -> { count, resetAt }

// Sessions: key -> { host: WebSocket, viewers: Set<WebSocket>, createdAt: number }
const sessions = new Map();

// Max viewers allowed per session
const MAX_VIEWERS_PER_SESSION = 10;

// Stale session TTL: 8 hours
const SESSION_TTL_MS = 8 * 60 * 60 * 1000;

function generateKey() {
  let key = '';
  for (let i = 0; i < 9; i++) key += crypto.randomInt(0, 10);
  return key;
}

function cleanKey(key) {
  return String(key).replace(/\D/g, '');
}

function formatKey(key) {
  const k = cleanKey(key);
  return `${k.slice(0, 3)}-${k.slice(3, 6)}-${k.slice(6, 9)}`;
}

function isRateLimited(ip) {
  const now = Date.now();
  let entry = rateLimitMap.get(ip);
  if (!entry || now > entry.resetAt) {
    entry = { count: 0, resetAt: now + RATE_LIMIT_WINDOW_MS };
    rateLimitMap.set(ip, entry);
  }
  entry.count++;
  return entry.count > RATE_LIMIT_MAX;
}

function send(ws, data) {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(data));
  }
}

// Only pass known-safe fields from input events to prevent injection
function sanitizeInput(msg) {
  switch (msg.type) {
    case 'input:mouse':
      return {
        action: typeof msg.action === 'string' ? msg.action : '',
        x: typeof msg.x === 'number' ? Math.max(0, Math.min(1, msg.x)) : 0,
        y: typeof msg.y === 'number' ? Math.max(0, Math.min(1, msg.y)) : 0,
        button: ['left', 'right', 'middle'].includes(msg.button) ? msg.button : 'left',
      };
    case 'input:keyboard':
      return {
        action: typeof msg.action === 'string' ? msg.action : '',
        key: typeof msg.key === 'string' ? msg.key.slice(0, 32) : '',
        modifiers: Array.isArray(msg.modifiers)
          ? msg.modifiers.filter(m => ['ctrl', 'alt', 'shift', 'command'].includes(m))
          : [],
      };
    case 'input:scroll':
      return {
        x: typeof msg.x === 'number' ? Math.max(0, Math.min(1, msg.x)) : 0,
        y: typeof msg.y === 'number' ? Math.max(0, Math.min(1, msg.y)) : 0,
        deltaX: typeof msg.deltaX === 'number' ? msg.deltaX : 0,
        deltaY: typeof msg.deltaY === 'number' ? msg.deltaY : 0,
      };
    default:
      return {};
  }
}

// Serve the viewer SPA when accessed via browser
const server = http.createServer((req, res) => {
  if (req.method === 'GET' && (req.url === '/' || req.url === '/index.html')) {
    const filePath = path.join(__dirname, '..', 'web', 'public', 'index.html');
    fs.readFile(filePath, (err, data) => {
      if (err) {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        res.end('Viewer not found. Ensure web/public/index.html exists.\n');
        return;
      }
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(data);
    });
  } else {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end('Remotely signaling server\n');
  }
});

const wss = new WebSocketServer({ server, maxPayload: 64 * 1024 }); // 64 KB max message

wss.on('connection', (ws, req) => {
  // Use socket address when available (not loopback); otherwise take the
  // rightmost x-forwarded-for entry (appended by the reverse proxy, not spoofable).
  const socketIp = req.socket.remoteAddress;
  const isLoopback = socketIp === '127.0.0.1' || socketIp === '::1' || socketIp === '::ffff:127.0.0.1';
  const ip = isLoopback
    ? (req.headers['x-forwarded-for']?.split(',').pop()?.trim() || socketIp)
    : socketIp;

  ws._sessionKey = null;
  ws._role = null; // 'host' | 'viewer'

  ws.on('message', (raw) => {
    let msg;
    try {
      msg = JSON.parse(raw);
    } catch {
      return;
    }

    if (!msg.type || typeof msg.type !== 'string') return;

    switch (msg.type) {
      case 'host:create': {
        if (ws._role) return; // already registered
        if (isRateLimited(ip)) {
          send(ws, {
            type: 'error',
            code: 'rate_limited',
            message: 'Too many sessions. Try again in a minute.',
          });
          return;
        }

        let key;
        let attempts = 0;
        do {
          key = generateKey();
          attempts++;
        } while (sessions.has(key) && attempts < 100);

        if (sessions.has(key)) {
          send(ws, { type: 'error', code: 'server_full', message: 'Server at capacity.' });
          return;
        }

        ws._sessionKey = key;
        ws._role = 'host';
        sessions.set(key, { host: ws, viewers: new Set(), createdAt: Date.now() });
        send(ws, { type: 'host:created', key, displayKey: formatKey(key) });
        console.log(`[session] created ${formatKey(key)} from ${ip}`);
        break;
      }

      case 'viewer:join': {
        if (ws._role) return; // already registered
        const key = cleanKey(msg.key || '');
        if (key.length !== 9) {
          send(ws, { type: 'error', code: 'invalid_key', message: 'Invalid session key.' });
          return;
        }

        const session = sessions.get(key);
        if (!session) {
          send(ws, { type: 'error', code: 'not_found', message: 'Session not found.' });
          return;
        }

        if (session.viewers.size >= MAX_VIEWERS_PER_SESSION) {
          send(ws, { type: 'error', code: 'session_full', message: 'Session is full.' });
          return;
        }

        ws._sessionKey = key;
        ws._role = 'viewer';
        session.viewers.add(ws);

        send(ws, { type: 'viewer:joined', displayKey: formatKey(key) });
        send(session.host, { type: 'viewer:connected' });
        console.log(`[session] viewer joined ${formatKey(key)}`);
        break;
      }

      // WebRTC signaling — relay SDP/ICE without decoding
      case 'rtc:offer': {
        if (ws._role !== 'host') return;
        const session = sessions.get(ws._sessionKey);
        if (!session) return;
        for (const viewer of session.viewers) {
          send(viewer, { type: 'rtc:offer', sdp: msg.sdp });
        }
        break;
      }

      case 'rtc:answer': {
        if (ws._role !== 'viewer') return;
        const session = sessions.get(ws._sessionKey);
        if (!session) return;
        send(session.host, { type: 'rtc:answer', sdp: msg.sdp });
        break;
      }

      case 'rtc:ice': {
        const session = sessions.get(ws._sessionKey);
        if (!session) return;
        const candidate = msg.candidate;
        if (ws._role === 'host') {
          for (const viewer of session.viewers) send(viewer, { type: 'rtc:ice', candidate });
        } else if (ws._role === 'viewer') {
          send(session.host, { type: 'rtc:ice', candidate });
        }
        break;
      }

      // Input events — relay from viewer to host only
      case 'input:mouse':
      case 'input:keyboard':
      case 'input:scroll': {
        if (ws._role !== 'viewer') return;
        const session = sessions.get(ws._sessionKey);
        if (!session) return;
        send(session.host, { type: msg.type, ...sanitizeInput(msg) });
        break;
      }

      case 'clipboard:sync': {
        const session = sessions.get(ws._sessionKey);
        if (!session) return;
        const payload = {
          type: 'clipboard:sync',
          text: typeof msg.text === 'string' ? msg.text.slice(0, 65536) : '',
        };
        if (ws._role === 'host') {
          for (const viewer of session.viewers) send(viewer, payload);
        } else if (ws._role === 'viewer') {
          send(session.host, payload);
        }
        break;
      }

      case 'ping': {
        send(ws, { type: 'pong', ts: msg.ts });
        break;
      }

      default:
        break;
    }
  });

  ws.on('close', () => {
    if (!ws._sessionKey) return;
    const session = sessions.get(ws._sessionKey);
    if (!session) return;

    if (ws._role === 'host') {
      for (const viewer of session.viewers) {
        send(viewer, { type: 'session:ended' });
      }
      sessions.delete(ws._sessionKey);
      console.log(`[session] ended ${formatKey(ws._sessionKey)}`);
    } else if (ws._role === 'viewer') {
      session.viewers.delete(ws);
    }
  });

  ws.on('error', (err) => {
    console.error('[ws] connection error:', err.message);
  });
});

// Purge stale sessions hourly
setInterval(() => {
  const now = Date.now();
  for (const [key, session] of sessions) {
    if (now - session.createdAt > SESSION_TTL_MS) {
      for (const viewer of session.viewers) send(viewer, { type: 'session:ended' });
      sessions.delete(key);
      console.log(`[session] purged stale session ${formatKey(key)}`);
    }
  }
}, 60_000);

server.listen(PORT, () => {
  console.log(`Remotely signaling server listening on port ${PORT}`);
  console.log(`Viewer URL: http://localhost:${PORT}`);
});
