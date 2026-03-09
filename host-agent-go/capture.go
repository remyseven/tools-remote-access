package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// IVF container constants
const (
	ivfFileHeaderSize  = 32
	ivfFrameHeaderSize = 12
)

// newVideoTrack creates a VP8 track (must be added to PC before CreateOffer).
func newVideoTrack() (*webrtc.TrackLocalStaticSample, error) {
	return webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video", "remotely-screen",
	)
}

// startCapture launches FFmpeg and pipes VP8 IVF frames into track.
// Returns a cancel func that stops FFmpeg.
func startCapture(fps, quality int, track *webrtc.TrackLocalStaticSample) (cancel func(), err error) {
	w, h := getScreenSize()
	if w%2 != 0 {
		w--
	}
	if h%2 != 0 {
		h--
	}

	args := ffmpegArgs(fps, quality, w, h)
	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil // discard FFmpeg noise

	if err := cmd.Start(); err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("ffmpeg not found — please install FFmpeg and ensure it is in your PATH")
		}
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	done := make(chan struct{})
	cancel = func() {
		select {
		case <-done:
		default:
			close(done)
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}

	go func() {
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		// Skip IVF global header (32 bytes, magic "DKIF")
		hdr := make([]byte, ivfFileHeaderSize)
		if _, err := io.ReadFull(stdout, hdr); err != nil {
			log.Printf("[capture] IVF header read error: %v", err)
			return
		}
		if string(hdr[:4]) != "DKIF" {
			log.Printf("[capture] unexpected IVF magic: %q", string(hdr[:4]))
			return
		}

		frameDuration := time.Duration(float64(time.Second) / float64(fps))
		frameHdr := make([]byte, ivfFrameHeaderSize)

		for {
			select {
			case <-done:
				return
			default:
			}

			// IVF per-frame header: uint32 size + uint64 pts
			if _, err := io.ReadFull(stdout, frameHdr); err != nil {
				if err != io.EOF {
					log.Printf("[capture] IVF frame header error: %v", err)
				}
				return
			}
			frameSize := binary.LittleEndian.Uint32(frameHdr[:4])

			frameData := make([]byte, frameSize)
			if _, err := io.ReadFull(stdout, frameData); err != nil {
				log.Printf("[capture] IVF frame data error: %v", err)
				return
			}

			if err := track.WriteSample(media.Sample{
				Data:     frameData,
				Duration: frameDuration,
			}); err != nil {
				log.Printf("[capture] WriteSample error: %v", err)
				return
			}
		}
	}()

	return cancel, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return err == exec.ErrNotFound || containsStr(msg, "executable file not found") || containsStr(msg, "no such file")
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 || len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
