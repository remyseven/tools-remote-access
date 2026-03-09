package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var (
	serverURL = flag.String("server", "wss://remote.logicnode.us", "Signaling server URL")
	quality   = flag.Int("quality", 75, "FFmpeg JPEG quality (1-100)")
	fps       = flag.Int("fps", 15, "Capture frame rate (1-30)")
)

var iceServers = []webrtc.ICEServer{
	{URLs: []string{"stun:stun.l.google.com:19302"}},
	{URLs: []string{"stun:stun1.l.google.com:19302"}},
}

// Shared state (accessed only from ws message goroutine or under protection)
var (
	wsConn         *websocket.Conn
	peerConn       *webrtc.PeerConnection
	captureCancel  func()
)

type HostCreatedMsg struct {
	Key        string `json:"key"`
	DisplayKey string `json:"displayKey"`
}

type InputMsg struct {
	Type      string   `json:"type"`
	Action    string   `json:"action"`
	X         float64  `json:"x"`
	Y         float64  `json:"y"`
	Button    string   `json:"button"`
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers"`
	DeltaX    float64  `json:"deltaX"`
	DeltaY    float64  `json:"deltaY"`
}

func sendWS(data interface{}) {
	if wsConn == nil {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	if err := wsConn.WriteMessage(websocket.TextMessage, b); err != nil {
		log.Printf("[ws] send error: %v", err)
	}
}

func cleanupPeer() {
	if captureCancel != nil {
		captureCancel()
		captureCancel = nil
	}
	if peerConn != nil {
		_ = peerConn.Close()
		peerConn = nil
	}
}

func onViewerConnected() {
	cleanupPeer()

	// Track must be added to PC before CreateOffer so it appears in SDP
	track, err := newVideoTrack()
	if err != nil {
		log.Printf("[rtc] create track: %v", err)
		return
	}

	pc, err := startWebRTC(track)
	if err != nil {
		log.Printf("[rtc] failed to start: %v", err)
		return
	}
	peerConn = pc

	cancel, err := startCapture(*fps, *quality, track)
	if err != nil {
		log.Printf("[capture] failed to start: %v", err)
		return
	}
	captureCancel = cancel
}

func connect() {
	for {
		log.Printf("[agent] connecting to %s...", *serverURL)
		c, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
		if err != nil {
			log.Printf("[ws] dial error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}
		wsConn = c
		log.Println("[agent] connected")

		sendWS(map[string]interface{}{
			"type":     "host:create",
			"hostInfo": map[string]string{"platform": "windows"},
		})

		runLoop(c)
		cleanupPeer()
		wsConn = nil
		log.Println("[agent] disconnected — reconnecting in 5s...")
		time.Sleep(5 * time.Second)
	}
}

func runLoop(c *websocket.Conn) {
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			log.Printf("[ws] read error: %v", err)
			return
		}

		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &base); err != nil {
			continue
		}

		switch base.Type {
		case "host:created":
			var m HostCreatedMsg
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			fmt.Printf("\n  ┌─────────────────────────────┐\n")
			fmt.Printf("  │  Session key: %s   │\n", m.DisplayKey)
			fmt.Printf("  └─────────────────────────────┘\n")
			fmt.Printf("  Share this key with the viewer.\n\n")

		case "viewer:connected":
			log.Println("[agent] viewer connected — initiating WebRTC")
			onViewerConnected()

		case "rtc:answer":
			if peerConn == nil {
				continue
			}
			var m struct {
				SDP webrtc.SessionDescription `json:"sdp"`
			}
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if err := peerConn.SetRemoteDescription(m.SDP); err != nil {
				log.Printf("[rtc] setRemoteDescription error: %v", err)
			}

		case "rtc:ice":
			if peerConn == nil {
				continue
			}
			var m struct {
				Candidate *webrtc.ICECandidateInit `json:"candidate"`
			}
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if m.Candidate == nil || m.Candidate.Candidate == "" {
				continue // end-of-candidates signal
			}
			if err := peerConn.AddICECandidate(*m.Candidate); err != nil {
				log.Printf("[rtc] addIceCandidate error: %v", err)
			}

		case "input:mouse", "input:keyboard", "input:scroll":
			var m InputMsg
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			handleInput(m)

		case "clipboard:sync":
			log.Println("[clipboard] sync received (not yet implemented)")

		case "error":
			var m struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(raw, &m)
			log.Printf("[server] %s — %s", m.Code, m.Message)
		}
	}
}

func main() {
	flag.Parse()

	// Clamp values
	if *quality < 1 {
		*quality = 1
	} else if *quality > 100 {
		*quality = 100
	}
	if *fps < 1 {
		*fps = 1
	} else if *fps > 30 {
		*fps = 30
	}

	w, h := getScreenSize()
	fmt.Printf("Remotely host agent\n")
	fmt.Printf("  Server  : %s\n", *serverURL)
	fmt.Printf("  Screen  : %dx%d  FPS: %d\n", w, h, *fps)
	fmt.Printf("  Platform: windows\n\n")

	// Handle Ctrl-C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\n[agent] shutting down...")
		cleanupPeer()
		if wsConn != nil {
			_ = wsConn.Close()
		}
		os.Exit(0)
	}()

	connect()
}
