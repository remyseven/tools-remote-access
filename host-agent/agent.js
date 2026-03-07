'use strict';

const { spawn } = require('child_process');
const WebSocket = require('ws');
const { RTCPeerConnection, RTCSessionDescription, RTCIceCandidate, MediaStream, nonstandard: { RTCVideoSource } } = require('wrtc');
const robot = require('@jitsi/robotjs');

// --- CLI args ---

function parseArgs() {
  const argv = process.argv.slice(2);
  const result = { server: 'wss://remote.logicnode.us', quality: 75, fps: 15 };
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--server' && argv[i + 1]) result.server = argv[++i];
    else if (argv[i] === '--quality' && argv[i + 1]) result.quality = Math.max(1, Math.min(100, parseInt(argv[++i]) || 75));
    else if (argv[i] === '--fps' && argv[i + 1]) result.fps = Math.max(1, Math.min(30, parseInt(argv[++i]) || 15));
  }
  return result;
}

const args = parseArgs();
const SERVER_URL = args.server;
const FPS = args.fps;

const ICE_SERVERS = [
  { urls: 'stun:stun.l.google.com:19302' },
  { urls: 'stun:stun1.l.google.com:19302' },
];

// --- Screen dimensions (must be even for I420/YUV420p) ---

const screenSize = robot.getScreenSize();
const WIDTH = screenSize.width % 2 === 0 ? screenSize.width : screenSize.width - 1;
const HEIGHT = screenSize.height % 2 === 0 ? screenSize.height : screenSize.height - 1;
const FRAME_SIZE = Math.floor(WIDTH * HEIGHT * 1.5); // I420

// --- State ---

let ws = null;
let peerConnection = null;
let videoSource = null;
let ffmpegProcess = null;
let frameBuffer = Buffer.alloc(0);
let reconnectTimer = null;

// --- FFmpeg screen capture ---

function ffmpegInputArgs() {
  switch (process.platform) {
    case 'win32':
      return ['-f', 'gdigrab', '-framerate', String(FPS), '-i', 'desktop'];
    case 'darwin':
      return ['-f', 'avfoundation', '-framerate', String(FPS), '-capture_cursor', '1', '-i', '1'];
    default: // linux
      return ['-f', 'x11grab', '-framerate', String(FPS), '-i', ':0.0'];
  }
}

function startCapture() {
  if (ffmpegProcess) return;

  const ffmpegArgs = [
    ...ffmpegInputArgs(),
    '-vf', `scale=${WIDTH}:${HEIGHT}`,
    '-f', 'rawvideo',
    '-pix_fmt', 'yuv420p',
    'pipe:1',
  ];

  ffmpegProcess = spawn('ffmpeg', ffmpegArgs, { stdio: ['ignore', 'pipe', 'ignore'] });

  ffmpegProcess.stdout.on('data', (chunk) => {
    frameBuffer = Buffer.concat([frameBuffer, chunk]);
    while (frameBuffer.length >= FRAME_SIZE) {
      const frame = frameBuffer.slice(0, FRAME_SIZE);
      frameBuffer = frameBuffer.slice(FRAME_SIZE);
      if (videoSource) {
        try {
          videoSource.onFrame({ width: WIDTH, height: HEIGHT, data: new Uint8ClampedArray(frame) });
        } catch (err) {
          // Non-fatal — screen capture glitches should not crash the agent
          console.error('[capture] frame error:', err.message);
        }
      }
    }
  });

  ffmpegProcess.on('error', (err) => {
    if (err.code === 'ENOENT') {
      console.error('[ffmpeg] not found — please install FFmpeg and ensure it is in your PATH');
    } else {
      console.error('[ffmpeg] error:', err.message);
    }
    ffmpegProcess = null;
  });

  ffmpegProcess.on('exit', (code) => {
    console.log('[ffmpeg] exited with code', code);
    ffmpegProcess = null;
  });
}

function stopCapture() {
  if (ffmpegProcess) {
    ffmpegProcess.kill('SIGTERM');
    ffmpegProcess = null;
  }
  frameBuffer = Buffer.alloc(0);
}

// --- WebRTC ---

async function startWebRTC() {
  peerConnection = new RTCPeerConnection({ iceServers: ICE_SERVERS });
  videoSource = new RTCVideoSource();
  const videoTrack = videoSource.createTrack();
  const stream = new MediaStream([videoTrack]);
  peerConnection.addTrack(videoTrack, stream);

  startCapture();

  peerConnection.onicecandidate = ({ candidate }) => {
    if (candidate) {
      sendWs({ type: 'rtc:ice', candidate: {
        candidate: candidate.candidate,
        sdpMid: candidate.sdpMid,
        sdpMLineIndex: candidate.sdpMLineIndex,
        usernameFragment: candidate.usernameFragment,
      }});
    }
  };

  peerConnection.onconnectionstatechange = () => {
    const state = peerConnection.connectionState;
    console.log('[rtc] connection state:', state);
    if (state === 'disconnected' || state === 'failed' || state === 'closed') {
      cleanupPeerConnection();
    }
  };

  const offer = await peerConnection.createOffer();
  await peerConnection.setLocalDescription(offer);
  sendWs({ type: 'rtc:offer', sdp: peerConnection.localDescription });
}

function cleanupPeerConnection() {
  stopCapture();
  if (peerConnection) {
    peerConnection.close();
    peerConnection = null;
  }
  videoSource = null;
}

// --- Input injection ---

// Browser key names -> robotjs key names
const KEY_MAP = {
  ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down',
  Backspace: 'backspace', Delete: 'delete', Enter: 'enter', Escape: 'escape',
  Tab: 'tab', Home: 'home', End: 'end', PageUp: 'pageup', PageDown: 'pagedown',
  CapsLock: 'capslock', Insert: 'insert', PrintScreen: 'printscreen',
  ' ': 'space',
  F1: 'f1', F2: 'f2', F3: 'f3', F4: 'f4', F5: 'f5', F6: 'f6',
  F7: 'f7', F8: 'f8', F9: 'f9', F10: 'f10', F11: 'f11', F12: 'f12',
};

function mapKey(browserKey) {
  if (KEY_MAP[browserKey]) return KEY_MAP[browserKey];
  if (browserKey.length === 1) return browserKey.toLowerCase();
  return null;
}

function handleInput(msg) {
  try {
    switch (msg.type) {
      case 'input:mouse': {
        const x = Math.round(msg.x * WIDTH);
        const y = Math.round(msg.y * HEIGHT);
        switch (msg.action) {
          case 'move':
            robot.moveMouse(x, y);
            break;
          case 'down':
            robot.moveMouse(x, y);
            robot.mouseToggle('down', msg.button || 'left');
            break;
          case 'up':
            robot.moveMouse(x, y);
            robot.mouseToggle('up', msg.button || 'left');
            break;
          case 'click':
            robot.moveMouse(x, y);
            robot.mouseClick(msg.button || 'left');
            break;
          case 'dblclick':
            robot.moveMouse(x, y);
            robot.mouseClick(msg.button || 'left', true);
            break;
        }
        break;
      }

      case 'input:keyboard': {
        const key = mapKey(msg.key);
        if (!key) break;
        const mods = Array.isArray(msg.modifiers) ? msg.modifiers : [];
        switch (msg.action) {
          case 'down':
            robot.keyToggle(key, 'down', mods);
            break;
          case 'up':
            robot.keyToggle(key, 'up', mods);
            break;
          case 'press':
            robot.keyTap(key, mods);
            break;
        }
        break;
      }

      case 'input:scroll': {
        // deltaY > 0 = scroll down; convert browser delta (~100px) to robotjs ticks
        const dy = Math.sign(msg.deltaY) * Math.ceil(Math.abs(msg.deltaY) / 100);
        const dx = Math.sign(msg.deltaX) * Math.ceil(Math.abs(msg.deltaX) / 100);
        if (dy !== 0) robot.scrollMouse(0, dy);
        if (dx !== 0) robot.scrollMouse(dx, 0);
        break;
      }
    }
  } catch (err) {
    // Non-fatal — input injection errors should never crash the agent
    console.error('[input] error:', err.message);
  }
}

// --- WebSocket / signaling ---

function sendWs(data) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(data));
  }
}

function connect() {
  clearTimeout(reconnectTimer);
  console.log(`[agent] connecting to ${SERVER_URL}...`);
  ws = new WebSocket(SERVER_URL);

  ws.on('open', () => {
    console.log('[agent] connected');
    sendWs({ type: 'host:create', hostInfo: { platform: process.platform } });
  });

  ws.on('message', async (raw) => {
    let msg;
    try {
      msg = JSON.parse(raw);
    } catch {
      return;
    }

    switch (msg.type) {
      case 'host:created': {
        console.log('\n  ┌──────────────────────────┐');
        console.log(`  │  Session key: ${msg.displayKey}   │`);
        console.log('  └──────────────────────────┘');
        console.log('  Share this key with the viewer.\n');
        break;
      }

      case 'viewer:connected': {
        console.log('[agent] viewer connected — initiating WebRTC');
        try {
          await startWebRTC();
        } catch (err) {
          console.error('[rtc] failed to start:', err.message);
        }
        break;
      }

      case 'rtc:answer': {
        if (!peerConnection) break;
        try {
          await peerConnection.setRemoteDescription(new RTCSessionDescription(msg.sdp));
        } catch (err) {
          console.error('[rtc] setRemoteDescription error:', err.message);
        }
        break;
      }

      case 'rtc:ice': {
        if (!peerConnection || !msg.candidate) break;
        try {
          await peerConnection.addIceCandidate(new RTCIceCandidate(msg.candidate));
        } catch (err) {
          console.error('[rtc] addIceCandidate error:', err.message);
        }
        break;
      }

      case 'input:mouse':
      case 'input:keyboard':
      case 'input:scroll':
        handleInput(msg);
        break;

      case 'clipboard:sync':
        // Clipboard integration requires platform APIs not bundled here.
        // See: https://github.com/sindresorhus/clipboardy for an optional extension.
        console.log('[clipboard] sync received (not yet implemented)');
        break;

      case 'error':
        console.error('[server]', msg.code, '—', msg.message);
        break;
    }
  });

  ws.on('close', () => {
    console.log('[agent] disconnected — reconnecting in 5s...');
    cleanupPeerConnection();
    reconnectTimer = setTimeout(connect, 5000);
  });

  ws.on('error', (err) => {
    console.error('[ws] error:', err.message);
  });
}

// --- Startup ---

console.log(`Remotely host agent`);
console.log(`  Server : ${SERVER_URL}`);
console.log(`  Screen : ${WIDTH}x${HEIGHT}  FPS: ${FPS}`);
console.log(`  Platform: ${process.platform}\n`);

connect();

process.on('SIGINT', () => {
  console.log('\n[agent] shutting down...');
  cleanupPeerConnection();
  if (ws) ws.close();
  process.exit(0);
});
