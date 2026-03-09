//go:build !windows

package main

import "fmt"

func ffmpegArgs(fps, quality, w, h int) []string {
	scale := fmt.Sprintf("scale=%d:%d,format=yuv420p", w, h)
	crf := fmt.Sprintf("%d", 63-quality/2)
	// Linux: x11grab; macOS: avfoundation
	return []string{
		"-f", "x11grab",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", ":0.0",
		"-vf", scale,
		"-c:v", "libvpx",
		"-quality", "realtime",
		"-cpu-used", "8",
		"-b:v", "2M",
		"-maxrate", "2M",
		"-auto-alt-ref", "0",
		"-g", fmt.Sprintf("%d", fps),
		"-crf", crf,
		"-f", "ivf",
		"pipe:1",
	}
}
