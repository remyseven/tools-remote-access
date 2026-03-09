//go:build !windows

package main

import "log"

func getScreenSize() (int, int) {
	// Default fallback for non-Windows builds
	return 1920, 1080
}

func handleInput(msg InputMsg) {
	log.Printf("[input] input injection not supported on this platform (type=%s)", msg.Type)
}
