//go:build windows

package main

import (
	"log"
	"math"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	MOUSEEVENTF_MOVE        = 0x0001
	MOUSEEVENTF_LEFTDOWN    = 0x0002
	MOUSEEVENTF_LEFTUP      = 0x0004
	MOUSEEVENTF_RIGHTDOWN   = 0x0008
	MOUSEEVENTF_RIGHTUP     = 0x0010
	MOUSEEVENTF_MIDDLEDOWN  = 0x0020
	MOUSEEVENTF_MIDDLEUP    = 0x0040
	MOUSEEVENTF_WHEEL       = 0x0800
	MOUSEEVENTF_HWHEEL      = 0x1000
	MOUSEEVENTF_ABSOLUTE    = 0x8000
	MOUSEEVENTF_VIRTUALDESK = 0x4000

	KEYEVENTF_EXTENDEDKEY = 0x0001
	KEYEVENTF_KEYUP       = 0x0002
	KEYEVENTF_UNICODE     = 0x0004

	WHEEL_DELTA = 120
)

// INPUT struct (40 bytes on 64-bit): type(4) + padding(4) + union(32)
type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
	_           [8]byte // pad to 32 bytes for the union
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
	_           [16]byte // pad to 32 bytes for the union
}

type inputUnion struct {
	inputType uint32
	_         uint32 // alignment padding
	data      [32]byte
}

func sendMouseInput(dx, dy int32, flags, mouseData uint32) {
	mi := mouseInput{dx: dx, dy: dy, dwFlags: flags, mouseData: mouseData}
	var inp inputUnion
	inp.inputType = INPUT_MOUSE
	*(*mouseInput)(unsafe.Pointer(&inp.data[0])) = mi
	procSendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
}

func sendKeyInput(vk uint16, flags uint32) {
	ki := keybdInput{wVk: vk, dwFlags: flags}
	var inp inputUnion
	inp.inputType = INPUT_KEYBOARD
	*(*keybdInput)(unsafe.Pointer(&inp.data[0])) = ki
	procSendInput.Call(1, uintptr(unsafe.Pointer(&inp)), unsafe.Sizeof(inp))
}

func getScreenSize() (int, int) {
	w, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	h, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	return int(w), int(h)
}

// normalizeToAbsolute converts 0.0-1.0 ratio to 0-65535 absolute coordinate
func normalizeToAbsolute(ratio float64) int32 {
	v := ratio * 65535.0
	if v < 0 {
		v = 0
	} else if v > 65535 {
		v = 65535
	}
	return int32(math.Round(v))
}

func buttonFlags(button string, down bool) uint32 {
	switch strings.ToLower(button) {
	case "right":
		if down {
			return MOUSEEVENTF_RIGHTDOWN
		}
		return MOUSEEVENTF_RIGHTUP
	case "middle":
		if down {
			return MOUSEEVENTF_MIDDLEDOWN
		}
		return MOUSEEVENTF_MIDDLEUP
	default: // left
		if down {
			return MOUSEEVENTF_LEFTDOWN
		}
		return MOUSEEVENTF_LEFTUP
	}
}

// browserKeyToVK maps browser key names to Windows virtual key codes
func browserKeyToVK(key string) (uint16, bool) {
	if len(key) == 1 {
		r := rune(key[0])
		if unicode.IsLetter(r) {
			return uint16(unicode.ToUpper(r)), true
		}
		if unicode.IsDigit(r) {
			return uint16(r), true
		}
	}
	vkMap := map[string]uint16{
		"ArrowLeft": 0x25, "ArrowUp": 0x26, "ArrowRight": 0x27, "ArrowDown": 0x28,
		"Backspace": 0x08, "Delete": 0x2E, "Enter": 0x0D, "Escape": 0x1B,
		"Tab": 0x09, "Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
		"Insert": 0x2D, "CapsLock": 0x14, "PrintScreen": 0x2C,
		" ": 0x20,
		"F1": 0x70, "F2": 0x71, "F3": 0x72, "F4": 0x73,
		"F5": 0x74, "F6": 0x75, "F7": 0x76, "F8": 0x77,
		"F9": 0x78, "F10": 0x79, "F11": 0x7A, "F12": 0x7B,
		"Shift": 0x10, "Control": 0x11, "Alt": 0x12, "Meta": 0x5B,
		// OEM punctuation keys
		";": 0xBA, "=": 0xBB, ",": 0xBC, "-": 0xBD, ".": 0xBE,
		"/": 0xBF, "`": 0xC0, "[": 0xDB, "\\": 0xDC, "]": 0xDD, "'": 0xDE,
	}
	vk, ok := vkMap[key]
	return vk, ok
}

func handleInput(msg InputMsg) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[input] panic: %v", r)
		}
	}()

	switch msg.Type {
	case "input:mouse":
		ax := normalizeToAbsolute(msg.X)
		ay := normalizeToAbsolute(msg.Y)
		baseFlags := uint32(MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_VIRTUALDESK)
		switch msg.Action {
		case "move":
			sendMouseInput(ax, ay, baseFlags, 0)
		case "down":
			sendMouseInput(ax, ay, baseFlags|buttonFlags(msg.Button, true), 0)
		case "up":
			sendMouseInput(ax, ay, baseFlags|buttonFlags(msg.Button, false), 0)
		case "click":
			sendMouseInput(ax, ay, baseFlags|buttonFlags(msg.Button, true), 0)
			sendMouseInput(ax, ay, baseFlags|buttonFlags(msg.Button, false), 0)
		case "dblclick":
			for i := 0; i < 2; i++ {
				sendMouseInput(ax, ay, baseFlags|buttonFlags(msg.Button, true), 0)
				sendMouseInput(ax, ay, baseFlags|buttonFlags(msg.Button, false), 0)
			}
		}

	case "input:keyboard":
		vk, ok := browserKeyToVK(msg.Key)
		if !ok {
			return
		}
		// Press modifier keys down first
		mods := modifierVKs(msg.Modifiers)
		for _, m := range mods {
			sendKeyInput(m, 0)
		}
		switch msg.Action {
		case "down":
			sendKeyInput(vk, 0)
		case "up":
			sendKeyInput(vk, KEYEVENTF_KEYUP)
		case "press":
			sendKeyInput(vk, 0)
			sendKeyInput(vk, KEYEVENTF_KEYUP)
		}
		// Release modifiers
		for _, m := range mods {
			sendKeyInput(m, KEYEVENTF_KEYUP)
		}

	case "input:scroll":
		// Vertical scroll
		if msg.DeltaY != 0 {
			ticks := int32(math.Copysign(math.Ceil(math.Abs(msg.DeltaY)/100)*WHEEL_DELTA, -msg.DeltaY))
			sendMouseInput(0, 0, MOUSEEVENTF_WHEEL, uint32(ticks))
		}
		// Horizontal scroll
		if msg.DeltaX != 0 {
			ticks := int32(math.Copysign(math.Ceil(math.Abs(msg.DeltaX)/100)*WHEEL_DELTA, msg.DeltaX))
			sendMouseInput(0, 0, MOUSEEVENTF_HWHEEL, uint32(ticks))
		}
	}
}

func modifierVKs(mods []string) []uint16 {
	var out []uint16
	for _, m := range mods {
		switch strings.ToLower(m) {
		case "shift":
			out = append(out, 0x10)
		case "control", "ctrl":
			out = append(out, 0x11)
		case "alt":
			out = append(out, 0x12)
		case "meta", "super":
			out = append(out, 0x5B)
		}
	}
	return out
}
