package main

import (
	"log"

	"github.com/pion/webrtc/v3"
)

// startWebRTC creates a PeerConnection, adds the given track, creates an offer,
// and sends it via the signaling WebSocket. The caller must add the track to the
// PC before creating the offer so the track appears in the SDP.
func startWebRTC(track *webrtc.TrackLocalStaticSample) (*webrtc.PeerConnection, error) {
	cfg := webrtc.Configuration{ICEServers: iceServers}
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return nil, err
	}

	if _, err = pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return nil, err
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // end-of-candidates marker
		}
		sendWS(map[string]interface{}{
			"type":      "rtc:ice",
			"candidate": c.ToJSON(),
		})
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("[rtc] connection state: %s", s)
		if s == webrtc.PeerConnectionStateDisconnected ||
			s == webrtc.PeerConnectionStateFailed ||
			s == webrtc.PeerConnectionStateClosed {
			cleanupPeer()
		}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		_ = pc.Close()
		return nil, err
	}

	sendWS(map[string]interface{}{
		"type": "rtc:offer",
		"sdp":  pc.LocalDescription(),
	})

	return pc, nil
}
