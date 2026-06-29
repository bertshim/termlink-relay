// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package ptyhost

import (
	"io"
	"os"
	"sync"
)

// startStdinForward forwards local operator keystrokes from stdin to the PTY.
// The resize callback is unused on Unix, where SIGWINCH (see watchResize) drives
// PTY resizing; it exists to match the Windows build's signature.
func startStdinForward(dst io.Writer, mu *sync.Mutex, _ func(cols, rows int)) {
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				mu.Lock()
				dst.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
}
