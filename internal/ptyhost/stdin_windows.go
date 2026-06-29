// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package ptyhost

import (
	"io"
	"sync"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

var procReadConsoleInputW = syscall.NewLazyDLL("kernel32.dll").NewProc("ReadConsoleInputW")

const (
	eventKey              = uint16(0x0001)
	eventWindowBufferSize = uint16(0x0004)

	vkBack   = uint16(0x08)
	vkTab    = uint16(0x09)
	vkReturn = uint16(0x0D)
	vkEscape = uint16(0x1B)
	vkPrior  = uint16(0x21)
	vkNext   = uint16(0x22)
	vkEnd    = uint16(0x23)
	vkHome   = uint16(0x24)
	vkLeft   = uint16(0x25)
	vkUp     = uint16(0x26)
	vkRight  = uint16(0x27)
	vkDown   = uint16(0x28)
	vkInsert = uint16(0x2D)
	vkDelete = uint16(0x2E)
	vkF1     = uint16(0x70)
	vkF2     = uint16(0x71)
	vkF3     = uint16(0x72)
	vkF4     = uint16(0x73)
	vkF5     = uint16(0x74)
	vkF6     = uint16(0x75)
	vkF7     = uint16(0x76)
	vkF8     = uint16(0x77)
	vkF9     = uint16(0x78)
	vkF10    = uint16(0x79)
	vkF11    = uint16(0x7A)
	vkF12    = uint16(0x7B)

	leftCtrlPressed  = uint32(0x0008)
	rightCtrlPressed = uint32(0x0004)
)

// keyEventRecord matches Windows KEY_EVENT_RECORD (16 bytes)
type keyEventRecord struct {
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

// windowBufferSizeRecord matches Windows WINDOW_BUFFER_SIZE_RECORD (4 bytes)
type windowBufferSizeRecord struct {
	Cols int16
	Rows int16
}

// inputRecord matches Windows INPUT_RECORD (20 bytes)
type inputRecord struct {
	EventType uint16
	_         [2]byte
	Event     [16]byte
}

// keyToVT converts a key-down event to the VT/ANSI byte sequence the PTY
// expects, returning nil for events that produce no input.
func keyToVT(ke *keyEventRecord) []byte {
	if ke.KeyDown == 0 {
		return nil
	}
	isCtrl := ke.ControlKeyState&(leftCtrlPressed|rightCtrlPressed) != 0

	switch ke.VirtualKeyCode {
	case vkEscape:
		return []byte{0x1b}
	case vkBack:
		return []byte{0x7f}
	case vkReturn:
		return []byte{'\r'}
	case vkTab:
		return []byte{'\t'}
	case vkUp:
		return []byte{0x1b, '[', 'A'}
	case vkDown:
		return []byte{0x1b, '[', 'B'}
	case vkRight:
		return []byte{0x1b, '[', 'C'}
	case vkLeft:
		return []byte{0x1b, '[', 'D'}
	case vkHome:
		return []byte{0x1b, '[', 'H'}
	case vkEnd:
		return []byte{0x1b, '[', 'F'}
	case vkInsert:
		return []byte{0x1b, '[', '2', '~'}
	case vkDelete:
		return []byte{0x1b, '[', '3', '~'}
	case vkPrior:
		return []byte{0x1b, '[', '5', '~'}
	case vkNext:
		return []byte{0x1b, '[', '6', '~'}
	case vkF1:
		return []byte{0x1b, 'O', 'P'}
	case vkF2:
		return []byte{0x1b, 'O', 'Q'}
	case vkF3:
		return []byte{0x1b, 'O', 'R'}
	case vkF4:
		return []byte{0x1b, 'O', 'S'}
	case vkF5:
		return []byte{0x1b, '[', '1', '5', '~'}
	case vkF6:
		return []byte{0x1b, '[', '1', '7', '~'}
	case vkF7:
		return []byte{0x1b, '[', '1', '8', '~'}
	case vkF8:
		return []byte{0x1b, '[', '1', '9', '~'}
	case vkF9:
		return []byte{0x1b, '[', '2', '0', '~'}
	case vkF10:
		return []byte{0x1b, '[', '2', '1', '~'}
	case vkF11:
		return []byte{0x1b, '[', '2', '3', '~'}
	case vkF12:
		return []byte{0x1b, '[', '2', '4', '~'}
	}

	if ke.UnicodeChar == 0 {
		return nil
	}
	r := rune(ke.UnicodeChar)
	if isCtrl && r >= 0x40 && r < 0x60 {
		return []byte{byte(r & 0x1F)}
	}
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], r)
	return buf[:n]
}

// startStdinForward reads raw Windows console events via ReadConsoleInputW.
// KEY_EVENTs are converted to VT sequences and written to dst.
// WINDOW_BUFFER_SIZE_EVENTs call resize(cols, rows) so the PTY tracks the
// host terminal size — avoiding ANSI cursor corruption on the host display.
func startStdinForward(dst io.Writer, mu *sync.Mutex, resize func(cols, rows int)) {
	handle, _ := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	go func() {
		for {
			var rec inputRecord
			var nRead uint32
			ret, _, _ := procReadConsoleInputW.Call(
				uintptr(handle),
				uintptr(unsafe.Pointer(&rec)),
				1,
				uintptr(unsafe.Pointer(&nRead)),
			)
			if ret == 0 || nRead == 0 {
				return
			}
			switch rec.EventType {
			case eventKey:
				ke := (*keyEventRecord)(unsafe.Pointer(&rec.Event[0]))
				data := keyToVT(ke)
				if len(data) > 0 {
					mu.Lock()
					dst.Write(data)
					mu.Unlock()
				}
			case eventWindowBufferSize:
				if resize != nil {
					sz := (*windowBufferSizeRecord)(unsafe.Pointer(&rec.Event[0]))
					if sz.Cols > 0 && sz.Rows > 0 {
						resize(int(sz.Cols), int(sz.Rows))
					}
				}
			}
		}
	}()
}
