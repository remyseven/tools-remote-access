//go:build windows

package main

import "fmt"

func ffmpegArgs(fps, quality, w, h int) []string {
	return []string{
		"-f", "gdigrab",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", "desktop",
		"-vf", fmt.Sprintf("scale=%d:%d,format=yuv420p", w, h),
		"-c:v", "libvpx",
		"-quality", "realtime",
		"-cpu-used", "8",
		"-b:v", "2M",
		"-maxrate", "2M",
		"-auto-alt-ref", "0",
		"-g", fmt.Sprintf("%d", fps), // keyframe every 1 second — allows decoder to recover
		"-crf", fmt.Sprintf("%d", 63-quality/2), // quality 1-100 → crf 62-13
		"-f", "ivf",
		"pipe:1",
	}
}
